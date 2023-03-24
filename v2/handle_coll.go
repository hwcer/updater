package updater

import (
	"github.com/hwcer/cosgo/logger"
	"github.com/hwcer/cosmo"
	"github.com/hwcer/updater/v2/dataset"
	"github.com/hwcer/updater/v2/dirty"
)

type Receive func(oid string, doc any)

type collectionModel interface {
	New(update *Updater, fn Receive) error //获取所有列表
	Getter(adapter *Updater, keys []string, fn Receive) error
	Setter(adapter *Updater, bulkWrite *cosmo.BulkWrite) error
	BulkWrite(adapter *Updater) *cosmo.BulkWrite
}

type Collection struct {
	*statement
	keys    documentKeys
	model   collectionModel
	dirty   dirty.Dirty
	Dataset dataset.Collection
}

func NewCollection(u *Updater, model any, ram RAMType) Handle {
	r := &Collection{}
	r.model = model.(collectionModel)
	r.statement = NewStatement(u, ram, r.Operator)
	return r
}

// Has 查询key(DBName)是否已经初始化
func (this *Collection) has(key string) (r bool) {
	if this.statement.ram == RAMTypeAlways || (this.keys != nil && this.keys[key]) {
		return true
	}
	if this.dirty != nil && this.dirty.Has(key) {
		return true
	}
	if this.Dataset != nil && this.Dataset.Has(key) {
		return true
	}
	return false
}

//	func (this *Collection) set(src any, k string, v any) error {
//		if i, ok := src.(documentSet); ok {
//			return i.Set(k, v)
//		}
//		sch, err := schema.Parse(src)
//		if err != nil {
//			return err
//		}
//		if err = sch.SetValue(this.dataset, k, v); err != nil {
//			return err
//		}
//		logger.Debug("建议给%v添加Set接口提升性能", sch.Name)
//		return nil
//	}
func (this *Collection) get(k string) (r *dataset.Document) {
	//if this.dirty != nil {
	//	var ok bool
	//	if r, ok = this.dirty[k]; ok {
	//		return //执行过程中修改
	//	}
	//}
	return this.Dataset.Get(k)
}

//	func (this *Document) val(k string) (r int64) {
//		if v := this.get(k); v != nil {
//			r = ParseInt64(v)
//		}
//		return
//	}
func (this *Collection) save() (err error) {
	if len(this.dirty) == 0 {
		return
	}
	bulkWrite := this.model.BulkWrite(this.Updater)
	if err = this.model.Setter(this.statement.Updater, this.dirty.BulkWrite(bulkWrite)); err == nil {
		this.dirty = nil
	}
	return
}

func (this *Collection) reset() {
	this.keys = documentKeys{}
	this.statement.reset()
	if this.dirty == nil {
		this.dirty = dirty.New()
	}
	if this.Dataset == nil {
		this.Dataset = dataset.New()
		if this.statement.ram == RAMTypeAlways {
			this.Error = this.model.New(this.statement.Updater, this.Receive)
		}
	}
}

func (this *Collection) release() {
	this.keys = nil
	this.statement.release()
	if this.statement.ram == RAMTypeNone {
		this.dirty = nil
		this.Dataset = nil
	}
}

// 关闭时执行,玩家下线
func (this *Collection) destruct() (err error) {
	return this.save()
}

// Get 返回item,不可叠加道具只能使用oid获取
func (this *Collection) Get(key any) (r any) {
	if oid, err := this.Updater.CreateId(key); err == nil {
		if i := this.get(oid); i != nil {
			r = i.Interface()
		}
	} else {
		logger.Debug(err)
	}
	return
}

// Val 直接获取 item中的val值
func (this *Collection) Val(key any) (r int64) {
	if oid, err := this.Updater.CreateId(key); err == nil {
		if i := this.get(oid); i != nil {
			r = i.VAL()
		}
	}
	return
}

func (this *Collection) Select(keys ...any) {
	if this.ram == RAMTypeAlways {
		return
	}
	for _, k := range keys {
		if oid, err := this.Updater.CreateId(k); err == nil && !this.has(oid) {
			this.keys[oid] = true
		} else {
			logger.Debug(err)
		}
	}
}

