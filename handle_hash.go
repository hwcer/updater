package updater

import (
	"fmt"
	"github.com/hwcer/cosgo/schema"
	"github.com/hwcer/updater/dataset"
	"github.com/hwcer/updater/operator"
)

type hashModel interface {
	Symbol(u *Updater) any                                                //获取信息标识符号,如果和当前不一样将会重置数据
	Getter(u *Updater, symbol any, keys []int32) (map[int32]int64, error) //获取数据接口,返回 []byte(bson.raw) , bson.Document
	Setter(u *Updater, symbol any, update map[int32]int64) error          //保存数据接口
}

type hashData map[int32]int64
type hashKeys map[int32]bool

func (this hashKeys) Keys() (r []int32) {
	for k, _ := range this {
		r = append(r, k)
	}
	return
}
func (this hashKeys) Has(k int32) (r bool) {
	_, r = this[k]
	return
}

func (this hashData) Has(k int32) (r bool) {
	_, r = this[k]
	return
}
func (data hashData) Merge(src hashData) {
	for k, v := range src {
		data[k] = v
	}
}

// Hash HashMAP储存
type Hash struct {
	*statement
	name    string //model database name
	keys    hashKeys
	model   hashModel
	symbol  any      //标记时效性
	dirty   hashData //需要写入数据的数据
	dataset hashData //数据集
}

func NewHash(u *Updater, model any, ram RAMType) Handle {
	r := &Hash{}
	r.model = model.(hashModel)
	r.statement = NewStatement(u, ram, r.operator)
	if sch, err := schema.Parse(model); err == nil {
		r.name = sch.Table
	} else {
		Logger.Fatal(err)
	}
	return r
}

func (this *Hash) Parser() Parser {
	return ParserTypeHash
}

// has 检查k是否已经缓存,或者下次会被缓存
func (this *Hash) has(key int32) (r bool) {
	if this.ram == RAMTypeAlways {
		return true
	}
	if this.keys != nil && this.keys.Has(key) {
		return true
	}
	if this.dirty.Has(key) || this.dataset.Has(key) {
		return true
	}
	return
}

func (this *Hash) val(iid int32) (r int64) {
	if v, ok := this.values[iid]; ok {
		return v
	}
	r = this.dataset[iid]
	this.values[iid] = r
	return r
}

func (this *Hash) save() (err error) {
	if len(this.dirty) == 0 {
		return
	}
	if err = this.model.Setter(this.statement.Updater, this.symbol, this.dirty); err == nil {
		this.dirty = nil
	}
	return
}

// reset 运行时开始时
func (this *Hash) reset() {
	this.statement.reset()
	if s := this.model.Symbol(this.Updater); s != this.symbol {
		if err := this.save(); err != nil {
			Logger.Alert("保存数据失败,name:%v,data:%v\n%v", this.name, this.dirty, err)
		}
		this.symbol = s
		this.dirty = nil
		this.dataset = nil
	}
	if this.dirty == nil {
		this.dirty = hashData{}
	}
	if this.dataset == nil {
		this.dataset = hashData{}
	}
	if this.keys == nil && this.ram != RAMTypeAlways {
		this.keys = hashKeys{}
	}
}

// release 运行时释放
func (this *Hash) release() {
	this.keys = nil
	this.statement.release()
	if this.statement.ram == RAMTypeNone {
		this.dirty = nil
		this.dataset = nil
	}
}
func (this *Hash) init() error {
	this.symbol = this.model.Symbol(this.Updater)
	if this.statement.ram == RAMTypeMaybe || this.statement.ram == RAMTypeAlways {
		this.dataset, this.Updater.Error = this.model.Getter(this.Updater, this.symbol, nil)
	}
	return this.Updater.Error
}

// 关闭时执行,玩家下线
func (this *Hash) destroy() (err error) {
	return this.save()
}

func (this *Hash) Get(k any) (r any) {
	if id := dataset.ParseInt32(k); id > 0 {
		r = this.val(id)
	}
	return
}
func (this *Hash) Val(k any) (r int64) {
	if id := dataset.ParseInt32(k); id > 0 {
		r = this.val(id)
	}
	return
}

// Set 设置
// Set(k int32,v int64)
func (this *Hash) Set(k any, v ...any) {
	switch len(v) {
	case 1:
		this.operator(operator.TypesSet, k, 0, ParseInt64(v[0]))
	default:
		this.Updater.Error = ErrArgsIllegal(k, v)
	}
}

// Select 指定需要更新的字段
func (this *Hash) Select(keys ...any) {
	if this.statement.ram == RAMTypeAlways {
		return
	}
	for _, k := range keys {
		if id := dataset.ParseInt32(k); id > 0 && !this.has(id) {
			this.keys[id] = true
		}
	}
}

func (this *Hash) Data() error {
	if this.Updater.Error != nil {
		return this.Updater.Error
	}
	if len(this.keys) == 0 {
		return nil
	}
	keys := this.keys.Keys()
	if src, err := this.model.Getter(this.statement.Updater, this.symbol, keys); err != nil {
		return err
	} else if src != nil {
		this.dataset.Merge(src)
	}
	this.keys = nil
	return nil
}

func (this *Hash) verify() (err error) {
	if this.Updater.Error != nil {
		return this.Updater.Error
	}
	for _, act := range this.statement.operator {
		if err = this.Parse(act); err != nil {
			return
		}
	}
	this.statement.verified = true
	return
}

func (this *Hash) submit() (r []*operator.Operator, err error) {
	defer this.statement.done()
	if err = this.Updater.Error; err != nil {
		return
	}
	r = this.statement.operator
	for _, op := range r {
		if op.Type.IsValid() {
			if v, ok := op.Result.(int64); ok {
				this.dirty[op.IID] = v
				this.dataset[op.IID] = v
			} else {
				fmt.Printf("hash save error:%+v\n", op)
			}
		}
	}
	if err = this.save(); err != nil && this.ram != RAMTypeNone {
		Logger.Alert("数据库[%v]同步数据错误,等待下次同步:%v", this.name, err)
		err = nil
	}
	return
}

func (this *Hash) Range(f func(int32, int64) bool) {
	for k, v := range this.dataset {
		if !f(k, v) {
			return
		}
	}
}

func (this *Hash) Values() map[int32]int64 {
	return this.dataset
}

func (this *Hash) IType(iid int32) IType {
	if h, ok := this.model.(modelIType); ok {
		v := h.IType(iid)
		return itypesDict[v]
	} else {
		return this.Updater.IType(iid)
	}
}

func (this *Hash) operator(t operator.Types, k any, v int64, r any) {
	id, ok := TryParseInt32(k)
	if !ok {
		_ = this.Errorf("updater Hash Operator key must int32:%v", k)
		return
	}
	if t != operator.TypesDel {
		if _, ok = TryParseInt64(v); !ok {
			_ = this.Errorf("updater Hash Operator val must int64:%v", v)
			return
		}
	}
	op := operator.New(t, v, r)
	op.IID = id
	if !this.has(id) {
		this.keys[id] = true
		this.Updater.changed = true
	}

	it := this.IType(op.IID)
	if it == nil {
		Logger.Debug("IType not exist:%v", op.IID)
		return
	}
	op.Bag = it.Id()
	if listen, ok := it.(ITypeListener); ok {
		listen.Listener(this.Updater, op)
	}
	this.statement.Operator(op)
	if this.verified {
		this.Updater.Error = this.Parse(op)
	}
}
