package updater

import (
	"fmt"
	"github.com/hwcer/updater/dataset"
	"github.com/hwcer/updater/operator"
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
func collectionHandleResolve(coll *Collection, op *operator.Operator) error {
	return nil
}

func collectionHandleDel(coll *Collection, op *operator.Operator) (err error) {
	if err = collectionParseId(coll, op); err == nil {
		coll.values[op.OID] = 0
	}
	return
}

// New 必须是创建好的ITEM对象
func collectionHandleNew(coll *Collection, op *operator.Operator) (err error) {
	if op.OID == "" || op.IID <= 0 || op.Value <= 0 {
		return coll.Updater.Errorf("operator[New] oid iid,value cannot be empty:%+v", op)
	}
	if op.Result == nil {
		return coll.Updater.Errorf("operator[New] Result empty:%+v", op)
	}
	if _, ok := op.Result.([]any); !ok {
		return coll.Updater.Errorf("operator[New] Result type must be []any :%+v", op)
	}
	coll.values[op.OID] += op.Value
	return
}

func collectionHandleAdd(coll *Collection, op *operator.Operator) (err error) {
	op.OID, err = coll.ObjectId(op.IID)
	if err == ErrUnableUseIIDOperation {
		return collectionHandleNewEquip(coll, op) //不可以堆叠装备类道具
	} else if err != nil {
		return
	}
	if v, ok := coll.val(op.OID); !ok {
		return collectionHandleNewItem(coll, op) //可以叠加的装备
	} else {
		r := op.Value + v
		op.Result = r
		coll.values[op.OID] = r
	}
	return
}

func collectionHandleSub(coll *Collection, op *operator.Operator) (err error) {
	if op.OID, err = coll.ObjectId(op.IID); err != nil {
		return
	}
	d, ok := coll.val(op.OID)
	if op.Value > d && coll.Updater.strict {
		return ErrItemNotEnough(op.IID, op.Value, d)
	}
	if !ok {
		op.Type = operator.Types_Drop
	} else {
		r := d - op.Value
		if r < 0 {
			r = 0
		}
		op.Result = r
		coll.values[op.OID] = r
	}
	return
}

func collectionHandleSet(coll *Collection, op *operator.Operator) (err error) {
	if err = collectionParseId(coll, op); err != nil {
		return err
	}
	if _, ok := coll.val(op.OID); !ok {
		return ErrItemNotExist(op.OID)
	}
	update, _ := op.Result.(dataset.Update)
	if v, ok := update[operator.ItemNameVAL]; ok {
		coll.values[op.IID] = ParseInt64(v)
	}
	return
}

// collectionCompareTransform MAX MIN符合规则的转换成ADD或者SET
func collectionCompareTransform(coll *Collection, op *operator.Operator, ok bool) (err error) {
	if !ok {
		op.Type = operator.Types_Add
		err = collectionHandleAdd(coll, op)
	} else {
		op.Type = operator.Types_Set
		op.Result = dataset.NewUpdate(operator.ItemNameVAL, op.Value)
		err = collectionHandleSet(coll, op)
	}
	return
}
func collectionHandleMax(coll *Collection, op *operator.Operator) (err error) {
	if err = collectionParseId(coll, op); err != nil {
		return err
	}
	if v, ok := coll.val(op.OID); op.Value > v {
		err = collectionCompareTransform(coll, op, ok)
	} else {
		op.Type = operator.Types_Drop
	}
	return
}

func collectionHandleMin(coll *Collection, op *operator.Operator) (err error) {
	if err = collectionParseId(coll, op); err != nil {
		return err
	}
	if v, ok := coll.val(op.OID); op.Value < v {
		err = collectionCompareTransform(coll, op, ok)
	} else {
		op.Type = operator.Types_Drop
	}
	return
}

// collectionHandleNewEquip
func collectionHandleNewEquip(coll *Collection, op *operator.Operator) error {
	op.Type = operator.Types_New
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
	for _, item := range newItem {
		doc := dataset.NewDocument(item)
		oid := doc.OID()
		coll.values[oid] = 1
	}
	op.Result = newItem

	return nil
}

func collectionHandleNewItem(coll *Collection, op *operator.Operator) error {
	op.Type = operator.Types_New
	it := coll.IType(op.IID)
	if it == nil {
		return ErrITypeNotExist(op.IID)
	}
	if op.Value == 0 {
		op.Value = 1
	}
	if item, err := it.New(coll.Updater, op); err == nil {
		op.Result = []any{item}
		coll.values[op.OID] = op.Value
	} else {
		return err
	}

	return nil
}
func collectionParseId(coll *Collection, op *operator.Operator) (err error) {
	if op.OID == "" {
		op.OID, err = coll.ObjectId(op.IID)
	} else if op.IID <= 0 {
		op.IID, err = Config.ParseId(coll.Updater, op.OID)
	}
	return
}
