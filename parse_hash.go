package updater

import (
	"errors"
)

var hashParseHandle = make(map[ActType]func(*Hash, *Cache) error)

func init() {
	hashParseHandle[ActTypeAdd] = hashHandleAdd
	hashParseHandle[ActTypeSet] = hashHandleSet
	hashParseHandle[ActTypeSub] = hashHandleSub
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

func hashHandleSet(h *Hash, act *Cache) (err error) {
	act.Ret = act.Val
	h.data.Set(act.Key, act.Val)
	h.update.Set(act.Key, act.Val)
	return
}
