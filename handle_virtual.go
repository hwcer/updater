package updater

import (
	"github.com/hwcer/updater/dataset"
	"github.com/hwcer/updater/operator"
)

type VirtualModel interface {
	Has(u *Updater, k any) bool
	Get(u *Updater, k any) (r any)
	IType(int32) int32
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
func (this *Virtual) Val(k any) (r int64) {
	return dataset.ParseInt64(this.model.Get(this.Updater, k))
}

func (this *Virtual) Data() (err error) {
	return
}

// IType iid>0 时按 iid 查找，iid==0 时返回模型默认 IType
func (this *Virtual) IType(iid int32) IType {
	it := this.model.IType(iid)
	if it == 0 {
		return nil
	}
	return itypesDict[it]
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
	d := this.Val(k)
	iid, key, ok := this.key(k)
	if !ok {
		_ = this.Updater.Errorf("Virtual Add Args Error,name:%s,key:%v", this.name, k)
		return
	}

	op := this.newOperator(operator.TypesAdd, iid, key, value, map[string]any{key: d + value})
	this.model.Update(this.Updater, op)
	this.statement.insert(op)
}

func (this *Virtual) Sub(k any, v any) {
	value := dataset.ParseInt64(v)
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
	this.model.Update(this.Updater, op)
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
	this.statement.insert(op)
}

func (this *Virtual) Has(k any) bool {
	return this.model.Has(this.Updater, k)
}

// ===================== 类型特有私有方法 =====================

func (this *Virtual) newOperator(t operator.Types, iid int32, key string, v int64, r any) *operator.Operator {
	op := operator.New(t, key, v, r)
	op.IID = iid
	if it := this.IType(op.IID); it != nil {
		op.IType = it.ID()
	}
	return op
}
