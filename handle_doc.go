package updater

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hwcer/cosgo/schema"
	"github.com/hwcer/logger"
	"github.com/hwcer/updater/dataset"
	"github.com/hwcer/updater/operator"
)

// documentModel 文档模型接口
// 建议在业务model中实现 dataset.ModelGet 和 dataset.ModelSet 接口提高性能
type documentModel interface {
	New(update *Updater) any                                             //初始化对象
	IType(int32) int32                                                   //DOC使用FIELD操作时，无法通过IID获取类型，必须明确指定
	Field(update *Updater, iid int32) (string, error)                    //使用IID映射字段名
	Getter(update *Updater, data *dataset.Document, keys []string) error //获取数据接口,需要对data进行赋值,keys==nil 获取所有
	Setter(update *Updater, dirty dataset.Update) error                  //保存数据接口
}

// Document 文档存储
type Document struct {
	statement
	name    string
	model   documentModel  //handle model
	setter  dataset.Update //需要持久化到数据库的数据
	dataset *dataset.Document //数据
}

func NewDocument(u *Updater, m *Model) Handle {
	r := &Document{}
	r.name = m.name
	r.model = m.model.(documentModel)
	r.statement = *newStatement(u, m, r.Has)
	return r
}

// ===================== Handle 接口公开方法 =====================

func (this *Document) Get(k any) (r any) {
	if key, err := this.Field(k); err == nil {
		r = this.dataset.Val(key)
	} else {
		logger.Alert("Document get error,name:%s,key:%v,err:%v", this.name, k, err)
	}
	return
}

func (this *Document) Val(k any) (r int64) {
	if key, err := this.Field(k); err == nil {
		r, _ = this.val(key)
	}
	return
}

func (this *Document) Data() (err error) {
	if this.Updater.Error != nil {
		return this.Updater.Error
	}
	if len(this.keys) == 0 {
		return nil
	}
	keys := this.keys.ToString()
	if err = this.model.Getter(this.Updater, this.dataset, keys); err == nil {
		this.statement.date()
	}
	return
}

func (this *Document) IType(iid int32) IType {
	v := this.model.IType(iid)
	return itypesDict[v]
}

func (this *Document) Select(keys ...any) {
	for _, k := range keys {
		if key, err := this.Field(k); err == nil {
			this.statement.Select(key)
		} else {
			logger.Alert("Document Select error,name:%s,key:%v,err:%v", this.name, k, err)
		}
	}
}

func (this *Document) Parser() Parser {
	return ParserTypeDocument
}

// ===================== Handle 接口私有方法 =====================

func (this *Document) increase(id int32, v int64) {
	var k string
	if k, this.Updater.Error = this.Field(id); this.Updater.Error == nil {
		this.operator(operator.TypesAdd, k, v, nil)
	}
}

func (this *Document) decrease(id int32, v int64) {
	var k string
	if k, this.Updater.Error = this.Field(id); this.Updater.Error == nil {
		this.operator(operator.TypesSub, k, v, nil)
	}
}

func (this *Document) save() (err error) {
	if this.setter == nil {
		this.setter = dataset.Update{}
	}
	if err = this.dataset.Save(this.setter); err != nil {
		return err
	}
	if len(this.setter) == 0 {
		this.setter = nil
		return nil
	}
	if err = this.model.Setter(this.Updater, this.setter); err == nil {
		this.setter = nil
	} else {
		ds, _ := json.Marshal(this.setter)
		logger.Alert("database save error,uid:%s,Collection:%s\nOperation:%s\nerror:%s", this.Updater.Uid(), this.name, ds, err.Error())
		var s bool
		if s, err = onSaveErrorHandle(this.Updater, err); !s {
			this.setter = nil
		}
	}
	return err
}

func (this *Document) reset() {
	this.statement.reset()
	if this.dataset == nil {
		this.dataset = dataset.NewDoc(nil)
	}
	if reset, ok := this.model.(ModelReset); ok {
		if reset.Reset(this.Updater, this.Updater.last) {
			this.Updater.Error = this.reload()
		}
	}
}

func (this *Document) reload() error {
	this.dataset = nil
	this.statement.reload()
	return this.loading()
}

