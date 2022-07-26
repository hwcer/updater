package updater

import (
	"fmt"
)

//不可以叠加的道具

var tableParseHandle = make(map[ActType]func(*Table, *Cache) error)

func init() {
	tableParseHandle[ActTypeDel] = hmapHandleDel
	tableParseHandle[ActTypeSet] = hmapHandleSet
	tableParseHandle[ActTypeNew] = tableHandleNew
	tableParseHandle[ActTypeResolve] = hmapHandleResolve
}

func parseTable(this *Table, act *Cache) error {
	if f, ok := tableParseHandle[act.AType]; ok {
		return f(this, act)
	}
	return fmt.Errorf("table_act_parse not exist:%v", act.AType)
}

func tableHandleNew(t *Table, act *Cache) (err error) {
	v, ok := ParseInt(act.Val)
	if !ok || v <= 0 {
		return ErrActValIllegal(act)
	}
	var d interface{}
	var newItem []interface{}
	for i := int64(1); i <= v; i++ {
		if d, err = tableHandleNewItem(t, act); err != nil {
			return
		} else {
			newItem = append(newItem, d)
		}
	}
	act.Ret = newItem
	bulkWrite := t.BulkWrite()
	bulkWrite.Insert(newItem...)
	return
}

func tableHandleNewItem(t *Table, act *Cache) (interface{}, error) {
	if itNew, ok := act.IType.(ITypeNew); ok {
		return itNew.New(t.updater, act)
	}
	oid, err := act.IType.CreateId(t.base.updater, act.IID)
	if err != nil {
		return nil, err
	}
	data := t.base.New()
	item := NewData(t.model.Schema, data)
	err = item.MSet(map[string]interface{}{
		ItemNameOID: oid,
		ItemNameIID: act.IID,
		ItemNameVAL: int64(1),
		ItemNameUID: t.updater.uid,
	})
	if err != nil {
		return nil, err
	}
	t.dataset.Set(data)
	if onCreate, ok := act.IType.(ITypeOnCreate); ok {
		onCreate.OnCreate(t.updater, data)
	}
	if cp, ok := data.(ModelCopy); ok {
		return cp.Copy(), nil
	} else {
		return data, nil
	}
}
