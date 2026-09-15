package hamster

import (
	"encoding/json"

	"github.com/hwcer/logger"
	"github.com/hwcer/updater/dataset"
	"github.com/hwcer/updater/operator"
)

// ValuesModel 核心版数值 KV 模型接口 —— 纯存储契约（int32 只是键类型，无任何道具语义）。
// 与扩展层 updater.ValuesModel 的差异仅是首参 *Store。
type ValuesModel interface {
	Getter(s *Store, data *dataset.Values, keys []int32) error
	Setter(s *Store, bulkWrite BulkWrite, dirty dataset.Data, unset []int32) error
}

// Values 数值键值对（核心版纯数据结构：int32→int64 的 KV + 字段级 Add/Sub）。
//
// 与扩展层 Values 的差异：没有 IType 查询（那边 IType 查不到会**静默丢弃操作**，
// 这边没有丢弃路径）、没有溢出检查。适合积分、计数、公会资金这类纯数值场景。
type Values struct {
	statement Statement
	name      string
	model     ValuesModel
	dataset   *dataset.Values
}

func newValues(s *Store, m *Model) Handle {
	r := &Values{}
	r.name = m.name
	r.model = m.model.(ValuesModel)
	r.statement = *NewStatement(s, m.ram, r.Has)
	//核心版产出的 operator IType 恒 0，默认不进通用更新通道（与 Collection 同口径）
	r.statement.Receiver(DiscardReceiver)
	return r
}

// ===================== Handle 接口公开方法 =====================

func (this *Values) Get(k any) any {
	return this.dataset.Val(dataset.ParseInt32(k))
}

func (this *Values) Val(k any) (r int64) {
	return this.dataset.Val(dataset.ParseInt32(k))
}

func (this *Values) Data() (err error) {
	if err = this.statement.Store.Error; err != nil {
		return
	}
	if len(this.statement.keys) == 0 {
		return nil
	}
	keys := this.statement.keys.ToInt32()
	if err = this.model.Getter(this.statement.Store, this.dataset, keys); err == nil {
		this.statement.Date()
	}
	return
}

// Select 指定需要从数据库拉取的 key。内存模式(RAMTypeAlways)下数据已全量在内存，直接跳过。
func (this *Values) Select(keys ...any) {
	if this.statement.ram == RAMTypeAlways {
		return
	}
	for _, k := range keys {
		if id, ok := dataset.TryParseInt32(k); ok {
			this.statement.Select(id)
		}
	}
}

func (this *Values) Parser() Parser {
	return ParserTypeValues
}

// ===================== Handle 接口生命周期方法 =====================

func (this *Values) save() (err error) {
	bw := this.statement.Store.BulkWrite()
	if bw == nil {
		return ErrBulkWriteNotInit
	}
	dirty, unsets := this.dataset.Save()
	if len(dirty) > 0 || len(unsets) > 0 {
		if err = this.model.Setter(this.statement.Store, bw, dirty, unsets); err != nil {
			ds, _ := json.Marshal(dirty)
			logger.Alert("database save error,id:%s,Values:%s\nOperation:%s\nerror:%s", this.statement.Store.Id(), this.name, ds, err.Error())
		}
	}
	return
}

func (this *Values) reset() {
	this.statement.Reset()
	if this.dataset == nil {
		this.dataset = dataset.NewValues()
	}
	if r, ok := this.model.(ModelReset); ok && r.Reset(this.statement.Store, this.statement.Store.Last()) {
		this.statement.Store.Error = this.reload()
	}
}

func (this *Values) reload() error {
	this.dataset = nil
	this.statement.Reload()
	return this.loading()
}

func (this *Values) loading() error {
	if this.dataset == nil {
		this.dataset = dataset.NewValues()
	}
	if this.statement.Loading() {
		this.statement.Store.Error = this.model.Getter(this.statement.Store, this.dataset, nil)
		if err := this.statement.Store.Error; err == nil {
			this.statement.loader = true
		}
	}
	return this.statement.Store.Error
}

func (this *Values) release() {
	this.statement.Release()
	if this.statement.ram == RAMTypeNone {
		this.dataset = nil
	} else {
		this.dataset.Release()
	}
}

func (this *Values) destroy() (err error) {
	return this.save()
}

func (this *Values) commit() (err error) {
	if err = this.statement.Store.WriteAble(); err != nil {
		return
	}
	this.statement.Submit()
	if err = this.save(); err != nil && this.statement.ram != RAMTypeNone {
		logger.Alert("数据库[%v]同步数据错误,等待下次同步:%v", this.name, err)
		err = nil
	}
	return
}

func (this *Values) verify() (err error) {
	if err = this.statement.Store.WriteAble(); err != nil {
		return
	}
	// 下标遍历(而非 range)：当前 parse 分支都不追加操作，但那是实现的性质、不是接口保证。
	// ⚠️ Ops() 每轮重取：append 扩容后旧切片头看不见新增。
	for i := 0; i < len(this.statement.operator); i++ {
		if err = this.Parse(this.statement.operator[i]); err != nil {
			return
		}
	}
	this.statement.Verify()
	return
}

// ===================== 类型特有公开方法 =====================

func (this *Values) Add(k int32, v any) *operator.Operator {
	return this.operator(operator.TypesAdd, k, dataset.ParseInt64(v))
}

func (this *Values) Sub(k int32, v any) *operator.Operator {
	return this.operator(operator.TypesSub, k, dataset.ParseInt64(v))
}

func (this *Values) Set(k int32, v any) *operator.Operator {
	return this.operator(operator.TypesSet, k, dataset.ParseInt64(v))
}

func (this *Values) Unset(k int32) *operator.Operator {
	return this.operator(operator.TypesUnset, k, 0)
}

func (this *Values) Len() int {
	return this.dataset.Len()
}

func (this *Values) Has(k any) bool {
	return this.dataset.Has(dataset.ParseInt32(k))
}

func (this *Values) All() dataset.Data {
	return this.dataset.All()
}

func (this *Values) Range(f func(int32, int64) bool) {
	this.dataset.Range(f)
}

func (this *Values) Insert(op *operator.Operator, before ...bool) {
	this.statement.Insert(op, before...)
}

// Operators 本次请求已通过 verify、尚未 submit 的 operator 列表（只读）
func (this *Values) Operators() []*operator.Operator {
	return this.statement.cache
}

// ===================== 类型特有私有方法 =====================

func (this *Values) operator(t operator.Types, k int32, v int64) *operator.Operator {
	if err := this.statement.Store.WriteAble(); err != nil {
		return nil
	}
	if v <= 0 && (t == operator.TypesAdd || t == operator.TypesSub) {
		return nil
	}
	op := operator.New(t, "", v, nil)
	op.IID = k //键复用协议的 IID 字段；IType 恒 0（无主数据，默认不进下发通道）
	this.statement.Select(k)
	this.statement.Insert(op)
	return op
}
