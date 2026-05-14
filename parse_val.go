package updater

import (
	"fmt"

	"github.com/hwcer/updater/operator"
)

var hashParseHandle = make(map[operator.Types]func(*Values, *operator.Operator) error)

func init() {
	hashParseHandle[operator.TypesAdd] = hashParseAdd
	hashParseHandle[operator.TypesSub] = hashParseSub
	hashParseHandle[operator.TypesSet] = hashParseSet
	hashParseHandle[operator.TypesDel] = hashParseDel
	hashParseHandle[operator.TypesDrop] = hashParseResolve
	hashParseHandle[operator.TypesResolve] = hashParseResolve
}

func (this *Values) Parse(op *operator.Operator) (err error) {
	if err = overflow(this.Updater, this, op); err != nil {
		return
	}
	if f, ok := hashParseHandle[op.OType]; ok {
		return f(this, op)
	}
	return fmt.Errorf("hash operator type not exist:%v", op.OType.ToString())
}
func hashParseResolve(this *Values, op *operator.Operator) (err error) {
	return
}

func hashParseAdd(this *Values, op *operator.Operator) (err error) {
	if op.Value <= 0 {
		return ErrArgsIllegal(op.IID, op.Value)
	}
	r := this.dataset.Val(op.IID) + op.Value
	op.Result = map[int32]int64{op.IID: r}
	this.dataset.Set(op.IID, r)
	return
}

func hashParseSub(this *Values, op *operator.Operator) error {
	if op.Value <= 0 {
		return ErrArgsIllegal(op.IID, op.Value)
	}
	d := this.dataset.Val(op.IID)
	r := d - op.Value
	if d < op.Value && !this.Updater.CreditAllowed {
		return ErrItemNotEnough(op.IID, op.Value, d)
	}
	op.Result = map[int32]int64{op.IID: r}
	this.dataset.Set(op.IID, r)
	return nil
}

func hashParseSet(this *Values, op *operator.Operator) (err error) {
	r := op.Value
	op.Result = map[int32]int64{op.IID: r}
	this.dataset.Set(op.IID, r)
	return
}

func hashParseDel(this *Values, op *operator.Operator) (err error) {
	op.Result = map[int32]int64{op.IID: 0}
	this.dataset.Set(op.IID, 0)
	return
}
