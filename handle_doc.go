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

// DocumentModel 文档模型接口
// 建议在业务model中实现 dataset.ModelGet 和 dataset.ModelSet 接口提高性能
// 可选实现 ModelIMax 覆盖全局 Config 的上限查询
// IType 为必需方法(等价 ModelIType):Document 大部分操作按 field 定位,iid 为 0,
// 只能由模型给出默认 IType,全局 Config.IType(0) 无法兜底;Values/Virtual 无此需求,IType 是可选的
type DocumentModel interface {
	New(update *Updater) any
	IType(int32) int32
	Field(update *Updater, iid int32) (string, error)
	Getter(update *Updater, data *dataset.Document, keys []string) error
	Setter(update *Updater, bulkWrite BulkWrite, dirty dataset.Update, unset []string) error
}

// Document 文档存储
type Document struct {
	statement
	name    string
	model   DocumentModel
	dataset *dataset.Document
}

func NewDocument(u *Updater, m *Model) Handle {
	r := &Document{}
	r.name = m.name
	r.model = m.model.(DocumentModel)
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

// Count 按 iid 汇总的持有总量
// 文档型整个模型只有一个文档、iid 映射到字段,故与 Val 等价,不需要扫描
func (this *Document) Count(iid int32) int64 {
	return this.Val(iid)
}

func (this *Document) IMax(iid int32) int64 {
	return modelIMax(this.model, iid)
}

func (this *Document) IType(iid int32) IType {
	return modelIType(this.model, iid)
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
	bw := this.Updater.BulkWrite()
	if bw == nil {
		return ErrBulkWriteNotInit
	}
	dirty, unsets := this.dataset.Save()
	if len(dirty) > 0 || len(unsets) > 0 {
		if err = this.model.Setter(this.Updater, bw, dirty, unsets); err != nil {
			ds, _ := json.Marshal(dirty)
			logger.Alert("database save error,uid:%s,Document:%s\nOperation:%s\nerror:%s", this.Updater.Uid(), this.name, ds, err.Error())
		}
	}
	return
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
	// 下标遍历(而非 range):Parse 中 overflow→Resolve 可能往本 handle 追加操作(与自己同模型),
	// range 按初始长度迭代会漏掉,使其被 statement.verify() 搬进 cache 却未 Parse、最终不落库。见 handle_coll.go。
	for i := 0; i < len(this.statement.operator); i++ {
		if err = this.Parse(this.statement.operator[i]); err != nil {
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

func (this *Document) Unset(k any) *operator.Operator {
	var field string
	if field, this.Updater.Error = this.Field(k); this.Updater.Error == nil {
		return this.operator(operator.TypesUnset, field, 0, nil)
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
	//子键路径(如 soulrelics.1)：只校验根字段存在，**不改写 key**。
	//
	//校验是必须的：不校验则根字段名写错(nosuchfield.1)会一路放行到
	//dataset.Document.Set，那里 `if !doc.Has(k) { return }` 直接静默返回，
	//调用方拿不到任何错误、还以为写成功了。整字段路径走下面的 Name() 早就会报错，
	//这里补上只是让两条路径口径一致。
	//
	//🔴 只能取 err，**不能**把 Name() 的返回值拼回 key：Name() 返回 JSName()
	//(protobuf 结构上是 PascalCase)，拼回去会把 soulrelics.1 变成 SoulRelics.1，
	//而落库时 cosmo 的 update.Transform 对含 "." 的 key 原样下发、不查 schema
	//——结果是往库里写一个大小写不符的野字段，真正的字段纹丝不动，静默丢数据。
	if i := strings.Index(key, "."); i > 0 {
		if _, err = this.Name(key[:i]); err != nil {
			return "", err
		}
		return key, nil
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

	if v <= 0 && (t == operator.TypesAdd || t == operator.TypesSub) {
		return nil
	}

	op := operator.New(t, k, v, r)

	this.statement.Select(op.Field)
	it := this.IType(0)
	if it == nil {
		this.Updater.Error = fmt.Errorf("document operator key empty:%+v", op)
		op.Release()
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
