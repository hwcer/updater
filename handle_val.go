package updater

import (
	"encoding/json"

	"github.com/hwcer/logger"
	"github.com/hwcer/updater/dataset"
	"github.com/hwcer/updater/operator"
)

type valuesModel interface {
	IType(iid int32) int32
	Getter(u *Updater, data *dataset.Values, keys []int32) (err error) //获取数据接口
	Setter(u *Updater, data dataset.Data) error                        //保存数据接口
}

// Values 数字型键值对
type Values struct {
	statement
	name    string //model database name
	model   valuesModel
	setter  dataset.Data //需要写入数据的数据
	dataset *dataset.Values
}

func NewValues(u *Updater, m *Model) Handle {
	r := &Values{}
	r.name = m.name
	r.model = m.model.(valuesModel)
	r.statement = *newStatement(u, m, r.Has)
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
	if this.Updater.Error != nil {
		return this.Updater.Error
	}
	if len(this.keys) == 0 {
		return nil
	}
	keys := this.keys.ToInt32()
	if err = this.model.Getter(this.statement.Updater, this.dataset, keys); err == nil {
		this.statement.date()
	}
	return
}

func (this *Values) IType(iid int32) IType {
	it := this.model.IType(iid)
	if it == 0 {
		return nil
	}
	return itypesDict[it]
}

// Select 指定需要从数据库更新的字段
func (this *Values) Select(keys ...any) {
	if this.ram == RAMTypeAlways {
		return
	}
	for _, k := range keys {
		if iid, ok := dataset.TryParseInt32(k); ok {
			this.statement.Select(iid)
		}
	}
}

func (this *Values) Parser() Parser {
	return ParserTypeValues
}

// ===================== Handle 接口私有方法 =====================

func (this *Values) increase(k int32, v int64) {
	this.operator(operator.TypesAdd, k, v)
}
func (this *Values) decrease(k int32, v int64) {
	this.operator(operator.TypesSub, k, v)
}

func (this *Values) save() (err error) {
	if this.setter == nil {
		this.setter = dataset.Data{}
	}
	this.dataset.Save(this.setter)
	if len(this.setter) == 0 {
		this.setter = nil
		return nil
	}
	if err = this.model.Setter(this.statement.Updater, this.setter); err == nil {
		this.setter = nil
	} else {
		ds, _ := json.Marshal(this.setter)
		logger.Alert("database save error,uid:%s,Collection:%s\nOperation:%s\nerror:%s", this.Updater.Uid(), this.name, ds, err.Error())
		var s bool
		if s, err = onSaveErrorHandle(this.Updater, err); !s {
			this.setter = nil
		}
	}
	return
}

func (this *Values) reset() {
	this.statement.reset()
	if this.dataset == nil {
		this.dataset = dataset.NewValues()
	}
	if reset, ok := this.model.(ModelReset); ok {
		if reset.Reset(this.Updater, this.Updater.last) {
			this.Updater.Error = this.reload()
		}
	}
}

func (this *Values) reload() error {
	this.dataset = nil
	this.statement.reload()
	return this.loading()
}

func (this *Values) loading() error {
	if this.dataset == nil {
		this.dataset = dataset.NewValues()
	}
	if this.statement.loading() {
		if this.Updater.Error = this.model.Getter(this.Updater, this.dataset, nil); this.Updater.Error == nil {
			this.statement.loader = true
		}
	}
	return this.Updater.Error
}

func (this *Values) release() {
	this.statement.release()
	if this.statement.ram == RAMTypeNone {
		this.dataset = nil
	} else {
		this.dataset.Release()
	}
}

func (this *Values) destroy() (err error) {
	return this.save()
}

func (this *Values) submit() (err error) {
	if err = this.Updater.WriteAble(); err != nil {
		return
	}
	this.statement.submit()
	if err = this.save(); err != nil && this.ram != RAMTypeNone {
		logger.Alert("数据库[%v]同步数据错误,等待下次同步:%v", this.name, err)
		err = nil
	}
	return
}

func (this *Values) verify() (err error) {
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

func (this *Values) Delete(k int32) *operator.Operator {
	return this.operator(operator.TypesDel, k, 0)
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
	this.statement.insert(op, before...)
}

// ===================== 类型特有私有方法 =====================

func (this *Values) operator(t operator.Types, k int32, v int64) *operator.Operator {
	if err := this.Updater.WriteAble(); err != nil {
		return nil
	}

	op := operator.New(t, "", v, nil)
	op.IID = k
	this.statement.Select(k)
	it := this.IType(op.IID)
	if it == nil {
		logger.Debug("IType not exist:%v", op.IID)
		return nil
	}
	op.IType = it.ID()
	if listen, ok := it.(ITypeListener); ok {
		listen.Listener(this.Updater, op)
	}
	this.statement.insert(op)
	return op
}
