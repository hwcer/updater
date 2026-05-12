package updater

import (
	"fmt"

	"github.com/hwcer/updater/operator"
)

var documentParseHandle = make(map[operator.Types]func(*Document, *operator.Operator) error)

func init() {
	documentParseHandle[operator.TypesAdd] = documentParseAdd
	documentParseHandle[operator.TypesSet] = documentParseSet
	documentParseHandle[operator.TypesSub] = documentParseSub
	documentParseHandle[operator.TypesDrop] = documentParseResolve
	documentParseHandle[operator.TypesResolve] = documentParseResolve
}

func (this *Document) Parse(op *operator.Operator) (err error) {
	if err = overflow(this.Updater, this, op); err != nil {
		return
	}
	if f, ok := documentParseHandle[op.OType]; ok {
		return f(this, op)
	}
	return fmt.Errorf("document operator type not exist:%v", op.OType.ToString())
}
func documentParseResolve(this *Document, op *operator.Operator) (err error) {
	return
}
func documentParseAdd(this *Document, op *operator.Operator) (err error) {
	if op.Value <= 0 {
		return ErrArgsIllegal(op.IID, op.Value)
	}
	r, _ := this.val(op.Field)
	r += op.Value
	this.dataset.Set(op.Field, r)
	op.Result = map[string]any{op.Field: r}
	return
}

func documentParseSub(this *Document, op *operator.Operator) error {
	if op.Value <= 0 {
		return ErrArgsIllegal(op.IID, op.Value)
	}
	d, _ := this.val(op.Field)
	r := d - op.Value
	if d < op.Value && !this.Updater.CreditAllowed {
		return ErrItemNotEnough(op.IID, op.Value, d)
	}
	this.dataset.Set(op.Field, r)
	op.Result = map[string]any{op.Field: r}
	return nil
}

func documentParseSet(this *Document, op *operator.Operator) (err error) {
	r := op.Result
	this.dataset.Set(op.Field, r)
	op.Result = map[string]any{op.Field: r}
	return
}