func (this *Document) loading() (err error) {
	if this.dataset == nil {
		this.dataset = dataset.NewDoc(nil)
	}
	if this.statement.loading() {
		if this.Updater.Error = this.model.Getter(this.Updater, this.dataset, nil); this.Updater.Error == nil {
			this.statement.loader = true
		}
	} else if this.dataset.IsNil() {
		this.dataset.Reset(this.model.New(this.statement.Updater))
	}
	return this.Updater.Error
}

func (this *Document) release() {
	this.statement.release()
	if this.statement.ram == RAMTypeNone {
		this.dataset = nil
	} else {
		this.dataset.Release()
	}
}

func (this *Document) destroy() (err error) {
	return this.save()
}

func (this *Document) submit() (err error) {
	if err = this.Updater.WriteAble(); err != nil {
		return
	}
	this.statement.submit()
	if err = this.save(); err != nil && this.ram != RAMTypeNone {
		logger.Alert("数据库[%v]同步数据错误,等待下次同步:%v", this.Table(), err)
		err = nil
	}
	return
}

func (this *Document) verify() (err error) {
	if err = this.Updater.WriteAble(); err != nil {
		return
	}
	for _, act := range this.statement.operator {
		if err = this.Parse(act); err != nil {
			return
		}
	}
	this.statement.verify()
	return
}

// ===================== 类型特有公开方法 =====================

func (this *Document) Add(k any, v any) *operator.Operator {
	var field string
	if field, this.Updater.Error = this.Field(k); this.Updater.Error == nil {
		return this.operator(operator.TypesAdd, field, dataset.ParseInt64(v), nil)
	}
	return nil
}

func (this *Document) Sub(k any, v any) *operator.Operator {
	var field string
	if field, this.Updater.Error = this.Field(k); this.Updater.Error == nil {
		return this.operator(operator.TypesSub, field, dataset.ParseInt64(v), nil)
	}
	return nil
}

// Set 设置
// Set(k string|int32,v any)
func (this *Document) Set(k any, v any) *operator.Operator {
	var field string
	if field, this.Updater.Error = this.Field(k); this.Updater.Error == nil {
		return this.operator(operator.TypesSet, field, 0, v)
	}
	return nil
}

func (this *Document) Has(k any) bool {
	return false
}

func (this *Document) Range(f func(k string, v any) bool) {
	this.dataset.Range(f)
}

func (this *Document) Any() any {
	return this.dataset.Any()
}

func (this *Document) Table() (r string) {
	if sch := this.Schema(); sch != nil {
		r = sch.Table
	}
	return
}

func (this *Document) Schema() *schema.Schema {
	sch, err := this.dataset.Schema()
	if err != nil {
		this.Updater.Error = err
	}
	return sch
}

// Name  db name
func (this *Document) Name(k string) (r string, err error) {
	if sch := this.Schema(); sch != nil {
		if field := sch.LookUpField(k); field != nil {
			r = field.JSName()
		} else {
			err = fmt.Errorf("document field not exist,model:%s,Field:%s", sch.Name, k)
		}
	}
	return
}

// Field key || iid 获取字段名
func (this *Document) Field(k any) (key string, err error) {
	switch v := k.(type) {
	case string:
		key = v
	default:
		iid := dataset.ParseInt32(k)
		key, err = this.model.Field(this.Updater, iid)
	}
	if err != nil {
		return
	}
	if strings.Contains(key, ".") {
		return
	}
	key, err = this.Name(key)
	return
}

func (this *Document) Insert(op *operator.Operator, before ...bool) {
	this.statement.insert(op, before...)
}

// ===================== 类型特有私有方法 =====================

func (this *Document) val(k string) (r int64, ok bool) {
	if v := this.dataset.Val(k); v != nil {
		r, ok = dataset.TryParseInt64(v)
	}
	return
}

func (this *Document) operator(t operator.Types, k string, v int64, r any) *operator.Operator {
	if err := this.Updater.WriteAble(); err != nil {
		return nil
	}
	if t == operator.TypesDel {
		logger.Debug("updater document del is disabled")
		return nil
	}

	op := operator.New(t, k, v, r)

	this.statement.Select(op.Field)
	it := this.IType(0)
	if it == nil {
		this.Updater.Error = fmt.Errorf("document operator key empty:%+v", op)
		return nil
	}
	op.IType = it.ID()
	if oc, ok := it.(ITypeOID); ok {
		op.OID = oc.GetOID(this.Updater, op.IID)
	}
	if listen, ok := it.(ITypeListener); ok {
		listen.Listener(this.Updater, op)
	}

	this.statement.insert(op)
	return op
}
