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

func tableHandleNew(t *Table, act *Cache) error {
	v, ok := ParseInt(act.Val)
	if !ok || v <= 0 {
		return ErrActValIllegal(act)
	}
	var newItem []interface{}
	for i := int64(1); i <= v; i++ {
		if d, err := tableHandleNewItem(t, act); err != nil {
			return err
		} else {
			newItem = append(newItem, d)
		}
	}
	act.Ret = newItem
	bulkWrite := t.BulkWrite()
	bulkWrite.Insert(newItem...)
	return nil
}
