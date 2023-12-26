package updater

import (
	"fmt"
	"github.com/hwcer/updater/dataset"
	"github.com/hwcer/updater/operator"
)

var documentParseHandle = make(map[operator.Types]func(*Document, *operator.Operator) error)

func init() {
	documentParseHandle[operator.TypesAdd] = documentParseAdd
	documentParseHandle[operator.TypesSet] = documentParseSet
	documentParseHandle[operator.TypesSub] = documentParseSub
	documentParseHandle[operator.TypesMax] = documentParseMax
	documentParseHandle[operator.TypesMin] = documentParseMin
	documentParseHandle[operator.TypesDrop] = documentParseResolve
	documentParseHandle[operator.TypesResolve] = documentParseResolve
}

func (this *Document) Parse(op *operator.Operator) (err error) {
	if err = overflow(this.Updater, this, op); err != nil {
		return
	}
	if f, ok := documentParseHandle[op.Type]; ok {
		return f(this, op)
	}
	return fmt.Errorf("document operator type not exist:%v", op.Type.ToString())
}
func documentParseResolve(this *Document, op *operator.Operator) (err error) {
	return
}
func documentParseAdd(this *Document, op *operator.Operator) (err error) {
	r, _ := this.val(op.Key)
	r += op.Value
	op.Result = r
	this.values[op.Key] = r
	return
}

func documentParseSub(this *Document, op *operator.Operator) (err error) {
	d, _ := this.val(op.Key)
	if op.Value > d && this.Updater.strict {
		return ErrItemNotEnough(op.Key, op.Value, d)
	}
	r := d - op.Value
	if r < 0 {
		r = 0
	}
	op.Result = r
	this.values[op.Key] = r
	return
}

func documentParseSet(this *Document, op *operator.Operator) (err error) {
	op.Type = operator.TypesSet
	if r, ok := dataset.TryParseInt64(op.Result); ok {
		this.values[op.Key] = r
	}
	return
}

func documentParseMax(this *Document, op *operator.Operator) (err error) {
	v, _ := this.val(op.Key)
	if op.Value > v {
		op.Result = op.Value
		err = documentParseSet(this, op)
	} else {
		op.Type = operator.TypesDrop
	}
	return
}

func documentParseMin(this *Document, op *operator.Operator) (err error) {
	v, _ := this.val(op.Key)
	if op.Value < v {
		op.Result = op.Value
		err = documentParseSet(this, op)
	} else {
		op.Type = operator.TypesDrop
	}
	return
}
