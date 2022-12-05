package updater

import (
	"errors"
)

var documentParseHandle = make(map[ActType]func(*Document, *Cache) error)

func init() {
	documentParseHandle[ActTypeAdd] = documentParseAdd
	documentParseHandle[ActTypeSet] = documentParseSet
	documentParseHandle[ActTypeSub] = documentParseSub
	documentParseHandle[ActTypeMax] = documentParseMax
	documentParseHandle[ActTypeMin] = documentParseMin
	documentParseHandle[ActTypeDel] = documentParseDel
}

func documentParse(doc *Document, act *Cache) error {
	if doc.Adapter.strict && act.AType == ActTypeSub {
		av, _ := ParseInt(act.Val)
		dv := doc.GetInt64(act.Key)
		if av > dv {
			return ErrItemNotEnough(act.IID, av, dv)
		}
	}
	if f, ok := documentParseHandle[act.AType]; ok {
		return f(doc, act)
	}
	return errors.New("hash_act_parser not exist")
}

func documentParseAdd(doc *Document, act *Cache) (err error) {
	if act.Ret, err = doc.Document.Inc(act.Key, act.Val); err == nil {
		doc.update.Inc(act.Key, act.Val)
	}
	return
}

func documentParseSub(doc *Document, act *Cache) (err error) {
	v, _ := ParseInt(act.Val)
	r := -v
	if act.Ret, err = doc.Document.Inc(act.Key, r); err == nil {
		doc.update.Inc(act.Key, r)
	}
	return

}

func documentParseMax(doc *Document, act *Cache) (err error) {
	if act.Ret, err = doc.Document.Max(act.Key, act.Val); err == nil {
		act.AType = ActTypeSet
		err = documentParseSet(doc, act)
	} else {
		act.AType = ActTypeDrop
	}
	return
}

func documentParseMin(doc *Document, act *Cache) (err error) {
	if act.Ret, err = doc.Document.Min(act.Key, act.Val); err == nil {
		act.AType = ActTypeSet
		err = documentParseSet(doc, act)
	} else {
		act.AType = ActTypeDrop
	}
	return
}

func documentParseSet(doc *Document, act *Cache) (err error) {
	act.Ret = act.Val
	if err = doc.Document.Set(act.Key, act.Val); err == nil {
		doc.update.Set(act.Key, act.Val)
	}
	return
}
func documentParseDel(doc *Document, act *Cache) (err error) {
	act.Ret = act.Val
	if err = doc.Document.Unset(act.Key); err == nil {
		doc.update.UnSet(act.Key, act.Val)
	}
	return
}
