package updater

import (
	"errors"
)

var hashParseHandle = make(map[ActType]func(*Hash, *Cache) error)

func init() {
	hashParseHandle[ActTypeAdd] = hashHandleAdd
	hashParseHandle[ActTypeSet] = hashHandleSet
	hashParseHandle[ActTypeSub] = hashHandleSub
	hashParseHandle[ActTypeMax] = hashHandleMax
	hashParseHandle[ActTypeMin] = hashHandleMin
}

func parseHash(h *Hash, act *Cache) error {
	if h.updater.strict && act.AType == ActTypeSub {
		av, _ := ParseInt(act.Val)
		dv := h.Val(act.Key)
		if av > dv {
			return ErrItemNotEnough(act.IID, av, dv)
		}
	}
	if f, ok := hashParseHandle[act.AType]; ok {
		return f(h, act)
	}
	return errors.New("hash_act_parser not exist")
}

func hashHandleAdd(h *Hash, act *Cache) (err error) {
	v, _ := ParseInt(act.Val)
	act.Ret, err = h.data.Add(act.Key, v)
	h.update.Inc(act.Key, v)
	return
}

func hashHandleSub(h *Hash, act *Cache) (err error) {
	v, _ := ParseInt(act.Val)
	act.Ret, err = h.data.Add(act.Key, -v)
	h.update.Inc(act.Key, -v)
	return

}

func hashHandleMax(h *Hash, act *Cache) (err error) {
	v, _ := ParseInt(act.Val)
	d, ok := h.data.GetInt(act.Key)
	if ok && v > d {
		act.AType = ActTypeSet
		err = hashHandleSet(h, act)
	} else {
		act.AType = ActTypeDrop
	}
	return
}

func hashHandleMin(h *Hash, act *Cache) (err error) {
	v, _ := ParseInt(act.Val)
	d, ok := h.data.GetInt(act.Key)
	if ok && v < d {
		act.AType = ActTypeSet
		err = hashHandleSet(h, act)
	} else {
		act.AType = ActTypeDrop
	}
	return
}

func hashHandleSet(h *Hash, act *Cache) (err error) {
	act.Ret = act.Val
	h.data.Set(act.Key, act.Val)
	b, _ := h.data.Get(act.Key)
	h.update.Set(act.Key, b)
	return
}
