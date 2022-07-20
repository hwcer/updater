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
	it := Config.IType(act.IID)
	if it == nil {
		return ErrITypeNotExist(act.IID)
	}
	var data interface{}
	if itNew, ok := it.(ITypeNew); ok {
		data, err = itNew.New(h.updater, act)
	} else {
		var oid string
		if act.OID != "" {
			oid = act.OID
		} else if oid, err = it.CreateId(h.updater, act.IID); err != nil {
			return
		}
		data = h.base.New()
		item := NewData(h.model.Schema, data)
		val := make(map[string]interface{})
		val[ItemNameOID] = oid
		val[ItemNameIID] = act.IID
		val[ItemNameUID] = h.updater.uid
		switch act.AType {
		case ActTypeAdd, ActTypeMax, ActTypeMin:
			v, _ := ParseInt(act.Val)
			val[ItemNameVAL] = v
		case ActTypeSub:
			v, _ := ParseInt(act.Val)
			val[ItemNameVAL] = -v
		case ActTypeSet:
			values, _ := act.Val.(map[string]interface{})
			for k, v := range values {
				val[k] = v
			}
		}
		err = item.MSet(val)
	}

	if err != nil {
		return
	}

	bulkWrite := h.BulkWrite()
	if mod, ok := data.(ModelCopy); ok {
		h.dataset.Set(mod.Copy())
	} else {
		h.dataset.Set(data)
	}

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
