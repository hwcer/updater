package updater

import (
	"fmt"
	"github.com/hwcer/updater/v2/operator"
)

var hashParseHandle = make(map[operator.Types]func(*Hash, *operator.Operator) error)

func init() {
	hashParseHandle[operator.Types_Del] = hashParseDel
	hashParseHandle[operator.Types_Add] = hashParseAdd
	hashParseHandle[operator.Types_Sub] = hashParseSub
	hashParseHandle[operator.Types_Max] = hashParseMax
	hashParseHandle[operator.Types_Min] = hashParseMin
	hashParseHandle[operator.Types_Set] = hashParseSet
}

func (this *Hash) Parse(op *operator.Operator) error {
	if f, ok := hashParseHandle[op.Type]; ok {
		return f(this, op)
	}
	return fmt.Errorf("map parser not exist:%v", op)
}

func hashParseAdd(this *Hash, op *operator.Operator) (err error) {
	v := ParseInt64(op.Value)
	r := this.val(op.IID) + v
	op.Result = r
	this.values[op.IID] = r
	return
}

func hashParseSub(this *Hash, op *operator.Operator) (err error) {
	d := this.val(op.IID)
	v := ParseInt64(op.Value)
	if v > d {
		if this.Updater.tolerate {
			v = d
		} else {
			err = ErrItemNotEnough(op.IID, v, d)
		}
		return
	}
	if d <= 0 {
		op.Type = operator.Types_Drop
	} else {
		r := d - v
		op.Result = r
		this.values[op.Key] = r
	}
	return
}

func hashParseSet(this *Hash, op *operator.Operator) (err error) {
	op.Type = operator.Types_Set
	v := ParseInt64(op.Value)
	op.Result = v
	this.values[op.IID] = v
	return
}

func hashParseDel(this *Hash, cache *operator.Operator) (err error) {
	cache.Result = ZeroInt64
	this.values[cache.IID] = ZeroInt64
	return
}

func hashParseMax(this *Hash, op *operator.Operator) (err error) {
	v := ParseInt64(op.Value)
	if d := this.val(op.IID); v > d {
		err = hashParseSet(this, op)
	} else {
		op.Result = operator.Types_Drop
	}
	return
}

func hashParseMin(this *Hash, op *operator.Operator) (err error) {
	v := ParseInt64(op.Value)
	if d := this.val(op.IID); v < d {
		err = hashParseSet(this, op)
	} else {
		op.Type = operator.Types_Drop
	}
	return
}
