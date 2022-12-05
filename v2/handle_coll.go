package updater

import (
	"github.com/hwcer/adapter/bson"
	"github.com/hwcer/cosmo/clause"
	"github.com/hwcer/logger"
)

type collectionModel interface {
	Getter(adapter *Updater, filter clause.Filter) ([]any, error)
	BulkWrite(adapter *Updater, model any) BulkWrite
}

type Collection struct {
	base
	bson.Collection
	model     collectionModel
	bulkWrite BulkWrite
}

func NewCollection(adapter *Updater, model any) Handle {
	r := &Collection{}
	r.base = *NewBase(adapter)
	r.model = model.(collectionModel)
	return r
}
func (this *Collection) reset() {
	this.base.reset()
	this.Collection = bson.Collection{}
}

func (this *Collection) release() {
	this.base.release()
	this.bulkWrite = nil
	this.Collection = nil
}

func (this *Collection) Sub(k ikey, v ival) {
	if cache, ok := this.createCache(ActTypeSub, k, v); ok && bson.ParseInt64(cache.Val) > 0 {
		this.act(cache)
	}
}

func (this *Collection) Max(k ikey, v ival) {
	if cache, ok := this.createCache(ActTypeMax, k, v); ok {
		this.act(cache)
	}
}

func (this *Collection) Min(k ikey, v ival) {
	if cache, ok := this.createCache(ActTypeMin, k, v); ok {
		this.act(cache)
	}
}

func (this *Collection) Add(k ikey, v ival) {
	if cache, ok := this.createCache(ActTypeAdd, k, v); ok && bson.ParseInt64(cache.Val) > 0 {
		if it := cache.GetIType(); !it.Unique() {
			cache.AType = ActTypeNew
		}
		this.act(cache)
	}
}

// Set id= iid||oid ,v=map[string]interface{}
// v类型为数字时，一律转换为Map{"val":v}
//
//	v=map[key]val 中key可以是使用"."操作符号(a.b.c =1)
func (this *Collection) Set(k ikey, v any) {
	if cache, ok := this.createCache(ActTypeSet, k, v); ok {
		this.act(cache)
	}
}

func (this *Collection) Del(k ikey) {
	if cache, ok := this.createCache(ActTypeMin, k, 0); ok {
		this.act(cache)
	}
}

// Get 返回bson.Document
func (this *Collection) Get(id ikey) any {
	return this.Document(id)
}

// Val 直接获取 item中的val值
func (this *Collection) Val(id ikey) (r int64) {
	if doc := this.Document(id); doc != nil {
		r = doc.GetInt64(ItemNameVAL)
	}
	return
}

func (this *Collection) Bind(k ikey, i any) error {
	id, err := this.Adapter.CreateId(k)
	if err != nil {
		return err
	}
	return this.Collection.Unmarshal(id, i)
}

func (this *Collection) Select(keys ...ikey) {
	for _, k := range keys {
		if id, err := this.Adapter.CreateId(k); err == nil {
			this.base.Select(id)
		}
	}
}

func (this *Collection) Document(k ikey) (doc bson.Document) {
	if id, err := this.Adapter.CreateId(k); err == nil {
		doc = this.Collection.Get(id)
	}
	return
}

func (this *Collection) act(cache *Cache) {
	if this.Error != nil {
		return
	}
	if cache.AType.MustSelect() {
		this.base.Select(cache.OID)
	}
	it := cache.GetIType()
	if listener, ok := it.(ITypeListener); ok {
		if this.Error = listener.Listener(this.Adapter, cache); this.Error != nil {
			return
		}
	}
	this.base.Act(cache)
	if this.bulkWrite != nil {
		_ = this.Verify()
	}
}

func (this *Collection) Data() error {
	if this.Error != nil {
		return this.Error
	}
	query := this.base.Fields.Query()
	if len(query) == 0 {
		return nil
	}
	defer this.base.Fields.done()

	rows, err := this.model.Getter(this.Adapter, query)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if _, err = this.Collection.Set(row, false); err != nil {
			return err
		}
	}
	return nil
}

func (this *Collection) Verify() (err error) {
	if this.Error != nil {
		return this.Error
	}
	defer func() {
		this.base.acts = nil
	}()
	_ = this.BulkWrite()
	if len(this.base.acts) == 0 {
		return nil
	}
	for _, act := range this.base.acts {
		if err = this.doAct(act); err != nil {
			return
		}
	}
	return nil
}

func (this *Collection) Save() (cache []*Cache, err error) {
	if this.Error != nil {
		return nil, this.Error
	}
	if this.bulkWrite == nil {
		return
	}
	defer func() {
		if this.Error == nil {
			this.cache = nil
		}
	}()
	if err == this.bulkWrite.Save() {
		cache = this.cache
	}
	return
}

// Count 统计iid数量之和(val累加)
func (this *Collection) Count(iid int32) (r int64) {
	this.Collection.Range(func(_ string, doc bson.Document) bool {
		if doc.GetInt32(ItemNameIID) == iid {
			r += doc.GetInt64(ItemNameVAL)
		}
		return true
	})
	return
}

func (this *Collection) BulkWrite() BulkWrite {
	if this.bulkWrite == nil {
		this.bulkWrite = this.model.BulkWrite(this.Adapter, this.model)
	}
	return this.bulkWrite
}

func (this *Collection) createCache(t ActType, k ikey, v ival) (cache *Cache, ok bool) {
	cache = &Cache{AType: t, Val: v}
	if bson.IsNumber(v) {
		cache.Key = ItemNameVAL
	} else {
		cache.Key = CacheKeyWildcard
	}

	defer func() {
		if this.Error != nil || cache.IID == 0 {
			ok = false
		} else {
			ok = true
		}
	}()
	if IsOID(k) {
		cache.OID = k.(string)
		cache.IID, this.Error = Config.ParseId(this.Adapter, cache.OID)
		return
	}
	cache.IID = bson.ParseInt32(k)
	if this.Error != nil || cache.IID == 0 {
		return
	}
	it := cache.GetIType()
	if it == nil {
		this.Error = ErrITypeNotExist(cache.IID)
		return
	}
	if it.Unique() {
		cache.OID, this.Error = this.Adapter.CreateId(cache.IID)
	} else if t != ActTypeAdd && t != ActTypeNew {
		this.Error = ErrActKeyIllegal(cache)
		logger.Warn("不可叠加道具只能使用OID进行%v操作:%v", t.String(), k)
	}
	return
}

func (this *Collection) doAct(cache *Cache) (err error) {
	defer func() {
		if err == nil {
			this.base.cache = append(this.base.cache, cache)
		} else {
			this.bulkWrite = nil
			this.base.Error = err
		}
	}()
	val := bson.ParseInt64(cache.Val)
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
				this.Adapter.overflow[cache.IID] += overflow
			}
		}
		if val == 0 {
			cache.AType = ActTypeResolve
		}
	}
	return parseCollection(this, cache)
}