//func (this *Collection) act(cache *Cache) {
//	if this.Error != nil {
//		return
//	}
//	if cache.AType.MustSelect() {
//		this.base.Select(cache.OID)
//	}
//	it := cache.GetIType()
//	if listener, ok := it.(ITypeListener); ok {
//		if this.Error = listener.Listener(this.Adapter, cache); this.Error != nil {
//			return
//		}
//	}
//	this.base.Act(cache)
//	if this.bulkWrite != nil {
//		_ = this.Verify()
//	}
//}

func (this *Collection) Data() (err error) {
	if this.Error != nil {
		return this.Error
	}
	if len(this.keys) == 0 {
		return nil
	}
	keys := this.keys.Keys()
	err = this.model.Getter(this.Updater, keys, this.Receive)
	this.keys = nil
	return
}

func (this *Collection) Verify() (err error) {
	if this.Error != nil {
		return this.Error
	}
	for _, act := range this.statement.operator {
		if err = this.Parse(act); err != nil {
			return
		}
		if act.Effective() && act.Operator.IsValid() {
			//this.dirty[act.OID] = act.Ret //todo 修改内存
		}
	}
	return
}

func (this *Collection) Save() (err error) {
	if this.Error != nil {
		return this.Error
	}
	//同步到内存
	for _, act := range this.statement.operator {

		if act.Operator.IsValid() {
			if e := this.set(act.Field, act.Ret); e != nil {
				logger.Debug("数据保存失败可能是类型不匹配已经丢弃,table:%v,key:%v,val:%v", this.schema.Table, act.Field, act.Ret)
			} else {
				this.dirty.Parse(act.Operator, act.OID, act.Field, act.Value)
			}
		}
	}
	this.statement.done()
	if err = this.save(); err != nil && this.ram != RAMTypeNone {
		logger.Warn("同步数据失败,等待下次同步:%v", err)
		err = nil
	}
	return
}

func (this *Collection) Operator(t dirty.Operator, k any, v any) {
	cache := dirty.NewCache(t, v)

	//del set 使用oid,iid,使用iid时,必须可以无限叠加,具有唯一OID
	if t == dirty.OperatorTypeDel || t == dirty.OperatorTypeSet {
		cache.OID, cache.IID, this.Error = this.Updater.ObjectId(k)
	} else {
		cache.IID = ParseInt32(k)
	}
	if this.Error != nil || cache.IID <= 0 {
		return
	}

	if t == dirty.OperatorTypeSet {
		if d := dirty.ParseValue(v); d != nil {
			cache.Value = d
		} else {
			logger.Warn("Collection.Set参数错误;k:%v,v:%v", k, v)
			return
		}
	} else if t != dirty.OperatorTypeDel {
		cache.Field = dataset.ItemNameVAL
		cache.Value = v
	}
	//if cache.OID, this.Error = this.Updater.CreateId(cache.IID); this.Error != nil {
	//	return
	//}
	if it := this.Updater.IType(cache.IID); it != nil {
		if listener, ok := it.(ITypeListener); ok {
			if this.Error = listener.Listener(this.Updater, cache); this.Error != nil {
				return
			}
		}
	}
	this.statement.Operator(cache)
}

// Receive 接收业务逻辑层数据
func (this *Collection) Receive(id string, data any) {
	this.Dataset.Set(id, data)
}

func (this *Collection) verify(cache *dirty.Cache) (err error) {
	defer func() {
		if err == nil {
			this.base.cache = append(this.base.cache, cache)
		} else {
			this.bulkWrite = nil
			this.base.Error = err
		}
	}()
	val := ParseInt64(cache.Val)
	//检查扣除道具时数量是否足够
	if this.Adapter.strict && cache.AType == ActTypeSub {
		dv := this.Val(cache.OID)
		if dv < val {
			return ErrItemNotEnough(cache.IID, val, dv)
		}
	}
	it := cache.GetIType()
	//溢出判定
	if cache.AType == ActTypeAdd || cache.AType == ActTypeNew {
		num := this.Count(cache.IID)
		tot := val + num
		imax := Config.IMax(cache.IID)

		if imax > 0 && tot > imax {
			overflow := tot - imax
			if overflow > val {
				overflow = val //imax有改动
			}
			val -= overflow
			cache.Val = val
			if resolve, ok := it.(ITypeResolve); ok {
				if err = resolve.Resolve(this.Adapter, cache); err != nil {
					return
				} else {
					overflow = 0
				}
			}
			if overflow > 0 {
				//this.Adapter.overflow[cache.IID] += overflow
			}
		}
		if val == 0 {
			cache.AType = ActTypeResolve
		}
	}
	return parseCollection(this, cache)
}
