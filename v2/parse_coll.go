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
	collectionParseHandle[operator.Types_New] = collectionHandleNew
	collectionParseHandle[operator.Types_Add] = collectionHandleAdd
	collectionParseHandle[operator.Types_Sub] = collectionHandleSub
	collectionParseHandle[operator.Types_Set] = collectionHandleSet
	collectionParseHandle[operator.Types_Del] = collectionHandleDel
	collectionParseHandle[operator.Types_Max] = collectionHandleMax
	collectionParseHandle[operator.Types_Min] = collectionHandleMin
	collectionParseHandle[operator.Types_Resolve] = collectionHandleResolve
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

func collectionHandleNew(coll *Collection, op *operator.Operator) (err error) {
	op.Type = operator.Types_New
	var v int64
	if it := coll.IType(op.IID); !it.Multiple() {
		v, err = collectionHandleNewUnique(coll, op)
	} else {
		v, err = collectionHandleNewMultiple(coll, op)
	}
	if err == nil {
		coll.values[op.IID] = coll.val(op.IID) + v
	}
	return
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
		cache.Type = operator.Types_Drop
	} else {
		r := d - v
		cache.Result = r
		coll.values[cache.IID] = r
	}
	return
}

func collectionHandleSet(coll *Collection, cache *operator.Operator) (err error) {
	if d := coll.val(cache.IID); d <= 0 {
		return ErrItemNotExist(cache.OID)
		//return collectionHandleNew(coll, cache)
	}
	cache.Result = cache.Value
	cache.Type = operator.Types_Set
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
		cache.Type = operator.Types_Drop
	}
	return
}

func collectionHandleMin(coll *Collection, cache *operator.Operator) (err error) {
	if d, v := coll.val(cache.IID), ParseInt64(cache.Value); v < d {
		err = collectionTransformSet(coll, cache)
	} else {
		cache.Type = operator.Types_Drop
	}
	return
}

func collectionHandleNewUnique(coll *Collection, op *operator.Operator) (v int64, err error) {
	it := coll.IType(op.IID)
	if it == nil {
		return 0, ErrITypeNotExist(op.IID)
	}
	v = ParseInt64(op.Value)
	if v == 0 {
		op.Value, v = 1, 1
	}
	if op.Result != nil {
		return
	}
	var item any
	var newItem []any
	for i := int64(1); i <= v; i++ {
		if item, err = it.New(coll.Updater, op); err == nil {
			newItem = append(newItem, item)
		} else {
			return
		}
	}
	op.Result = newItem
	return
}

func collectionHandleNewMultiple(coll *Collection, op *operator.Operator) (v int64, err error) {
	it := coll.IType(op.IID)
	if it == nil {
		return 0, ErrITypeNotExist(op.IID)
	}
	v = ParseInt64(op.Value)
	if v == 0 {
		op.Value, v = 1, 1
	}
	var item any
	if item, err = it.New(coll.Updater, op); err == nil {
		op.Result = []any{item}
	}
	return
}
