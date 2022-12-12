package updater

import (
	"errors"
	"fmt"
	"github.com/hwcer/cosgo/logger"
	"github.com/hwcer/cosmo/update"
	"github.com/hwcer/updater/bson"
	"reflect"
)

// 无限叠加的道具
var collectionParseHandle = make(map[ActType]func(*Collection, *Cache) error)

func init() {
	collectionParseHandle[ActTypeNew] = collectionHandleNew
	collectionParseHandle[ActTypeAdd] = collectionHandleAdd
	collectionParseHandle[ActTypeSub] = collectionHandleSub
	collectionParseHandle[ActTypeSet] = collectionHandleSet
	collectionParseHandle[ActTypeDel] = collectionHandleDel
	collectionParseHandle[ActTypeMax] = collectionHandleMax
	collectionParseHandle[ActTypeMin] = collectionHandleMin
	collectionParseHandle[ActTypeResolve] = collectionHandleResolve
}

func parseCollection(this *Collection, act *Cache) (err error) {
	f, ok := collectionParseHandle[act.AType]
	if !ok {
		return fmt.Errorf("collectionParseHandle not exist:%v", act.AType)
	}
	return f(this, act)
}

// hmapHandleResolve 仅仅标记不做任何处理
func collectionHandleResolve(coll *Collection, act *Cache) error {
	return nil
}

func collectionHandleDel(coll *Collection, act *Cache) error {
	coll.Del(act.OID)
	act.AType = ActTypeDel
	bulkWrite := coll.BulkWrite()
	bulkWrite.Delete(act.OID)
	return nil
}

func collectionHandleNew(coll *Collection, cache *Cache) error {
	if it := cache.GetIType(); !it.Unique() {
		return collectionHandleNewMultiple(coll, cache)
	}
	v, doc, err := collectionHandleNewUnique(coll, cache)
	if err != nil {
		return err
	}
	bulkWrite := coll.BulkWrite()
	cache.AType = ActTypeNew
	cache.OID = doc.GetString(ItemNameOID)
	cache.Ret = v
	bulkWrite.Insert(v)
	return nil
}

func collectionHandleAdd(coll *Collection, cache *Cache) (err error) {
	doc := coll.Collection.Get(cache.OID)
	if doc == nil {
		return collectionHandleNew(coll, cache)
	}
	if cache.Ret, err = doc.Inc(cache.Key, cache.Val); err != nil {
		return
	}
	bulkWrite := coll.BulkWrite()
	upsert := update.Update{}
	upsert.Inc(cache.Key, cache.Val)
	bulkWrite.Update(upsert, cache.OID)
	return
}

func collectionHandleSub(coll *Collection, cache *Cache) (err error) {
	val := -bson.ParseInt64(cache.Val)
	doc := coll.Collection.Get(cache.OID)
	if doc == nil {
		cache.Val = val //扣成负数,必须Adapter.strict ==false
		err = collectionHandleNew(coll, cache)
	}
	if cache.Ret, err = doc.Inc(cache.Key, val); err != nil {
		return
	}
	bulkWrite := coll.BulkWrite()
	upsert := update.Update{}
	upsert.Inc(cache.Key, val)
	bulkWrite.Update(upsert, cache.OID)

	return
}

func collectionHandleMax(coll *Collection, cache *Cache) (err error) {
	doc := coll.Collection.Get(cache.OID)
	if doc == nil {
		return collectionHandleNew(coll, cache)
	}
	cache.Ret, err = doc.Max(cache.Key, cache.Val)
	if err == bson.ErrorNotChange {
		cache.AType = ActTypeDrop
	} else if err == nil {
		cache.AType = ActTypeSet
		bulkWrite := coll.BulkWrite()
		upsert := update.Update{}
		upsert.Set(cache.Key, cache.Ret)
		bulkWrite.Update(upsert, cache.OID)
	}
	return
}

func collectionHandleMin(coll *Collection, cache *Cache) (err error) {
	doc := coll.Collection.Get(cache.OID)
	if doc == nil {
		return collectionHandleNew(coll, cache)
	}
	cache.Ret, err = doc.Min(cache.Key, cache.Val)
	if err == bson.ErrorNotChange {
		cache.AType = ActTypeDrop
	} else if err == nil {
		cache.AType = ActTypeSet
		bulkWrite := coll.BulkWrite()
		upsert := update.Update{}
		upsert.Set(cache.Key, cache.Ret)
		bulkWrite.Update(upsert, cache.OID)
	}
	return
}

func collectionHandleSet(coll *Collection, cache *Cache) (err error) {
	doc := coll.Collection.Get(cache.OID)
	if doc == nil {
		return collectionHandleNew(coll, cache)
	}
	var values map[string]interface{}
	if cache.Key == CacheKeyWildcard {
		values, err = collectionHandleMultipleSet(doc, cache)
	} else {
		values, err = collectionHandleSingleSet(doc, cache)
	}
	if err != nil {
		return
	}
	cache.Ret = cache.Val
	bulkWrite := coll.BulkWrite()
	upsert := update.Update{}
	for k, v := range values {
		upsert.Set(k, v)
	}
	bulkWrite.Update(upsert, cache.OID)
	return
}

func collectionHandleNewUnique(coll *Collection, cache *Cache) (v interface{}, doc bson.Document, err error) {
	v, err = cache.IType.New(coll.Adapter, cache)
	if err != nil {
		return
	}
	if doc, err = coll.Collection.Insert(v); err != nil {
		logger.Info("collectionHandleNewUnique:%v", err)
		return
	}
	return
}
func collectionHandleNewMultiple(coll *Collection, cache *Cache) error {
	v := bson.ParseInt64(cache.Val)
	var newItem []any
	for i := int64(1); i <= v; i++ {
		if d, _, err := collectionHandleNewUnique(coll, cache); err != nil {
			return err
		} else {
			newItem = append(newItem, d)
		}
	}
	cache.Ret = newItem
	cache.AType = ActTypeNew
	bulkWrite := coll.BulkWrite()
	bulkWrite.Insert(newItem...)
	return nil
}

func collectionHandleSingleSet(doc bson.Document, cache *Cache) (r map[string]interface{}, err error) {
	if err = doc.Set(cache.Key, cache.Val); err == nil {
		r = map[string]interface{}{}
		r[cache.Key] = cache.Val
		return
	}
	return
}

func collectionHandleMultipleSet(doc bson.Document, act *Cache) (r map[string]interface{}, err error) {
	vf := reflect.Indirect(reflect.ValueOf(act.Val))
	if vf.Kind() != reflect.Map {
		return nil, errors.New(" act.val's Kind is not Map")
	}
	r = map[string]interface{}{}
	for _, k := range vf.MapKeys() {
		if k.Kind() != reflect.String {
			return nil, errors.New("key's Kind is not String")
		}
		v := vf.MapIndex(k)
		mk, mv := k.String(), v.Interface()
		if err = doc.Set(mk, mv); err != nil {
			return
		}
		r[mk] = mv
	}
	return
}
