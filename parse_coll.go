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
	coll.dataset.Delete(op.OID)
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
	items, ok := op.Result.([]any)
	if !ok {
		return coll.Updater.Errorf("operator[New] Result type must be []any :%+v", op)
	}
	op.Result, err = collectionHandleInsert(coll, items...)
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
		err = coll.dataset.Set(op.OID, dataset.ItemNameVAL, op.Result)
	}
	return
}

func collectionHandleSub(coll *Collection, op *operator.Operator) error {
	if op.OID == "" {
		return ErrOIDEmpty(op.IID)
	}
	d, _ := coll.val(op.OID)
	r, err := coll.Updater.deduct(op.IID, d, op.Value)
	if err != nil {
		return err
	}
	op.Result = r
	err = coll.dataset.Set(op.OID, dataset.ItemNameVAL, r)
	return nil
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
		doc := dataset.NewDoc(i)
		doc.Update(op.Result.(dataset.Update))
		op.Type = operator.TypesNew
		op.Value = doc.GetInt64(dataset.ItemNameVAL)
		op.Result = []any{doc.Any()}
		return collectionHandleNew(coll, op)
	} else if !ok {
		return ErrItemNotExist(op.OID)
	}

	update, _ := op.Result.(dataset.Update)
	if v, ok := update[dataset.ItemNameVAL]; ok {
		err = coll.dataset.Set(op.OID, dataset.ItemNameVAL, dataset.ParseInt64(v))
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
func collectionHandleNewEquip(coll *Collection, op *operator.Operator) (err error) {
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
	var item any
	var items []any
	for i := int64(1); i <= op.Value; i++ {
		if item, err = it.New(coll.Updater, op); err != nil {
			return
		} else {
			items = append(items, item)
		}
	}
	op.Result, err = collectionHandleInsert(coll, items...)
	return
}

func collectionHandleNewItem(coll *Collection, op *operator.Operator) (err error) {
	op.Type = operator.TypesNew
	it := coll.ITypeCollection(op.IID)
	if it == nil {
		return ErrITypeNotExist(op.IID)
	}
	if op.Value == 0 {
		op.Value = 1
	}
	var item any
	if item, err = it.New(coll.Updater, op); err == nil {
		op.Result, err = collectionHandleInsert(coll, item)
	}
	return
}

func collectionHandleInsert(coll *Collection, vs ...any) (r []any, err error) {
	for _, v := range vs {
		doc := dataset.NewDoc(v)
		//var j dataset.Update
		//if j, err = doc.Json(); err != nil {
		//	return
		//}
		r = append(r, v)
		err = coll.dataset.Insert(doc)
	}
	return
}
