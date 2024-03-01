package updater

import (
	"fmt"
	"github.com/hwcer/updater/dataset"
	"github.com/hwcer/updater/operator"
)

var collectionParseHandle = make(map[operator.Types]func(*Collection, *operator.Operator) error)

func init() {
	collectionParseHandle[operator.TypesNew] = collectionHandleNew
	collectionParseHandle[operator.TypesAdd] = collectionHandleAdd
	collectionParseHandle[operator.TypesSub] = collectionHandleSub
	collectionParseHandle[operator.TypesSet] = collectionHandleSet
	collectionParseHandle[operator.TypesDel] = collectionHandleDel
	collectionParseHandle[operator.TypesMax] = collectionHandleMax
	collectionParseHandle[operator.TypesMin] = collectionHandleMin
	collectionParseHandle[operator.TypesDrop] = collectionHandleResolve
	collectionParseHandle[operator.TypesResolve] = collectionHandleResolve
}

func (this *Collection) Parse(op *operator.Operator) (err error) {
	if err = overflow(this.Updater, this, op); err != nil {
		return
	}
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
	if op.OID == "" {
		return ErrOIDEmpty(op.IID)
	}
	coll.values.set(op.OID, 0) //TODO
	return
}

// New 必须是创建好的ITEM对象,仅外部直接创建新对象时调用
func collectionHandleNew(coll *Collection, op *operator.Operator) (err error) {
	if op.OID == "" {
		return coll.Updater.Errorf("operator[New] oid  cannot be empty:%+v", op)
	}
	if op.Result == nil {
		return coll.Updater.Errorf("operator[New] Result empty:%+v", op)
	}
	if _, ok := op.Result.([]any); !ok {
		return coll.Updater.Errorf("operator[New] Result type must be []any :%+v", op)
	}
	coll.values.add(op.OID, op.Value)
	return
}

func collectionHandleAdd(coll *Collection, op *operator.Operator) (err error) {
	if it := coll.ITypeCollection(op.IID); it != nil && !it.Stacked() {
		//不可以堆叠装备类道具
		return collectionHandleNewEquip(coll, op)
	}
	//可以叠加的道具
	if op.OID == "" {
		return ErrOIDEmpty(op.IID)
	}
	if v, ok := coll.val(op.OID); !ok {
		return collectionHandleNewItem(coll, op)
	} else {
		op.Result = op.Value + v
		coll.values.add(op.OID, op.Value)
	}
	return
}

func collectionHandleSub(coll *Collection, op *operator.Operator) (err error) {
	if op.OID == "" {
		return ErrOIDEmpty(op.IID)
	}
	d, _ := coll.val(op.OID)
	if op.Value > d {
		return ErrItemNotEnough(op.IID, op.Value, d)
	}
	op.Result = d - op.Value
	coll.values.sub(op.OID, op.Value)
	return
}

func collectionHandleSet(coll *Collection, op *operator.Operator) (err error) {
	if op.OID == "" {
		return ErrOIDEmpty(op.IID)
	}
	if _, ok := coll.val(op.OID); !ok && coll.model.Upsert(coll.Updater, op) {
		it := coll.ITypeCollection(op.IID)
		var i any
		if i, err = it.New(coll.Updater, op); err != nil {
			return
		}
		doc := dataset.NewDocument(i)
		if err = doc.Update(op.Result.(dataset.Update)); err != nil {
			return
		}
		op.Type = operator.TypesNew
		op.Value = doc.VAL()
		op.Result = []any{doc.Interface()}
		return collectionHandleNew(coll, op)
	} else if !ok {
		return ErrItemNotExist(op.OID)
	}

	update, _ := op.Result.(dataset.Update)
	if v, ok := update[dataset.ItemNameVAL]; ok {
		coll.values.set(op.OID, dataset.ParseInt64(v))
	}
	return
}

// collectionCompareTransform MAX MIN符合规则的转换成ADD或者SET
func collectionCompareTransform(coll *Collection, op *operator.Operator, ok bool) (err error) {
	if !ok {
		op.Type = operator.TypesAdd
		err = collectionHandleAdd(coll, op)
	} else {
		op.Type = operator.TypesSet
		op.Result = dataset.NewUpdate(dataset.ItemNameVAL, op.Value)
		err = collectionHandleSet(coll, op)
	}
	return
}
func collectionHandleMax(coll *Collection, op *operator.Operator) (err error) {
	if op.OID == "" {
		return ErrOIDEmpty(op.IID)
	}
	if v, ok := coll.val(op.OID); op.Value > v {
		err = collectionCompareTransform(coll, op, ok)
	} else {
		op.Type = operator.TypesDrop
	}
	return
}

func collectionHandleMin(coll *Collection, op *operator.Operator) (err error) {
	if op.OID == "" {
		return ErrOIDEmpty(op.IID)
	}
	if v, ok := coll.val(op.OID); op.Value < v {
		err = collectionCompareTransform(coll, op, ok)
	} else {
		op.Type = operator.TypesDrop
	}
	return
}

// collectionHandleNewEquip
func collectionHandleNewEquip(coll *Collection, op *operator.Operator) error {
	op.Type = operator.TypesNew
	it := coll.ITypeCollection(op.IID)
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
		coll.values.add(oid, 1)
	}
	op.Result = newItem
	return nil
}

func collectionHandleNewItem(coll *Collection, op *operator.Operator) error {
	op.Type = operator.TypesNew
	it := coll.ITypeCollection(op.IID)
	if it == nil {
		return ErrITypeNotExist(op.IID)
	}
	if op.Value == 0 {
		op.Value = 1
	}
	if item, err := it.New(coll.Updater, op); err == nil {
		op.Result = []any{item}
		coll.values.add(op.OID, op.Value)
	} else {
		return err
	}

	return nil
}
