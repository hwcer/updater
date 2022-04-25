package updater

import (
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
	v, ok := ParseInt(act.Val)
	if !ok || v <= 0 {
		return ErrActValIllegal
	}
	it := Config.IType(act.IID)
	if it == nil {
		return ErrITypeNotExist(act.IID)
	}
	var oid string
	if oid, err = h.CreateId(act.IID); err != nil {
		return
	}
	data := h.base.New()
	item := NewData(h.model.Schema, data)
	err = item.MSet(map[string]interface{}{
		ItemNameOID: oid,
		ItemNameIID: act.IID,
		ItemNameVAL: v,
		ItemNameUID: h.updater.uid,
	})
	if err != nil {
		return
	}

	bulkWrite := h.BulkWrite()
	h.dataset.Set(data)
	act.AType = ActTypeNew
	act.Ret = []interface{}{data}
	bulkWrite.Insert(data)
	if onCreate, ok := it.(ITypeOnCreate); ok {
		onCreate.OnCreate(h.updater, data)
	}
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
		return ErrActValIllegal
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
		return ErrActValIllegal
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

func hmapHandleSet(h *Table, act *Cache) (err error) {
	data, ok := h.dataset.Get(act.OID)
	if !ok {
		return hmapHandleNew(h, act)
	}
	bulkWrite := h.BulkWrite()
	act.Ret = act.Val
	val, ok := act.Val.(map[string]interface{})
	if !ok {
		return ErrActValIllegal
	}
	upsert := update.Update{}
	for k, v := range val {
		upsert.Set(k, v)
		if err = data.Set(k, v); err != nil {
			return
		}
	}
	bulkWrite.Update(upsert, act.OID)
	return
}
