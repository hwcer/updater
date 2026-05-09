package updater

import (
	"github.com/hwcer/logger"
	"github.com/hwcer/updater/dataset"
	"github.com/hwcer/updater/operator"
)

type mappingModel interface {
	Has(u *Updater, k any) bool
	Get(u *Updater, k any) (r any)
	Val(u *Updater, k any) (r int64)
	Update(u *Updater, k, v any)
	Select(u *Updater, keys ...any)
	Reload(u *Updater) error
}

// Mapping 数字型键值对
type Mapping struct {
	statement
	name  string //model database name
	model mappingModel
}

func NewMapping(u *Updater, m *Model) Handle {
	r := &Mapping{}
	r.name = m.name
	r.model = m.model.(mappingModel)
	r.statement = *newStatement(u, m, r.operator, r.Has)
	return r
}

func (this *Mapping) Parser() Parser {
	return ParserTypeMapping
}

func (this *Mapping) reload() error {
	return this.model.Reload(this.Updater)
}

func (this *Mapping) loading() error {
	return nil
}

func (this *Mapping) save() (err error) {
	return
}

// reset 运行时开始时
func (this *Mapping) reset() {
	this.statement.reset()
	if reset, ok := this.model.(ModelReset); ok {
		if reset.Reset(this.Updater, this.Updater.last) {
			this.Updater.Error = this.reload()
		}
	}
}

// release 运行时释放
func (this *Mapping) release() {
	this.statement.release()
}

// 关闭时执行,玩家下线
func (this *Mapping) destroy() (err error) {
	return this.save()
}

func (this *Mapping) Has(k any) bool {
	return this.model.Has(this.Updater, k)
}

func (this *Mapping) Get(k any) (r any) {
	return this.model.Get(this.Updater, k)
}
func (this *Mapping) Val(k any) (r int64) {
	return this.model.Val(this.Updater, k)
}

// Set 设置
// Set(k int32,v int64)
func (this *Mapping) Set(k any, v ...any) {
	switch len(v) {
	case 1:
		this.operator(operator.TypesSet, k, 0, dataset.ParseInt64(v[0]))
	default:
		this.Updater.Error = ErrArgsIllegal(k, v)
	}
}

// Select 指定需要从数据库更新的字段
func (this *Mapping) Select(keys ...any) {
	this.model.Select(this.Updater, keys...)
}

func (this *Mapping) Data() (err error) {
	return
}

func (this *Mapping) verify() (err error) {
	if err = this.Updater.WriteAble(); err != nil {
		return
	}
	for _, act := range this.statement.operator {
		if err = this.Parse(act); err != nil {
			return
		}
	}
	this.statement.verify()
	return
}

func (this *Mapping) submit() (err error) {
	if err = this.Updater.WriteAble(); err != nil {
		return
	}
	this.statement.submit()
	return
}

func (this *Mapping) IType(iid int32) IType {
	if h, ok := this.model.(modelIType); ok {
		v := h.IType(iid)
		return itypesDict[v]
	}
	return this.Updater.IType(iid)
}

func (this *Mapping) operator(t operator.Types, k any, v int64, r any) {
	if err := this.Updater.WriteAble(); err != nil {
		return
	}
	op := operator.New(t, v, r)
	switch rk := k.(type) {
	case string:
		op.Key = rk
		this.statement.Select(op.Key)
	default:
		op.IID = dataset.ParseInt32(rk)
		this.statement.Select(op.IID)
	}

	it := this.IType(op.IID)
	if it == nil {
		logger.Debug("IType not exist:%v", op.IID)
		return
	}
	op.Mod = it.ID()
	if listen, ok := it.(ITypeListener); ok {
		listen.Listener(this.Updater, op)
	}
	this.statement.Operator(op)
}
