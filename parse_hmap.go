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



func doHMapAct(data *Data, act *Cache) (err error) {
	switch act.AType {
	case ActTypeAdd,ActTypeSub:
		v, ok := ParseInt(act.Val)
		if !ok || v <= 0 {
			return ErrActValIllegal(act)
		}
		if act.AType == ActTypeSub{
			v = -v
		}
		act.Ret, err = data.Add(act.Key, v)
	case ActTypeSet:
		values := ParseMap(act.Key,act.Val)
		var r interface{}
		for k, v := range values {
			if r,err = data.Set(k, v); err == nil {
				values[k] = r
			}else{
				return
			}
		}
		act.Ret = values
	case ActTypeMax,ActTypeMin:
		v, ok := ParseInt(act.Val)
		if !ok {
			return ErrActValIllegal(act)
		}
		var d int64
		d, _ = data.GetInt(act.Key)
		if (act.AType == ActTypeMax && v > d) || (act.AType == ActTypeMin && v < d) {
			act.Ret,err = data.Set(act.Key, v)
		}
	}
	return
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
	val, err := tableHandleNewItem(h, act)
	if err!=nil{
		return err
	}
	data,_ := h.dataset.Get(act.OID)
	if err = doHMapAct(data,act);err!=nil{
		return
	}
	bulkWrite := h.BulkWrite()
	act.AType = ActTypeNew
	act.Ret = []interface{}{val}
	bulkWrite.Insert(val)
	return
}

func hmapHandleAdd(h *Table, act *Cache) (err error) {
	data, ok := h.dataset.Get(act.OID)
	if !ok {
		return hmapHandleNew(h, act)
	}
	if err = doHMapAct(data,act);err!=nil{
		return
	}
	bulkWrite := h.BulkWrite()
	upsert := update.Update{}
	upsert.Inc(act.Key, act.Val)
	bulkWrite.Update(upsert, act.OID)
	return nil
}

func hmapHandleSub(h *Table, act *Cache) (err error) {
	data, ok := h.dataset.Get(act.OID)
	if !ok {
		return hmapHandleNew(h, act)
	}
	if err = doHMapAct(data,act);err!=nil{
		return
	}
	v, _ := ParseInt(act.Val)
	bulkWrite := h.BulkWrite()
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
	if err = doHMapAct(data,act);err!=nil{
		return
	}
	if act.Ret == nil{
		act.AType = ActTypeDrop
	}else {
		act.AType = ActTypeSet
		bulkWrite := h.BulkWrite()
		upsert := update.Update{}
		upsert.Set(act.Key, act.Ret)
		bulkWrite.Update(upsert, act.OID)
	}
	return
}

func hmapHandleMin(h *Table, act *Cache) (err error) {
	data, ok := h.dataset.Get(act.OID)
	if !ok {
		return hmapHandleNew(h, act)
	}
	if err = doHMapAct(data,act);err!=nil{
		return
	}
	if act.Ret == nil{
		act.AType = ActTypeDrop
	}else {
		act.AType = ActTypeSet
		bulkWrite := h.BulkWrite()
		upsert := update.Update{}
		upsert.Set(act.Key, act.Ret)
		bulkWrite.Update(upsert, act.OID)
	}
	return
}

func hmapHandleSet(h *Table, act *Cache) (err error) {
	data, ok := h.dataset.Get(act.OID)
	if !ok {
		return hmapHandleNew(h, act)
	}
	if err = doHMapAct(data,act);err!=nil{
		return
	}
	bulkWrite := h.BulkWrite()
	upsert := update.Update{}
	values := act.Ret.(map[string]interface{})
	for k, v := range values {
		upsert.Set(k, v)
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
