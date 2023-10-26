package updater

import (
	"fmt"
	"github.com/hwcer/updater/operator"
)

var hashParseHandle = make(map[operator.Types]func(*Hash, *operator.Operator) error)

func init() {
	//hashParseHandle[operator.TypesDel] = hashParseDel
	hashParseHandle[operator.TypesAdd] = hashParseAdd
	hashParseHandle[operator.TypesSub] = hashParseSub
	hashParseHandle[operator.TypesMax] = hashParseMax
	hashParseHandle[operator.TypesMin] = hashParseMin
	hashParseHandle[operator.TypesSet] = hashParseSet
	hashParseHandle[operator.TypesResolve] = hashParseResolve
}

func (this *Hash) Parse(op *operator.Operator) (err error) {
	if err = overflow(this.Updater, this, op); err != nil {
		return
	}
	if f, ok := hashParseHandle[op.Type]; ok {
		return f(this, op)
	}
	return fmt.Errorf("hash operator type not exist:%v", op.Type.ToString())
}
func hashParseResolve(this *Hash, op *operator.Operator) (err error) {
	return
}

func hashParseAdd(this *Hash, op *operator.Operator) (err error) {
	r := this.val(op.IID) + op.Value
	op.Result = r
	this.values[op.IID] = r
	return
}

func hashParseSub(this *Hash, op *operator.Operator) (err error) {
	d := this.val(op.IID)
	if op.Value > d && this.Updater.strict {
		return ErrItemNotEnough(op.IID, op.Value, d)
	}
	r := d - op.Value
	if r < 0 {
		r = 0
	}
	op.Result = r
	this.values[op.Key] = r
	return
}

func hashParseSet(this *Hash, op *operator.Operator) (err error) {
	op.Type = operator.TypesSet
	this.values[op.IID] = ParseInt64(op.Result)
	return
}

//func hashParseDel(this *Hash, op *operator.Operator) (err error) {
//	op.Result = ZeroInt64
//	this.values[op.IID] = ZeroInt64
//	return
//}

func hashParseMax(this *Hash, op *operator.Operator) (err error) {
	if op.Value > this.val(op.IID) {
		op.Result = op.Value
		err = hashParseSet(this, op)
	} else {
		op.Result = operator.TypesDrop
	}
	return
}

func hashParseMin(this *Hash, op *operator.Operator) (err error) {
	if op.Value > this.val(op.IID) {
		op.Result = op.Value
		err = hashParseSet(this, op)
	} else {
		op.Type = operator.TypesDrop
	}
	return
}
