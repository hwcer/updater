package updater

import (
	"errors"
	"github.com/hwcer/updater/bson"
)

var hashParseHandle = make(map[ActType]func(*Hash, *Cache, int64) error)

func init() {
	hashParseHandle[ActTypeDel] = hashParseDel
	hashParseHandle[ActTypeAdd] = hashParseAdd
	hashParseHandle[ActTypeSub] = hashParseSub
	hashParseHandle[ActTypeMax] = hashParseMax
	hashParseHandle[ActTypeMin] = hashParseMin
	hashParseHandle[ActTypeSet] = hashParseSet

}

func hashParse(hash *Hash, act *Cache) error {
	v := bson.ParseInt64(act.Val)
	if hash.Adapter.strict && act.AType == ActTypeSub {
		dv := hash.Dataset.Get(act.Key)
		if v > dv {
			return ErrItemNotEnough(act.IID, v, dv)
		}
	}
	if f, ok := hashParseHandle[act.AType]; ok {
		return f(hash, act, v)
	}
	return errors.New("hash_act_parser not exist")
}

func hashParseAdd(hash *Hash, act *Cache, v int64) (err error) {
	hash.update.Inc(act.Key, v)
	act.Ret = hash.Dataset.Inc(act.Key, v)
	return
}

func hashParseSub(hash *Hash, act *Cache, v int64) (err error) {
	r := -v
	hash.update.Inc(act.Key, r)
	act.Ret = hash.Dataset.Inc(act.Key, r)
	return

}

func hashParseMax(hash *Hash, act *Cache, v int64) (err error) {
	if act.Ret, err = hash.Dataset.Max(act.Key, v); err == nil {
		act.AType = ActTypeSet
		err = hashParseSet(hash, act, v)
	} else {
		act.AType = ActTypeDrop
	}
	return
}

func hashParseMin(hash *Hash, act *Cache, v int64) (err error) {
	if act.Ret, err = hash.Dataset.Min(act.Key, v); err == nil {
		act.AType = ActTypeSet
		err = hashParseSet(hash, act, v)
	} else {
		act.AType = ActTypeDrop
	}
	return
}

func hashParseSet(hash *Hash, act *Cache, v int64) (err error) {
	act.Ret = v
	hash.update.Set(act.Key, v)
	hash.Dataset.Set(act.Key, v)
	return
}
func hashParseDel(hash *Hash, act *Cache, _ int64) (err error) {
	hash.update.UnSet(act.Key, 1)
	hash.Dataset.Del(act.Key)
	return
}
