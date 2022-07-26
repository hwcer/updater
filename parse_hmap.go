package updater

import (
	"errors"
	"fmt"
	"github.com/hwcer/cosmo/update"
)

//无限叠加的道具

type hmapParse func(*Table, *Cache) error

var hmapParseHandle = make(map[ActType]hmapParse)

func init() {
	hmapParseHandle[ActTypeAdd] = hmapHandleAdd
	hmapParseHandle[ActTypeSub] = hmapHandleSub
	hmapParseHandle[ActTypeSet] = hmapHandleSet
	hmapParseHandle[ActTypeDel] = hmapHandleDel
	hmapParseHandle[ActTypeMax] = hmapHandleMax
	hmapParseHandle[ActTypeMin] = hmapHandleMin
	hmapParseHandle[ActTypeResolve] = hmapHandleResolve
}

func parseHMap(this *Table, act *Cache) (err error) {
	var f hmapParse
	var ok bool
	if f, ok = hmapParseHandle[act.AType]; !ok {
		return fmt.Errorf("table_act_parse not exist:%v", act.AType)
	}
	return f(this, act)
}

//hmapHandleResolve 仅仅标记不做任何处理
func hmapHandleResolve(t *Table, act *Cache) error {
	return nil
}

func hmapHandleDel(t *Table, act *Cache) error {
	act.AType = ActTypeDel
	if !t.dataset.Del(act.OID) {
		return nil
	}
	bulkWrite := t.BulkWrite()
	bulkWrite.Delete(act.OID)
	return nil
}

func hmapHandleNew(h *Table, act *Cache) (err error) {
	data, err := tableHandleNewItem(h, act)
	bulkWrite := h.BulkWrite()
	act.AType = ActTypeNew
	act.Ret = []interface{}{data}
	bulkWrite.Insert(data)
	return
}

func hmapHandleAdd(h *Table, act *Cache) (err error) {
	data, ok := h.dataset.Get(act.OID)
	if !ok {
		return hmapHandleNew(h, act)
	}
	bulkWrite := h.BulkWrite()
	v, ok := ParseInt(act.Val)
	if !ok || v <= 0 {
		return ErrActValIllegal(act)
	}
	if act.Ret, err = data.Add(ItemNameVAL, v); err != nil {
		return
	}
	upsert := update.Update{}
	upsert.Inc(ItemNameVAL, v)
	bulkWrite.Update(upsert, act.OID)
	return nil
}

func hmapHandleSub(h *Table, act *Cache) (err error) {
	data, ok := h.dataset.Get(act.OID)
	if !ok {
		return hmapHandleNew(h, act)
	}
	v, ok := ParseInt(act.Val)
	if !ok || v <= 0 {
		return ErrActValIllegal(act)
	}
	bulkWrite := h.BulkWrite()
	if act.Ret, err = data.Add(ItemNameVAL, -v); err != nil {
		return
	}
	upsert := update.Update{}
	upsert.Inc(ItemNameVAL, -v)
	bulkWrite.Update(upsert, act.OID)
	return
}

func hmapHandleMax(h *Table, act *Cache) (err error) {
	data, ok := h.dataset.Get(act.OID)
	if !ok {
		return hmapHandleNew(h, act)
	}
	v, ok := ParseInt(act.Val)
	if !ok {
		return ErrActValIllegal(act)
	}
	var d int64
	d, ok = data.GetInt(act.Key)
	if ok && v > d {
		err = hmapHandleSet(h, act)
	} else {
		act.AType = ActTypeDrop
	}
	return
}

func hmapHandleMin(h *Table, act *Cache) (err error) {
	data, ok := h.dataset.Get(act.OID)
	if !ok {
		return hmapHandleNew(h, act)
	}
	v, ok := ParseInt(act.Val)
	if !ok {
		return ErrActValIllegal(act)
	}
	var d int64
	d, ok = data.GetInt(act.Key)
	if ok && v < d {
		err = hmapHandleSet(h, act)
	} else {
		act.AType = ActTypeDrop
	}
	return
}

func hmapHandleSet(h *Table, act *Cache) (err error) {
	data, ok := h.dataset.Get(act.OID)
	if !ok {
		return hmapHandleNew(h, act)
	}
	bulkWrite := h.BulkWrite()
	act.Ret = act.Val
	act.AType = ActTypeSet
	val := ParseMap(act.Val)
	upsert := update.Update{}
	for k, v := range val {
		if err = data.Set(k, v); err != nil {
			return
		}
		b, _ := data.Get(k)
		upsert.Set(k, b)
	}
	bulkWrite.Update(upsert, act.OID)
	return
}

func tableHandleNewItem(t *Table, act *Cache) (interface{}, error) {
	v, err := act.IType.New(t.updater, act)
	if err != nil {
		return nil, err
	}
	data, ok := v.(ModelTable)
	if !ok {
		return nil, errors.New("IType.New return error")
	}
	t.dataset.Set(data)
	return data.Copy(), nil
}
