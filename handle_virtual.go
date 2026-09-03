package updater

import (
	"github.com/hwcer/updater/dataset"
	"github.com/hwcer/updater/operator"
)

// VirtualModel 虚拟模型接口
// 可选实现 ModelIMax / ModelIType 覆盖全局 Config 的上限与类型查询
type VirtualModel interface {
	Has(u *Updater, k any) bool
	Get(u *Updater, k any) (r any)
	Field(int32) (string, bool) //格式化字段
	Update(u *Updater, op *operator.Operator)
	Select(u *Updater, keys ...any)
	Reload(u *Updater) error
}

// Virtual 虚拟数据层,本身不存储数据，操作委托给其他模块
type Virtual struct {
	statement
	name  string //model database name
	model VirtualModel
	// cache 本次请求内**已处理过**的键值，Val 优先读它。
	//
	// 🔴 少了它就是一个静默丢数据的坑：Virtual 的 operator 带的是**绝对值**(d±value)，
	// 而它委托出去的 Document.Set 要到 verify 才写内存 —— 不缓存的话，同一请求里
	// 第二次 Add 读到的还是**旧值**，算出的绝对值会把前一次整个覆盖掉。
	//
	// 实测（充值发货，首充奖励与常规道具恰好是同一种货币）：
	// 两次 Add(钻石,6) 只到账 6。不报错、operator 数量也对，纯静默。
	// Sub 更危险：余额校验 `d < value` 也读旧值，同一请求扣两次能扣成负数。
	//
	// Values 那边没这毛病，因为它的 operator 是**增量**语义、verify 时依次累加；
	// Virtual 走绝对值是为了把最终值直接交给委托方，所以只能自己记住中间态。
	cache map[string]int64
}

func NewVirtual(u *Updater, m *Model) Handle {
	r := &Virtual{}
	r.name = m.name
	r.model = m.model.(VirtualModel)
	r.statement = *newStatement(u, m, r.Has)
	return r
}

// ===================== Handle 接口公开方法 =====================

func (this *Virtual) Get(k any) (r any) {
	return this.model.Get(this.Updater, k)
}

// Val 取当前值。**本次请求已经改过的键读缓存**，没改过才回落到模型（内存）。
// 理由见 Virtual.cache —— 委托出去的写要到 verify 才生效，中途读模型拿到的是旧值。
func (this *Virtual) Val(k any) (r int64) {
	if _, key, ok := this.key(k); ok {
		if v, exist := this.cache[key]; exist {
			return v
		}
	}
	return dataset.ParseInt64(this.model.Get(this.Updater, k))
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

// Count 按 iid 汇总的持有总量
// 虚拟数据本身不存储、由模型直接给出数值,故与 Val 等价,不需要扫描
func (this *Virtual) Count(iid int32) int64 {
	return this.Val(iid)
}

func (this *Virtual) IMax(iid int32) int64 {
	return modelIMax(this.model, iid)
}

// IType iid>0 时按 iid 查找，iid==0 时返回模型默认 IType
func (this *Virtual) IType(iid int32) IType {
	return modelIType(this.model, iid)
}

func (this *Virtual) Select(keys ...any) {
	this.model.Select(this.Updater, keys...)
}

func (this *Virtual) Parser() Parser {
	return ParserTypeVirtual
}

// ===================== Handle 接口私有方法 =====================

func (this *Virtual) increase(k int32, v int64) {
	this.Add(k, v)
}

func (this *Virtual) decrease(k int32, v int64) {
	this.Sub(k, v)
}

func (this *Virtual) save() (err error) {
	return
}

func (this *Virtual) reset() {
	this.statement.reset()
	this.cache = nil //防御:正常由 release 清,这里再兜一次,免得异常路径把中间态带进新请求
	if reset, ok := this.model.(ModelReset); ok {
		if reset.Reset(this.Updater, this.Updater.last) {
			this.Updater.Error = this.reload()
		}
	}
}

func (this *Virtual) reload() error {
	this.cache = nil //数据要重新加载,之前记的中间态一律作废
	return this.model.Reload(this.Updater)
}

func (this *Virtual) loading() error {
	return nil
}

func (this *Virtual) release() {
	this.statement.release()
	this.cache = nil //缓存只在单次请求内有效
}

func (this *Virtual) verify() (err error) {
	this.statement.verify()
	return
}

func (this *Virtual) submit() (err error) {
	this.statement.submit()
	return
}

func (this *Virtual) destroy() (err error) {
	return nil
}

// ===================== 类型特有公开方法 =====================

func (this *Virtual) key(i any) (iid int32, key string, ok bool) {
	switch v := i.(type) {
	case string:
		key = v
		ok = true
	default:
		if iid = dataset.ParseInt32(i); iid > 0 {
			key, ok = this.model.Field(iid)
		}
	}
	return
}
func (this *Virtual) Add(k any, v any) {
	value := dataset.ParseInt64(v)
	if value <= 0 {
		return
	}
	d := this.Val(k)
	iid, key, ok := this.key(k)
	if !ok {
		_ = this.Updater.Errorf("Virtual Add Args Error,name:%s,key:%v", this.name, k)
		return
	}

	op := this.newOperator(operator.TypesAdd, iid, key, value, map[string]any{key: d + value})
	if op == nil {
		return
	}
	this.model.Update(this.Updater, op)
	this.record(key, d+value)
	this.statement.insert(op)
}

func (this *Virtual) Sub(k any, v any) {
	value := dataset.ParseInt64(v)
	if value <= 0 {
		return
	}
	d := this.Val(k)
	iid, key, ok := this.key(k)
	if !ok {
		_ = this.Updater.Errorf("Virtual Sub Args Error,name:%s,key:%v", this.name, k)
		return
	}
	if d < value && !this.Updater.CreditAllowed {
		this.Updater.Error = ErrItemNotEnough(iid, value, d)
		return
	}
	op := this.newOperator(operator.TypesSub, iid, key, value, map[string]any{key: d - value})
	if op == nil {
		return
	}
	this.model.Update(this.Updater, op)
	this.record(key, d-value)
	this.statement.insert(op)
}

func (this *Virtual) Set(k any, v any) {
	iid, key, ok := this.key(k)
	if !ok {
		_ = this.Updater.Errorf("Virtual Set Args Error,name:%s,key:%v", this.name, k)
		return
	}
	op := this.newOperator(operator.TypesSet, iid, key, 0, map[string]any{key: v})
	this.model.Update(this.Updater, op)
	this.record(key, dataset.ParseInt64(v))
	this.statement.insert(op)
}

func (this *Virtual) Has(k any) bool {
	return this.model.Has(this.Updater, k)
}

// ===================== 类型特有私有方法 =====================

func (this *Virtual) newOperator(t operator.Types, iid int32, key string, v int64, r any) *operator.Operator {
	if v <= 0 && (t == operator.TypesAdd || t == operator.TypesSub) {
		return nil
	}
	op := operator.New(t, key, v, r)
	op.IID = iid
	if it := this.IType(op.IID); it != nil {
		op.IType = it.ID()
	}
	return op
}
