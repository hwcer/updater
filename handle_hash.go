package updater

import (
	"fmt"
	"github.com/hwcer/cosgo/schema"
	"github.com/hwcer/logger"
	"github.com/hwcer/updater/dataset"
	"github.com/hwcer/updater/operator"
)

type hashModel interface {
	Getter(u *Updater, data *HashDataset, keys []int32) (err error) //获取数据接口
	Setter(u *Updater, data *HashDataset) error                     //保存数据接口
}

//func (data hashData) Merge(src hashData) {
//	for k, v := range src {
//		data[k] = v
//	}
//}

// Hash HashMAP储存
type Hash struct {
	statement
	name    string //model database name
	model   hashModel
	dataset HashDataset
	//dirty   hashData //需要写入数据的数据
	//expire  int64    //数据集过期时间，0-永不过期
	//dataset hashData //数据集
}

func NewHash(u *Updater, model any, ram RAMType) Handle {
	r := &Hash{}
	r.model = model.(hashModel)
	r.statement = *newStatement(u, ram, r.operator, r.Has)
	if sch, err := schema.Parse(model); err == nil {
		r.name = sch.Table
	} else {
		logger.Fatal(err)
	}
	r.dataset = HashDataset{Values: map[int32]int64{}}
	return r
}

func (this *Hash) Parser() Parser {
	return ParserTypeHash
}

func (this *Hash) init() error {
	if this.statement.ram == RAMTypeMaybe || this.statement.ram == RAMTypeAlways {
		this.Updater.Error = this.model.Getter(this.Updater, &this.dataset, nil)
	}
	return this.Updater.Error
}

func (this *Hash) val(iid int32) (r int64, ok bool) {
	if r, ok = this.values.get(iid); ok {
		return
	} else {
		r, ok = this.dataset.Get(iid)
	}
	return
}

func (this *Hash) save() (err error) {
	if this.Updater.Async || len(this.dataset.dirty) == 0 {
		return
	}
	if err = this.model.Setter(this.statement.Updater, &this.dataset); err == nil {
		this.dataset.dirty = nil
	}
	return
}

// reset 运行时开始时
func (this *Hash) reset() {
	this.statement.reset()
	if this.dataset.Expire > 0 && this.dataset.Expire < this.Updater.Time.Unix() {
		if this.Updater.Error = this.save(); this.Updater.Error != nil {
			logger.Alert("保存数据失败,name:%v,data:%v\n%v", this.name, this.dataset, this.Updater.Error)
		} else {
			_ = this.init()
		}
	}
}

// release 运行时释放
func (this *Hash) release() {
	this.statement.release()
	if !this.Updater.Async && this.statement.ram == RAMTypeNone {
		this.dataset.release()
	}
}

// 关闭时执行,玩家下线
func (this *Hash) destroy() (err error) {
	return this.save()
}

func (this *Hash) Has(k any) bool {
	return this.dataset.Has(dataset.ParseInt32(k))
}

func (this *Hash) Get(k any) (r any) {
	if id, ok := dataset.TryParseInt32(k); ok {
		r, _ = this.dataset.Get(id)
	}
	return
}
func (this *Hash) Val(k any) (r int64) {
	if id, ok := dataset.TryParseInt32(k); ok {
		r, _ = this.val(id)
	}
	return
}

// Set 设置
// Set(k int32,v int64)
func (this *Hash) Set(k any, v ...any) {
	switch len(v) {
	case 1:
		this.operator(operator.TypesSet, k, 0, dataset.ParseInt64(v[0]))
	default:
		this.Updater.Error = ErrArgsIllegal(k, v)
	}
}

// Select 指定需要从数据库更新的字段
func (this *Hash) Select(keys ...any) {
	if this.ram == RAMTypeAlways {
		return
	}
	for _, k := range keys {
		if iid, ok := dataset.TryParseInt32(k); ok {
			this.statement.Select(iid)
		}
	}
}

func (this *Hash) Data() (err error) {
	if this.Updater.Error != nil {
		return this.Updater.Error
	}
	if len(this.keys) == 0 {
		return nil
	}
	keys := this.keys.ToInt32()
	if err = this.model.Getter(this.statement.Updater, &this.dataset, keys); err == nil {
		this.statement.date()
	}
	return
}

func (this *Hash) Verify() (err error) {
	if this.Updater.Error != nil {
		return this.Updater.Error
	}
	for _, act := range this.statement.operator {
		if err = this.Parse(act); err != nil {
			return
		}
	}
	this.statement.verify()
	return
}

func (this *Hash) submit() (err error) {
	//defer this.statement.done()
	if err = this.Updater.Error; err != nil {
		return
	}
	//r = this.statement.operator
	for _, op := range this.statement.cache {
		if op.Type.IsValid() {
			if v, ok := op.Result.(int64); ok {
				this.dataset.Dirty(op.IID)
				this.dataset.Set(op.IID, v)
			} else {
				fmt.Printf("hash save error:%+v\n", op)
			}
		}
	}
	this.statement.submit()
	if err = this.save(); err != nil && this.ram != RAMTypeNone {
		logger.Alert("数据库[%v]同步数据错误,等待下次同步:%v", this.name, err)
		err = nil
	}
	return
}

func (this *Hash) Range(f func(int32, int64) bool) {
	for k, v := range this.dataset.Values {
		if !f(k, v) {
			return
		}
	}
}

//func (this *Hash) data() map[int32]int64 {
//	return this.dataset
//}

func (this *Hash) IType(iid int32) IType {
	if h, ok := this.model.(modelIType); ok {
		v := h.IType(iid)
		return itypesDict[v]
	} else {
		return this.Updater.IType(iid)
	}
}

func (this *Hash) operator(t operator.Types, k any, v int64, r any) {
	id, ok := dataset.TryParseInt32(k)
	if !ok {
		_ = this.Errorf("updater Hash Operator key must int32:%v", k)
		return
	}
	if t != operator.TypesDel {
		if _, ok = dataset.TryParseInt64(v); !ok {
			_ = this.Errorf("updater Hash Operator val must int64:%v", v)
			return
		}
	}
	op := operator.New(t, v, r)
	op.IID = id
	this.statement.Select(id)
	it := this.IType(op.IID)
	if it == nil {
		logger.Debug("IType not exist:%v", op.IID)
		return
	}
	op.Bag = it.Id()
	if listen, ok := it.(ITypeListener); ok {
		listen.Listener(this.Updater, op)
	}
	this.statement.Operator(op)
}
