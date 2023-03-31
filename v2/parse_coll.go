package updater

import (
	"fmt"
	"github.com/hwcer/updater/bson"
	"github.com/hwcer/updater/v2/dataset"
	"github.com/hwcer/updater/v2/operator"
)

// 无限叠加的道具
var collectionParseHandle = make(map[operator.Types]func(*Collection, *operator.Operator) error)

func init() {
	collectionParseHandle[operator.TypeNew] = collectionHandleNew
	collectionParseHandle[operator.TypeAdd] = collectionHandleAdd
	collectionParseHandle[operator.TypeSub] = collectionHandleSub
	collectionParseHandle[operator.TypeSet] = collectionHandleSet
	collectionParseHandle[operator.TypeDel] = collectionHandleDel
	collectionParseHandle[operator.TypeMax] = collectionHandleMax
	collectionParseHandle[operator.TypeMin] = collectionHandleMin
	collectionParseHandle[operator.TypeResolve] = collectionHandleResolve
}

func (this *Collection) Parse(act *operator.Operator) (err error) {
	f, ok := collectionParseHandle[act.Type]
	if !ok {
		return fmt.Errorf("collectionParseHandle not exist:%v", act.Type)
	}
	return f(this, act)
}

// hmapHandleResolve 仅仅标记不做任何处理
func collectionHandleResolve(coll *Collection, act *operator.Operator) error {
	return nil
}

func collectionHandleDel(coll *Collection, act *operator.Operator) error {
	return nil
}

func collectionHandleNew(coll *Collection, cache *operator.Operator) error {
	cache.Type = operator.TypeNew
	if it := coll.Updater.IType(cache.IID); !it.Unique() {
		return collectionHandleNewMultiple(coll, cache)
	} else {
		return collectionHandleNewUnique(coll, cache)
	}
}

func collectionHandleAdd(coll *Collection, cache *operator.Operator) (err error) {
	d := coll.val(cache.IID)
	if d <= 0 {
		return collectionHandleNew(coll, cache)
	}
	r := ParseInt64(cache.Value) + d
	cache.Result = r
	coll.values[cache.IID] = r
	return
}

func collectionHandleSub(coll *Collection, cache *operator.Operator) (err error) {
	d := coll.val(cache.IID)
	v := bson.ParseInt64(cache.Value)
	if v > d {
		if coll.Updater.tolerate {
			v = d
		} else {
			return ErrItemNotEnough(cache.IID, v, d)
		}
	}
	if d <= 0 {
		cache.Type = operator.TypeDrop
	} else {
		r := d - v
		cache.Result = r
		coll.values[cache.IID] = r
	}
	return
}

func collectionHandleSet(coll *Collection, cache *operator.Operator) (err error) {
	d := coll.val(cache.IID)
	if d <= 0 {
		return collectionHandleNew(coll, cache)
	}
	cache.Result = cache.Value
	cache.Type = operator.TypeSet
	update, _ := cache.Value.(dataset.Update)
	if v, ok := update[operator.ItemNameVAL]; ok {
		coll.values[cache.IID] = ParseInt64(v)
	}
	return
}
func collectionTransformSet(coll *Collection, cache *operator.Operator) error {
	cache.Value = dataset.NewUpdate(operator.ItemNameVAL, cache.Value)
	return collectionHandleSet(coll, cache)
}
func collectionHandleMax(coll *Collection, cache *operator.Operator) (err error) {
	if d, v := coll.val(cache.IID), ParseInt64(cache.Value); v > d {
		err = collectionTransformSet(coll, cache)
	} else {
		cache.Type = operator.TypeDrop
	}
	return
}

func collectionHandleMin(coll *Collection, cache *operator.Operator) (err error) {
	if d, v := coll.val(cache.IID), ParseInt64(cache.Value); v < d {
		err = collectionTransformSet(coll, cache)
	} else {
		cache.Type = operator.TypeDrop
	}
	return
}

func collectionHandleNewUnique(coll *Collection, cache *operator.Operator) error {
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
func collectionHandleNewMultiple(coll *Collection, cache *operator.Operator) error {
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
