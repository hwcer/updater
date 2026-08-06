package updater

import (
	"encoding/json"
	"fmt"

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
	name string
	// schema 首次解析成功后缓存:整个 handle 生命周期内文档类型固定(model.New 只产出一种类型),
	// 而 Field/Name/Table/Select 每次调用都要查字段,不缓存就要反复走 schema.Parse(反射取类型 + 全局 sync.Map)
	schema  *schema.Schema
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
	this.fieldOperator(operator.TypesAdd, id, v, nil)
}

func (this *Document) decrease(id int32, v int64) {
	this.fieldOperator(operator.TypesSub, id, v, nil)
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
	this.schema = nil
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
		this.schema = nil
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
	return this.fieldOperator(operator.TypesAdd, k, dataset.ParseInt64(v), nil)
}

func (this *Document) Sub(k any, v any) *operator.Operator {
	return this.fieldOperator(operator.TypesSub, k, dataset.ParseInt64(v), nil)
}

// Set 设置
// Set(k string|int32,v any)
func (this *Document) Set(k any, v any) *operator.Operator {
	return this.fieldOperator(operator.TypesSet, k, 0, v)
}

func (this *Document) Unset(k any) *operator.Operator {
	return this.fieldOperator(operator.TypesUnset, k, 0, nil)
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
	if this.schema != nil {
		return this.schema
	}
	if this.dataset == nil {
		this.Updater.Error = fmt.Errorf("document dataset not init,model:%s", this.name)
		return nil
	}
	sch, err := this.dataset.Schema()
	if err != nil {
		this.Updater.Error = err
		return nil
	}
	this.schema = sch
	return sch
}

// schema 取 schema,不可用时给出明确错误
// 🔴 必须报错:旧实现在 Schema() 为 nil 时一路返回 ("", nil),
// 调用方会拿着空字段名当成解析成功继续往下走
func (this *Document) sch() (*schema.Schema, error) {
	if sch := this.Schema(); sch != nil {
		return sch, nil
	}
	if this.Updater.Error != nil {
		return nil, this.Updater.Error
	}
	return nil, fmt.Errorf("document schema not ready,model:%s", this.name)
}

// Name 字段名(json 名)
//
// 🔴 updater 内部统一用 json 名,不用落库名。理由是 op.Field / op.Result 同时喂两个
// 下游:一是发给客户端的 payload(客户端按 json 名认字段),二是落库。让它带 json 名,
// 客户端那侧就是直通;落库那侧由 cosmo 在边界统一换成 DBName ——
// Update.Transform 与 Selector.Projection 都走 schema.DBName 逐段换名。
//
// 反过来(updater 发 DBName)则要求客户端认库名,且 Collection 的 op.Result 还得再转一次,
// 一份 map 两套命名,迟早分叉。
func (this *Document) Name(k string) (r string, err error) {
	sch, err := this.sch()
	if err != nil {
		return "", err
	}
	return sch.JSName(k)
}

// Field key || iid 获取字段名
func (this *Document) Field(k any) (key string, err error) {
	switch v := k.(type) {
	case string:
		key = v
	default:
		if key, err = this.model.Field(this.Updater, dataset.ParseInt32(k)); err != nil {
			return "", err
		}
	}
	//规范化成 json 名，含 mongo 风格的多级路径 a.b.c：字段段换名，map 键 / slice 下标
	//原样保留，逐段判定由 schema 负责(它才知道每一段落在什么容器上)。
	//落库名的转换在 cosmo 边界统一做，见 Name 的注释。
	//
	//🔴 校验必不可少：不校验则字段名写错(nosuchfield.1)会一路放行到 dataset.Document.Set，
	//那里 `if !doc.Has(k) { return }` 直接静默返回，调用方拿不到错误、还以为写成功了。
	//带点路径尤其要逐段校验 —— 中间段和末段写错，这一层不查就没人查了。
	sch, err := this.sch()
	if err != nil {
		return "", err
	}
	return sch.JSName(key)
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

// fieldOperator 先按 key||iid 定位字段名再生成操作,Add/Sub/Set/Unset 与 increase/decrease 共用
//
// 🔴 只在 Field 失败时写 Updater.Error:旧代码 `field, this.Updater.Error = this.Field(k)`
// 无条件赋值,解析成功时把 nil 写了回去 —— 此前挂起的错误被抹掉,紧接着的
// operator() → WriteAble() 误判为可写,本该被拦下的写入照样落库
func (this *Document) fieldOperator(t operator.Types, k any, v int64, r any) *operator.Operator {
	field, err := this.Field(k)
	if err != nil {
		this.Updater.Error = err
		return nil
	}
	return this.operator(t, field, v, r)
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
