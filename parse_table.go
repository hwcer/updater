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
		return ErrActValIllegal
	}
	it := Config.IType(act.IID)
	if it == nil {
		return ErrITypeNotExist(act.IID)
	}

	var newItem []interface{}
	for i := int64(1); i <= v; i++ {
		var oid string
		if oid, err = t.CreateId(act.IID); err != nil {
			return
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
			return
		}
		newItem = append(newItem, data)
		t.dataset.Set(data)
		if onCreate, ok := it.(ITypeOnCreate); ok {
			onCreate.OnCreate(t.updater, data)
		}
	}
	act.Ret = newItem
	bulkWrite := t.BulkWrite()
	bulkWrite.Insert(newItem...)
	return
}
