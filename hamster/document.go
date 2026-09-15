package hamster

import (
	"encoding/json"
	"fmt"

	"github.com/hwcer/cosgo/schema"
	"github.com/hwcer/logger"
	"github.com/hwcer/updater/dataset"
	"github.com/hwcer/updater/operator"
)

// DocumentModel 核心版单文档模型接口 —— 主档（公会主档、玩家档案）就注册成它。
//
// 与扩展层 updater.DocumentModel 的差异：**没有 IType / Field(iid)** ——
// 那两个是道具概念，唯一硬消费点是扩展层 operator 构造时的 IType(0) 查询
// （HAMSTER_PLAN.md 第二节 1），核心版按纯字段定位，不需要。
// 建议在业务 model 中实现 dataset.ModelGet / dataset.ModelSet 接口提高性能。
type DocumentModel interface {
	New(s *Store) any
	Getter(s *Store, data *dataset.Document, keys []string) error
	Setter(s *Store, bw BulkWrite, dirty dataset.Update, unset []string) error
}

// Document 单文档存储（核心版纯净实现，无任何道具概念）。
// 首要角色是**主档**：以 TableOrder 声明最大加载顺序，即可先于其他模型加载，
// 其余模型的 Getter 以 Store.Id() 为键基础派生查询。
type Document struct {
	Statement
	name string
	// schema 首次解析成功后缓存:整个 handle 生命周期内文档类型固定(model.New 只产出一种类型),
	// 而 Field/Name/Table/Select 每次调用都要查字段,不缓存就要反复走 schema.Parse(反射取类型 + 全局 sync.Map)
	schema  *schema.Schema
	model   DocumentModel
	dataset *dataset.Document
}

func newDocument(s *Store, m *Model) Handle {
	r := &Document{}
	r.name = m.name
	r.model = m.model.(DocumentModel)
	r.Statement = *NewStatement(s, m.ram, r.Has)
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
	if err = this.Store.getError(); err != nil {
		return
	}
	if len(this.keys) == 0 {
		return nil
	}
	keys := this.keys.ToString()
	if err = this.model.Getter(this.Store, this.dataset, keys); err == nil {
		this.Statement.Date()
	}
	return
}

func (this *Document) Select(keys ...any) {
	for _, k := range keys {
		if key, err := this.Field(k); err == nil {
			this.Statement.Select(key)
		} else {
			logger.Alert("Document Select error,name:%s,key:%v,err:%v", this.name, k, err)
		}
	}
}

func (this *Document) Parser() Parser {
	return ParserTypeDocument
}

// ===================== Handle 接口生命周期方法 =====================

func (this *Document) Save() (err error) {
	bw := this.Store.BulkWrite()
	if bw == nil {
		return ErrBulkWriteNotInit
	}
	dirty, unsets := this.dataset.Save()
	if len(dirty) > 0 || len(unsets) > 0 {
		if err = this.model.Setter(this.Store, bw, dirty, unsets); err != nil {
			ds, _ := json.Marshal(dirty)
			logger.Alert("database save error,id:%s,Document:%s\nOperation:%s\nerror:%s", this.Store.Id(), this.name, ds, err.Error())
		}
	}
	return
}

func (this *Document) Reset() {
	this.Statement.Reset()
	if this.dataset == nil {
		this.dataset = dataset.NewDoc(nil)
	}
}

func (this *Document) Reload() error {
	this.dataset = nil
	this.schema = nil
	this.Statement.Reload()
	return this.Loading()
}

func (this *Document) Loading() (err error) {
	if this.dataset == nil {
		this.dataset = dataset.NewDoc(nil)
	}
	if this.Statement.Loading() {
		this.Store.setError(this.model.Getter(this.Store, this.dataset, nil))
		if err = this.Store.getError(); err == nil {
			this.loader = true
		}
	} else if this.dataset.IsNil() {
		this.dataset.Reset(this.model.New(this.Store))
	}
	return this.Store.getError()
}

func (this *Document) Release() {
	this.Statement.Release()
	if this.Statement.ram == RAMTypeNone {
		this.dataset = nil
		this.schema = nil
	} else {
		this.dataset.Release()
	}
}

func (this *Document) Destroy() (err error) {
	return this.Save()
}

