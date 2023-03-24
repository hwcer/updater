package updater

import (
	"fmt"
	"github.com/hwcer/updater/v2/dirty"
)

var hashParseHandle = make(map[dirty.Operator]func(*Hash, *dirty.Cache, int64) error)

func init() {
	hashParseHandle[dirty.OperatorTypeDel] = hashParseDel
	hashParseHandle[dirty.OperatorTypeAdd] = hashParseAdd
	hashParseHandle[dirty.OperatorTypeSub] = hashParseSub
	hashParseHandle[dirty.OperatorTypeMax] = hashParseMax
	hashParseHandle[dirty.OperatorTypeMin] = hashParseMin
	hashParseHandle[dirty.OperatorTypeSet] = hashParseSet
}

func (this *Hash) Parse(act *dirty.Cache) error {
	v := ParseInt64(act.Value)
	if this.Updater.strict && act.Operator == dirty.OperatorTypeSub {
		if d := this.get(act.IID); v > d {
			return ErrItemNotEnough(act.IID, v, d)
		}
	}
	if f, ok := hashParseHandle[act.Operator]; ok {
		return f(this, act, v)
	}
	return fmt.Errorf("map parser not exist:%v", act)
}

func hashParseAdd(this *Hash, act *dirty.Cache, v int64) (err error) {
	act.Result = this.get(act.IID) + v
	return
}

func hashParseSub(this *Hash, act *dirty.Cache, v int64) (err error) {
	act.Result = this.get(act.IID) - v
	return
}

func hashParseMax(this *Hash, act *dirty.Cache, v int64) (err error) {
	if d := this.get(act.IID); v > d {
		act.Result = v
		act.Operator = dirty.OperatorTypeSet
	} else {
		act.Result = dirty.OperatorTypeDrop
	}
	return
}

func hashParseMin(this *Hash, act *dirty.Cache, v int64) (err error) {
	if d := this.get(act.IID); v < d {
		act.Result = v
		act.Operator = dirty.OperatorTypeSet
	} else {
		act.Operator = dirty.OperatorTypeDrop
	}
	return
}

func hashParseSet(_ *Hash, act *dirty.Cache, v int64) (err error) {
	act.Result = v
	return
}
func hashParseDel(this *Hash, act *dirty.Cache, _ int64) (err error) {
	act.Result = 0
	delete(this.dataset, act.IID)
	return
}
