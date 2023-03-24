package updater

import "github.com/hwcer/updater/v2/dirty"

type hashModel interface {
	New(u *Updater, symbol any) (map[int32]int64, error)                  //获取所有数据集
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

func (data hashData) Merge(src hashData) {
	for k, v := range src {
		data[k] = v
	}
}

// Hash HashMAP储存
type Hash struct {
	*statement
	keys    hashKeys
	model   hashModel
	symbol  any      //标记时效性
	dirty   hashData //需要写入数据的数据
	dataset hashData //数据集
}

func NewHash(u *Updater, model any, ram RAMType) Handle {
	r := &Hash{}
	r.model = model.(hashModel)
	r.statement = NewStatement(u, ram, r.Operator)
	return r
}

// has 检查k是否已经缓存,或者下次会被缓存
func (this *Hash) has(key int32) (r bool) {
	if this.statement.ram == RAMTypeAlways || (this.keys != nil && this.keys[key]) {
		return true
	}
	if this.dirty != nil {
		if _, r = this.dirty[key]; r {
			return
		}
	}
	if this.dataset != nil {
		if _, r = this.dataset[key]; r {
			return
		}
	}
	return
}
func (this *Hash) get(k int32) (r int64) {
	if this.dirty != nil {
		var ok bool
		if r, ok = this.dirty[k]; ok {
			return
		}
	}
	return this.dataset[k]
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
	this.keys = hashKeys{}
	this.statement.reset()
	if s := this.model.Symbol(this.Updater); s != this.symbol {
		this.symbol = s
		this.dirty = nil
		this.dataset = nil
	}
	if this.dirty == nil {
		this.dirty = hashData{}
	}
	if this.dataset == nil {
		if this.statement.ram == RAMTypeAlways {
			this.dataset, this.Error = this.model.New(this.statement.Updater, this.symbol)
		} else {
			this.dataset = hashData{}
		}
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

// 关闭时执行,玩家下线
func (this *Hash) destruct() (err error) {
	return this.save()
}

func (this *Hash) Get(k any) (r any) {
	if id, _ := k.(int32); id > 0 {
		r = this.get(id)
	}
	return
}
func (this *Hash) Val(k any) (r int64) {
	if id, _ := k.(int32); id > 0 {
		r = this.get(id)
	}
	return
}

// Select 指定需要更新的字段
func (this *Hash) Select(keys ...any) {
	if this.statement.ram == RAMTypeAlways {
		return
	}
	for _, k := range keys {
		if id, _ := k.(int32); id > 0 && !this.has(id) {
			this.keys[id] = true
		}
	}
}

func (this *Hash) Data() error {
	if this.Error != nil {
		return this.Error
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

func (this *Hash) Verify() (err error) {
	if this.Error != nil {
		return this.Error
	}
	if len(this.statement.operator) == 0 {
		return
	}
	for _, act := range this.statement.operator {
		if err = this.Parse(act); err != nil {
			return
		}
		if act.Effective() && act.Operator.IsValid() {
			this.dirty[act.IID] = act.Result.(int64)
		}
	}
	return
}

func (this *Hash) Save() (err error) {
	if this.Error != nil {
		return this.Error
	}
	for _, act := range this.statement.operator {
		if act.Operator.IsValid() {
			v := act.Result.(int64)
			this.dirty[act.IID] = v
			this.dataset[act.IID] = v
		}
	}
	this.statement.done()
	return this.save()
}

func (this *Hash) Operator(t dirty.Operator, k any, v any) {
	id, _ := k.(int32)
	if id <= 0 {
		this.statement.Errorf("updater Map Operator key must int32:%v", k)
		return
	}
	cache := dirty.NewCache(t, v)
	cache.IID = id
	if !this.has(id) {
		this.keys[id] = true
	}
	if it := this.Updater.IType(cache.IID); it != nil {
		if listener, ok := it.(ITypeListener); ok {
			if this.Error = listener.Listener(this.statement.Updater, cache); this.Error != nil {
				return
			}
		}
	}
	this.statement.Operator(cache)
	//if this.update != nil {
	//	_ = this.Verify()
	//}
}