func (this *Document) Commit() (err error) {
	if err = this.Store.WriteAble(); err != nil {
		return
	}
	this.Statement.Submit()
	if err = this.Save(); err != nil && this.Statement.ram != RAMTypeNone {
		logger.Alert("数据库[%v]同步数据错误,等待下次同步:%v", this.Table(), err)
		err = nil
	}
	return
}

func (this *Document) Verify() (err error) {
	if err = this.Store.WriteAble(); err != nil {
		return
	}
	// 下标遍历(而非 range)：当前 parse 分支都不追加操作，但那是实现的性质、不是接口保证
	// —— range 按初始长度迭代，哪天有分支开始追加就会静默漏掉新增的那几条。
	// 🔴 用 this.Statement.operator 限定：Document 上的 operator 方法会遮蔽嵌入的同名字段。
	for i := 0; i < len(this.Statement.operator); i++ {
		if err = this.Parse(this.Statement.operator[i]); err != nil {
			return
		}
	}
	this.Statement.Verify()
	return
}

// ===================== 类型特有公开方法 =====================

func (this *Document) Add(k any, v any) *operator.Operator {
	return this.fieldOperator(operator.TypesAdd, k, dataset.ParseInt64(v), nil)
}

func (this *Document) Sub(k any, v any) *operator.Operator {
	return this.fieldOperator(operator.TypesSub, k, dataset.ParseInt64(v), nil)
}

// Set 设置字段
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
		this.Store.setError(fmt.Errorf("document dataset not init,model:%s", this.name))
		return nil
	}
	sch, err := this.dataset.Schema()
	if err != nil {
		this.Store.setError(err)
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
	if err := this.Store.getError(); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("document schema not ready,model:%s", this.name)
}

// Name 字段名(json 名)
//
// 🔴 内部统一用 json 名,不用落库名。理由是 op.Field / op.Result 同时喂两个
// 下游:一是发给客户端的 payload(客户端按 json 名认字段),二是落库。让它带 json 名,
// 客户端那侧就是直通;落库那侧由 cosmo 在边界统一换成 DBName。
func (this *Document) Name(k string) (r string, err error) {
	sch, err := this.sch()
	if err != nil {
		return "", err
	}
	return sch.JSName(k)
}

// Field 字段名定位与校验（json 名规范化，含多级路径 a.b.c）。
//
// 🔴 校验必不可少：不校验则字段名写错(nosuchfield.1)会一路放行到 dataset.Document.Set，
// 那里 `if !doc.Has(k) { return }` 直接静默返回，调用方拿不到错误、还以为写成功了。
func (this *Document) Field(k any) (key string, err error) {
	v, ok := k.(string)
	if !ok {
		return "", fmt.Errorf("document field must be string:%+v", k)
	}
	key = v
	sch, err := this.sch()
	if err != nil {
		return "", err
	}
	return sch.JSName(key)
}

func (this *Document) Insert(op *operator.Operator, before ...bool) {
	this.Statement.Insert(op, before...)
}

// ===================== 类型特有私有方法 =====================

func (this *Document) val(k string) (r int64, ok bool) {
	if v := this.dataset.Val(k); v != nil {
		r, ok = dataset.TryParseInt64(v)
	}
	return
}

// fieldOperator 先定位字段名再生成操作,Add/Sub/Set/Unset 共用
//
// 🔴 只在 Field 失败时写 Error:无条件赋值会在解析成功时把 nil 写回去 ——
// 此前挂起的错误被抹掉,紧接着的 operator() → WriteAble() 误判为可写
func (this *Document) fieldOperator(t operator.Types, k any, v int64, r any) *operator.Operator {
	field, err := this.Field(k)
	if err != nil {
		this.Store.setError(err)
		return nil
	}
	return this.operator(t, field, v, r)
}

// operator 构造并入队一条操作。核心版 operator 的 IType 恒 0（合法值，默认不进下发通道）。
func (this *Document) operator(t operator.Types, k string, v int64, r any) *operator.Operator {
	if err := this.Store.WriteAble(); err != nil {
		return nil
	}
	if t == operator.TypesDel {
		logger.Debug("hamster document del is disabled")
		return nil
	}
	if v <= 0 && (t == operator.TypesAdd || t == operator.TypesSub) {
		return nil
	}
	op := operator.New(t, k, v, r)
	this.Statement.Select(op.Field)
	this.Statement.Insert(op)
	return op
}
