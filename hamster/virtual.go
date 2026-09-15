package hamster

import (
	"github.com/hwcer/updater/dataset"
	"github.com/hwcer/updater/operator"
)

// VirtualModel 核心版委托视图模型接口 —— 纯 string 键。
// 与扩展层 updater.VirtualModel 的差异：没有 Field(iid)（iid→字段映射是道具概念，留扩展层）。
type VirtualModel interface {
	Has(s *Store, k any) bool
	Get(s *Store, k any) (r any)
	Update(s *Store, op *operator.Operator)
	Select(s *Store, keys ...any)
	Reload(s *Store) error
}

// Virtual 委托视图（核心版纯数据结构）：本身不存储数据，读写委托给其他模块。
//
// 🔴 请求内中间态缓存（cache 字段）是通用的、必须的：operator 带的是**绝对值**(d±value)，
// 而委托出去的写要到 verify 才生效 —— 不缓存的话，同一请求里第二次 Add 读到的还是
// 旧值，算出的绝对值会把前一次整个覆盖掉（两次 Add(6) 只到账 6，纯静默）。
//
// 与扩展层 Virtual 的差异：键只支持 string（iid→字段的映射留在扩展层），
// 没有 IType 盖键（op.IType 恒 0）。
type Virtual struct {
	statement Statement
	name  string //model database name
	model VirtualModel
	cache map[string]int64 //本次请求内已处理过的键值，Val 优先读它
}

func newVirtual(s *Store, m *Model) Handle {
	r := &Virtual{}
	r.name = m.name
	r.model = m.model.(VirtualModel)
	r.statement = *NewStatement(s, m.ram, r.Has)
	return r
}

// ===================== Handle 接口公开方法 =====================

func (this *Virtual) Get(k any) (r any) {
	return this.model.Get(this.statement.Store, k)
}

// Val 取当前值。本次请求已经改过的键读缓存，没改过才回落到模型。
func (this *Virtual) Val(k any) (r int64) {
	if key, ok := this.key(k); ok {
		if v, exist := this.cache[key]; exist {
			return v
		}
	}
	return dataset.ParseInt64(this.model.Get(this.statement.Store, k))
}

// record 记下本次请求处理后该键的最新值，供后续 Val 读取。
func (this *Virtual) record(key string, v int64) {
	if this.cache == nil {
		this.cache = map[string]int64{}
	}
	this.cache[key] = v
}

func (this *Virtual) Data() (err error) {
	return
}

func (this *Virtual) Select(keys ...any) {
	this.model.Select(this.statement.Store, keys...)
}

func (this *Virtual) Parser() Parser {
	return ParserTypeVirtual
}

// ===================== Handle 接口生命周期方法 =====================

func (this *Virtual) save() (err error) {
	return
}

func (this *Virtual) reset() {
	this.statement.Reset()
	this.cache = nil //防御:正常由 release 清,这里再兜一次,免得异常路径把中间态带进新请求
	if r, ok := this.model.(ModelReset); ok && r.Reset(this.statement.Store, this.statement.Store.Last()) {
		this.statement.Store.Error = this.reload()
	}
}

func (this *Virtual) reload() error {
	this.cache = nil //数据要重新加载,之前记的中间态一律作废
	return this.model.Reload(this.statement.Store)
}

func (this *Virtual) loading() error {
	return nil
}

func (this *Virtual) release() {
	this.statement.Release()
	this.cache = nil //缓存只在单次请求内有效
}

func (this *Virtual) verify() (err error) {
	this.statement.Verify()
	return
}

func (this *Virtual) commit() (err error) {
	this.statement.Submit()
	return
}

func (this *Virtual) destroy() (err error) {
	return nil
}

// ===================== 类型特有公开方法 =====================

func (this *Virtual) key(i any) (key string, ok bool) {
	if key, ok = i.(string); ok {
		return
	}
	return "", false
}

func (this *Virtual) Add(k any, v any) {
	value := dataset.ParseInt64(v)
	if value <= 0 {
		return
	}
	d := this.Val(k)
	key, ok := this.key(k)
	if !ok {
		_ = this.statement.Store.Errorf("hamster Virtual Add Args Error,name:%s,key:%v(必须是 string)", this.name, k)
		return
	}
	op := this.newOperator(operator.TypesAdd, key, value, map[string]any{key: d + value})
	if op == nil {
		return
	}
	this.model.Update(this.statement.Store, op)
	this.record(key, d+value)
	this.statement.Insert(op)
}

func (this *Virtual) Sub(k any, v any) {
	value := dataset.ParseInt64(v)
	if value <= 0 {
		return
	}
	d := this.Val(k)
	key, ok := this.key(k)
	if !ok {
		_ = this.statement.Store.Errorf("hamster Virtual Sub Args Error,name:%s,key:%v(必须是 string)", this.name, k)
		return
	}
	if d < value && !this.statement.Store.CreditAllowed {
		this.statement.Store.Error = ErrNotEnough(key, value, d)
		return
	}
	op := this.newOperator(operator.TypesSub, key, value, map[string]any{key: d - value})
	if op == nil {
		return
	}
	this.model.Update(this.statement.Store, op)
	this.record(key, d-value)
	this.statement.Insert(op)
}

func (this *Virtual) Set(k any, v any) {
	key, ok := this.key(k)
	if !ok {
		_ = this.statement.Store.Errorf("hamster Virtual Set Args Error,name:%s,key:%v(必须是 string)", this.name, k)
		return
	}
	op := this.newOperator(operator.TypesSet, key, 0, map[string]any{key: v})
	this.model.Update(this.statement.Store, op)
	this.record(key, dataset.ParseInt64(v))
	this.statement.Insert(op)
}

func (this *Virtual) Has(k any) bool {
	return this.model.Has(this.statement.Store, k)
}

// Operators 本次请求已通过 verify、尚未 submit 的 operator 列表（只读）
func (this *Virtual) Operators() []*operator.Operator {
	return this.statement.cache
}

// ===================== 类型特有私有方法 =====================

func (this *Virtual) newOperator(t operator.Types, key string, v int64, r any) *operator.Operator {
	if v <= 0 && (t == operator.TypesAdd || t == operator.TypesSub) {
		return nil
	}
	op := operator.New(t, key, v, r)
	return op
}
