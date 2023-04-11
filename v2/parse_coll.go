package updater

import (
	"fmt"
	"github.com/hwcer/updater/v2/dataset"
	"github.com/hwcer/updater/v2/operator"
)

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

func (this *Collection) Parse(op *operator.Operator) (err error) {
	if f, ok := collectionParseHandle[op.Type]; ok {
		return f(this, op)
	}
	return fmt.Errorf("collection operator type not exist:%v", op.Type.ToString())

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
	if it := coll.IType(op.IID); !it.Multiple() {
		err = collectionHandleNewUnique(coll, op)
	} else {
		err = collectionHandleNewMultiple(coll, op)
	}
	if err == nil {
		coll.values[op.IID] = coll.val(op.IID) + op.Value
	}
	return
}

func collectionHandleAdd(coll *Collection, op *operator.Operator) (err error) {
	d := coll.val(op.IID)
	if d <= 0 {
		return collectionHandleNew(coll, op)
	}
	r := op.Value + d
	op.Result = r
	coll.values[op.IID] = r
	return
}

func collectionHandleSub(coll *Collection, op *operator.Operator) (err error) {
	d := coll.val(op.IID)
	if op.Value > d && !coll.Updater.tolerate {
		return ErrItemNotEnough(op.IID, op.Value, d)
	}
	r := d - op.Value
	if r < 0 {
		r = 0
	}
	op.Result = r
	coll.values[op.IID] = r
	return
}

func collectionHandleSet(coll *Collection, op *operator.Operator) (err error) {
	if d := coll.val(op.IID); d <= 0 {
		return ErrItemNotExist(op.OID)
	}
	op.Type = operator.Types_Set
	if update, ok := op.Result.(dataset.Update); ok {
		if v, ok := update[operator.ItemNameVAL]; ok {
			coll.values[op.IID] = ParseInt64(v)
		}
	}
	return
}
func collectionTransformSet(coll *Collection, op *operator.Operator) error {
	op.Result = dataset.NewUpdate(operator.ItemNameVAL, op.Value)
	return collectionHandleSet(coll, op)
}
func collectionHandleMax(coll *Collection, op *operator.Operator) (err error) {
	if op.Value > coll.val(op.IID) {
		err = collectionTransformSet(coll, op)
	} else {
		op.Type = operator.Types_Drop
	}
	return
}

func collectionHandleMin(coll *Collection, op *operator.Operator) (err error) {
	if op.Value < coll.val(op.IID) {
		err = collectionTransformSet(coll, op)
	} else {
		op.Type = operator.Types_Drop
	}
	return
}

func collectionHandleNewUnique(coll *Collection, op *operator.Operator) error {
	it := coll.IType(op.IID)
	if it == nil {
		return ErrITypeNotExist(op.IID)
	}
	if op.Value == 0 {
		op.Value = 1
	}
	if op.Result != nil {
		return nil
	}
	var newItem []any
	for i := int64(1); i <= op.Value; i++ {
		if item, err := it.New(coll.Updater, op); err == nil {
			newItem = append(newItem, item)
		} else {
			return err
		}
	}
	op.Result = newItem
	return nil
}

func collectionHandleNewMultiple(coll *Collection, op *operator.Operator) error {
	it := coll.IType(op.IID)
	if it == nil {
		return ErrITypeNotExist(op.IID)
	}
	if op.Value == 0 {
		op.Value = 1
	}
	if item, err := it.New(coll.Updater, op); err == nil {
		op.Result = []any{item}
	} else {
		return err
	}
	return nil
}
