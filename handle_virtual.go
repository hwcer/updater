package updater

import (
	"github.com/hwcer/updater/dataset"
	"github.com/hwcer/updater/operator"
)

type virtualModel interface {
	Has(u *Updater, k any) bool
	Get(u *Updater, k any) (r any)
	Val(u *Updater, k any) (r int64)

	Add(u *Updater, k any, v int64)
	Sub(u *Updater, k any, v int64)
	Set(u *Updater, k any, v any)

	IType() int32
	Select(u *Updater, keys ...any)
	Reload(u *Updater) error
}

// Virtual 虚拟数据层,本身不存储数据，操作委托给其他模块
type Virtual struct {
	statement
	name    string //model database name
	model   virtualModel
	forward bool // 是否将操作记录发送给前端，默认关闭
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
	return this.model.Val(this.Updater, k)
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

// verify/submit 仅在 forward 开启时生效，将操作记录推入 Updater.dirty 返回前端
func (this *Virtual) submit() (err error) {
	if this.forward {
		this.statement.submit()
	}
	return
}

func (this *Virtual) verify() (err error) {
	if this.forward {
		this.statement.verify()
	}
	return
}

// ===================== 类型特有公开方法 =====================

func (this *Virtual) Add(k any, v any) {
	value := dataset.ParseInt64(v)
	this.model.Add(this.Updater, k, value)
	r := map[any]any{k: value}
	this.operator(operator.TypesAdd, k, value, r)
}

func (this *Virtual) Sub(k any, v any) {
	value := dataset.ParseInt64(v)
	this.model.Sub(this.Updater, k, value)
	r := map[any]any{k: value}
	this.operator(operator.TypesSub, k, value, r)
}

func (this *Virtual) Set(k any, v any) {
	this.model.Set(this.Updater, k, v)
	r := map[any]any{k: v}
	this.operator(operator.TypesSet, k, 0, r)
}

func (this *Virtual) Has(k any) bool {
	return this.model.Has(this.Updater, k)
}

func (this *Virtual) Forward(v bool) {
	this.forward = v
}

// ===================== 类型特有私有方法 =====================

// operator 当 forward 开启时，将操作记录到 statement 用于返回给前端
// key 为 string 时存入 Field，为 int32 时存入 IID
func (this *Virtual) operator(t operator.Types, k any, v int64, r any) {
	if !this.forward {
		return
	}
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
	this.statement.insert(op)
}
