package updater

import (
	"fmt"
	"github.com/hwcer/cosgo/schema"
	"github.com/hwcer/logger"
	"github.com/hwcer/updater/dataset"
	"github.com/hwcer/updater/operator"
	"strings"
)

/*
切记不要直接修改dataset
建议使用dataset中实现以下接口提高性能

	Get(k string) any              //获取k的值
	Set(k string, v any) error    //设置k值的为v
*/
type documentModel interface {
	New(update *Updater) any                                             //初始化对象
	Field(update *Updater, iid int32) (string, error)                    //使用IID映射字段名
	Getter(update *Updater, data *dataset.Document, keys []string) error //获取数据接口,需要对data进行赋值,keys==nil 获取所有
	Setter(update *Updater, dirty dataset.Update) error                  //保存数据接口
}

// Document 文档存储
type Document struct {
	statement
	dirty   dataset.Update    //数据缓存
	model   documentModel     //handle model
	dataset *dataset.Document //数据
}

func NewDocument(u *Updater, model any, ram RAMType) Handle {
	r := &Document{}
	r.model = model.(documentModel)
	r.statement = *newStatement(u, ram, r.operator, r.has)
	return r
}

func (this *Document) val(k string) (r int64, ok bool) {
	if v := this.dataset.Val(k); v != nil {
		r, ok = dataset.TryParseInt64(v)
	}
	return
}

func (this *Document) save() (err error) {
	if this.Updater.Async {
		return
	}
	dirty := this.Dirty()
	if err = this.dataset.Save(dirty); err != nil {
		return
	}
	if len(dirty) == 0 {
		return nil
	}
	if err = this.model.Setter(this.statement.Updater, dirty); err == nil {
		this.dirty = nil
	}
	return
}

// reset 运行时开始时
func (this *Document) reset() {
	this.statement.reset()
	if this.dataset == nil {
		i := this.model.New(this.Updater)
		this.dataset = dataset.NewDoc(i)
	}
}

// release 运行时释放
func (this *Document) release() {
	this.statement.release()
	if !this.Updater.Async {
		if this.statement.ram == RAMTypeNone {
			this.dataset = nil
		} else {
			this.dataset.Release()
		}
	}
}
func (this *Document) init() (err error) {
	if this.statement.ram == RAMTypeMaybe {
		this.statement.ram = RAMTypeNone
		logger.Alert("updater Document模型不支持RAMTypeMaybe已经自动转换为RAMTypeNone")
	}
	if this.statement.ram == RAMTypeAlways {
		i := this.model.New(this.Updater)
		this.dataset = dataset.NewDoc(i)
		err = this.model.Getter(this.Updater, this.dataset, nil)
	}
	return
}

// 关闭时执行,玩家下线
func (this *Document) destroy() (err error) {
	return this.save()
}

func (this *Document) Has(k any) bool {
	return true
}

// Get  对象中的特定值
// 不建议使用GET获取特定字段值
func (this *Document) Get(k any) (r any) {
	if key, err := this.ObjectId(k); err == nil {
		r = this.dataset.Val(key)
	}
	return
}

// Val 不建议使用Val获取特定字段值的int64值
func (this *Document) Val(k any) (r int64) {
	if key, err := this.ObjectId(k); err == nil {
		r, _ = this.val(key)
	}
	return
}

// Set 设置
// Set(k string,v any)
func (this *Document) Set(k any, v ...any) {
	switch len(v) {
	case 1:
		this.operator(operator.TypesSet, k, 0, v[0])
	default:
		this.Updater.Error = ErrArgsIllegal(k, v)
	}
}

func (this *Document) Select(keys ...any) {
	for _, k := range keys {
		if key, err := this.ObjectId(k); err == nil {
			this.statement.Select(key)
		} else {
			logger.Alert(err)
		}
	}
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

func (this *Document) Verify() (err error) {
	if this.Updater.Error != nil {
		return this.Updater.Error
	}
	for _, act := range this.statement.operator {
		if err = this.Parse(act); err != nil {
			return
		}
	}
	this.statement.verify()
	return
}

// Table  表名
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
func (this *Document) Parser() Parser {
	return ParserTypeDocument
}

func (this *Document) submit() (err error) {
	//defer this.statement.done()
	if err = this.Updater.Error; err != nil {
		return
	}
	//同步到内存
	//r = this.statement.operator
	//for _, op := range this.statement.cache {
	//	if op.Type.IsValid() {
	//		if e := this.set(op.Key, op.Result); e != nil {
	//			logger.Debug("数据保存失败可能是类型不匹配已经丢弃,table:%v,field:%v,result:%v", this.Table(), op.Key, op.Result)
	//		} else {
	//			this.dirty[op.Key] = op.Result
	//		}
	//	}
	//}
	this.statement.submit()
	if err = this.save(); err != nil && this.ram != RAMTypeNone {
		logger.Alert("数据库[%v]同步数据错误,等待下次同步:%v", this.Table(), err)
		err = nil
	}
	return
}

// Dirty 设置脏数据,手动修改内存后置脏同步到数据库
func (this *Document) Dirty() dataset.Update {
	if this.dirty == nil {
		this.dirty = dataset.Update{}
	}
	return dataset.Update{}
}

func (this *Document) Range(f func(k string, v any) bool) {
	this.dataset.Range(f)
}

func (this *Document) Interface() any {
	return this.dataset.Any()
}

func (this *Document) ObjectId(k any) (key string, err error) {
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
	if strings.Index(key, ".") > 0 {
		return
	}
	if sch := this.Schema(); sch != nil {
		if field := sch.LookUpField(key); field != nil {
			key = field.DBName
		} else {
			err = fmt.Errorf("document field not exist")
		}
	}
	return
}
func (this *Document) IType(iid int32) IType {
	if h, ok := this.model.(modelIType); ok {
		v := h.IType(iid)
		return itypesDict[v]
	} else {
		return this.Updater.IType(iid)
	}
}
func (this *Document) operator(t operator.Types, k any, v int64, r any) {
	if t == operator.TypesDel {
		logger.Debug("updater document del is disabled")
		return
	}
	op := operator.New(t, v, r)
	switch s := k.(type) {
	case string:
		op.Key = s
	default:
		if op.IID = dataset.ParseInt32(k); op.IID > 0 {
			op.Key, this.Updater.Error = this.model.Field(this.Updater, op.IID)
		}
	}
	if this.Updater.Error != nil {
		return
	}
	//sch, err := this.dataset.Schema()
	//if err != nil {
	//	this.Updater.Error = err
	//	return
	//}
	//op.OID = sch.Table
	//if this.Updater.Error != nil {
	//	return
	//}
	this.statement.Select(op.Key)
	it := this.IType(op.IID)
	if it != nil {
		op.Bag = it.Id()
		if listen, ok := it.(ITypeListener); ok {
			listen.Listener(this.Updater, op)
		}
	}
	this.statement.Operator(op)
}
