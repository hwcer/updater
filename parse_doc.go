package updater

import (
	"fmt"
	"github.com/hwcer/updater/operator"
)

var documentParseHandle = make(map[operator.Types]func(*Document, *operator.Operator) error)

func init() {
	documentParseHandle[operator.Types_Add] = documentParseAdd
	documentParseHandle[operator.Types_Set] = documentParseSet
	documentParseHandle[operator.Types_Sub] = documentParseSub
	documentParseHandle[operator.Types_Max] = documentParseMax
	documentParseHandle[operator.Types_Min] = documentParseMin
}

func (this *Document) Parse(op *operator.Operator) error {
	if f, ok := documentParseHandle[op.Type]; ok {
		return f(this, op)
	}
	return fmt.Errorf("document operator type not exist:%v", op.Type.ToString())
}

func documentParseAdd(this *Document, op *operator.Operator) (err error) {
	r := this.val(op.Key) + op.Value
	op.Result = r
	this.values[op.Key] = r
	return
}

func documentParseSub(this *Document, op *operator.Operator) (err error) {
	d := this.val(op.Key)
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
	op.Type = operator.Types_Set
	if r, ok := TryParseInt64(op.Result); ok {
		this.values[op.Key] = r
	}
	return
}

func documentParseMax(this *Document, op *operator.Operator) (err error) {
	if op.Value > this.val(op.Key) {
		op.Result = op.Value
		err = documentParseSet(this, op)
	} else {
		op.Type = operator.Types_Drop
	}
	return
}

func documentParseMin(this *Document, op *operator.Operator) (err error) {
	if op.Value < this.val(op.Key) {
		op.Result = op.Value
		err = documentParseSet(this, op)
	} else {
		op.Type = operator.Types_Drop
	}
	return
}
