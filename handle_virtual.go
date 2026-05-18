package updater

import (
	"github.com/hwcer/updater/dataset"
	"github.com/hwcer/updater/operator"
)

type virtualModel interface {
	Has(u *Updater, k any) bool
	Get(u *Updater, k any) (r any)
	IType() int32
	Update(u *Updater, op *operator.Operator)
	Select(u *Updater, keys ...any)
	Reload(u *Updater) error
}

type virtualNotify interface {
	Notify() bool
}

// Virtual 虚拟数据层,本身不存储数据，操作委托给其他模块
type Virtual struct {
	statement
	name  string //model database name
	model virtualModel
}

func NewVirtual(u *Updater, m *Model) Handle {
	r := &Virtual{}
	r.name = m.name
	r.model = m.model.(virtualModel)
	r.statement = *newStatement(u, m, r.Has)
	return r
}

// ===================== Handle 接口公开方法 =====================

func (this *Virtual) Get(k any) (r any) {
	return this.model.Get(this.Updater, k)
}
func (this *Virtual) Val(k any) (r int64) {
	return dataset.ParseInt64(this.model.Get(this.Updater, k))
}

func (this *Virtual) Data() (err error) {
	return
}

// IType iid>0 时按 iid 查找，iid==0 时返回模型默认 IType
func (this *Virtual) IType(iid int32) IType {
	if iid > 0 {
		return this.Updater.IType(iid)
	}
	v := this.model.IType()
	return itypesDict[v]
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
	if reset, ok := this.model.(ModelReset); ok {
		if reset.Reset(this.Updater, this.Updater.last) {
			this.Updater.Error = this.reload()
		}
	}
}

func (this *Virtual) reload() error {
	return this.model.Reload(this.Updater)
}

func (this *Virtual) loading() error {
	return nil
}

func (this *Virtual) release() {
	this.statement.release()
}

func (this *Virtual) destroy() (err error) {
	return nil
}

// verify/submit 仅在 Notify 开启时生效，将操作记录推入 Updater.dirty 返回前端
func (this *Virtual) submit() (err error) {
	if this.Notify() {
		this.statement.submit()
	}
	return
}

func (this *Virtual) verify() (err error) {
	if this.Notify() {
		this.statement.verify()
	}
	return
}

// ===================== 类型特有公开方法 =====================

func (this *Virtual) Add(k any, v any) {
	value := dataset.ParseInt64(v)
	d := this.Val(k)
	op := this.newOperator(operator.TypesAdd, k, value, map[any]any{k: d + value})
	this.model.Update(this.Updater, op)
	if this.Notify() {
		this.statement.insert(op)
	} else {
		op.Release()
	}
}

func (this *Virtual) Sub(k any, v any) {
	value := dataset.ParseInt64(v)
	d := this.Val(k)
	if d < value && !this.Updater.CreditAllowed {
		this.Updater.Error = ErrItemNotEnough(dataset.ParseInt32(k), value, d)
		return
	}
	op := this.newOperator(operator.TypesSub, k, value, map[any]any{k: d - value})
	this.model.Update(this.Updater, op)
	if this.Notify() {
		this.statement.insert(op)
	} else {
		op.Release()
	}
}

func (this *Virtual) Set(k any, v any) {
	op := this.newOperator(operator.TypesSet, k, 0, map[any]any{k: v})
	this.model.Update(this.Updater, op)
	if this.Notify() {
		this.statement.insert(op)
	} else {
		op.Release()
	}
}

func (this *Virtual) Has(k any) bool {
	return this.model.Has(this.Updater, k)
}

func (this *Virtual) Notify() bool {
	if f, ok := this.model.(virtualNotify); ok {
		return f.Notify()
	}
	return true
}

// ===================== 类型特有私有方法 =====================

func (this *Virtual) newOperator(t operator.Types, k any, v int64, r any) *operator.Operator {
	op := operator.New(t, "", v, r)
	switch s := k.(type) {
	case string:
		op.Field = s
	default:
		op.IID = dataset.ParseInt32(k)
	}
	if it := this.IType(op.IID); it != nil {
		op.IType = it.ID()
	}
	return op
}
