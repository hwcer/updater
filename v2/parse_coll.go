package updater

import (
	"fmt"
	"github.com/hwcer/updater/bson"
	"github.com/hwcer/updater/v2/dataset"
	"github.com/hwcer/updater/v2/dirty"
)

// 无限叠加的道具
var collectionParseHandle = make(map[dirty.Operator]func(*Collection, *dirty.Cache) error)

func init() {
	collectionParseHandle[dirty.OperatorTypeNew] = collectionHandleNew
	collectionParseHandle[dirty.OperatorTypeAdd] = collectionHandleAdd
	collectionParseHandle[dirty.OperatorTypeSub] = collectionHandleSub
	collectionParseHandle[dirty.OperatorTypeSet] = collectionHandleSet
	collectionParseHandle[dirty.OperatorTypeDel] = collectionHandleDel
	collectionParseHandle[dirty.OperatorTypeMax] = collectionHandleMax
	collectionParseHandle[dirty.OperatorTypeMin] = collectionHandleMin
	collectionParseHandle[dirty.OperatorTypeResolve] = collectionHandleResolve
}

func (this *Collection) Parse(act *dirty.Cache) (err error) {
	f, ok := collectionParseHandle[act.Operator]
	if !ok {
		return fmt.Errorf("collectionParseHandle not exist:%v", act.Operator)
	}
	return f(this, act)
}

// hmapHandleResolve 仅仅标记不做任何处理
func collectionHandleResolve(coll *Collection, act *dirty.Cache) error {
	return nil
}

func collectionHandleDel(coll *Collection, act *dirty.Cache) error {
	return nil
}

func collectionHandleNew(coll *Collection, cache *dirty.Cache) error {
	cache.Operator = dirty.OperatorTypeNew
	if it := coll.Updater.IType(cache.IID); !it.Unique() {
		return collectionHandleNewMultiple(coll, cache)
	} else {
		return collectionHandleNewUnique(coll, cache)
	}
}

func collectionHandleAdd(coll *Collection, cache *dirty.Cache) (err error) {
	d := coll.val(cache.IID)
	if d <= 0 {
		return collectionHandleNew(coll, cache)
	}
	r := ParseInt64(cache.Value) + d
	cache.Result = r
	coll.values[cache.IID] = r
	return
}

func collectionHandleSub(coll *Collection, cache *dirty.Cache) (err error) {
	d := coll.val(cache.IID)
	v := bson.ParseInt64(cache.Value)
	if v > d {
		if coll.Updater.tolerance {
			v = d
		} else {
			return ErrItemNotEnough(cache.IID, v, d)
		}
	}
	if d <= 0 {
		cache.Operator = dirty.OperatorTypeDrop
	} else {
		r := d - v
		cache.Result = r
		coll.values[cache.IID] = r
	}
	return
}

func collectionHandleSet(coll *Collection, cache *dirty.Cache) (err error) {
	d := coll.val(cache.IID)
	if d <= 0 {
		return collectionHandleNew(coll, cache)
	}
	cache.Result = cache.Value
	cache.Operator = dirty.OperatorTypeSet
	update := cache.Update()
	if v, ok := update[dataset.ItemNameVAL]; ok {
		coll.values[cache.IID] = ParseInt64(v)
	}
	return
}

func collectionHandleMax(coll *Collection, cache *dirty.Cache) (err error) {
	if d, v := coll.val(cache.IID), ParseInt64(cache.Value); v > d {
		err = collectionHandleSet(coll, cache)
	} else {
		cache.Operator = dirty.OperatorTypeDrop
	}
	return
}

func collectionHandleMin(coll *Collection, cache *dirty.Cache) (err error) {
	if d, v := coll.val(cache.IID), ParseInt64(cache.Value); v < d {
		err = collectionHandleSet(coll, cache)
	} else {
		cache.Operator = dirty.OperatorTypeDrop
	}
	return
}

func collectionHandleNewUnique(coll *Collection, cache *dirty.Cache) error {
	it := coll.Updater.IType(cache.IID)
	if it == nil {
		return ErrITypeNotExist(cache.IID)
	}
	if d, err := it.New(coll.Updater, cache); err == nil {
		cache.Result = []any{d}
		coll.values[cache.IID] = ParseInt64(cache.Value)
		return nil
	} else {
		return err
	}
}
func collectionHandleNewMultiple(coll *Collection, cache *dirty.Cache) error {
	it := coll.Updater.IType(cache.IID)
	if it == nil {
		return ErrITypeNotExist(cache.IID)
	}
	v := ParseInt64(cache.Value)
	var newItem []any
	for i := int64(1); i <= v; i++ {
		if d, err := it.New(coll.Updater, cache); err == nil {
			newItem = append(newItem, d)
		}
	}
	if l := len(newItem); l > 0 {
		coll.values[cache.IID] = coll.val(cache.IID) + int64(l)
	}
	cache.Result = newItem
	return nil
}
