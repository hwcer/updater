package updater

import (
	"errors"
	"github.com/hwcer/updater/v2/operator"
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
	return errors.New("hash_act_parser not exist")
}

func documentParseAdd(this *Document, op *operator.Operator) (err error) {
	v := ParseInt64(op.Value)
	r := this.val(op.Key) + v
	op.Result = r
	this.values[op.Key] = r
	return
}

func documentParseSub(this *Document, op *operator.Operator) (err error) {
	d := this.val(op.Key)
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

func documentParseSet(this *Document, op *operator.Operator) (err error) {
	op.Type = operator.Types_Set
	op.Result = op.Value
	if r, ok := TryParseInt64(op.Value); ok {
		this.values[op.Key] = r
	}
	return
}

func documentParseMax(this *Document, op *operator.Operator) (err error) {
	v := ParseInt64(op.Value)
	if d := this.val(op.Key); v > d {
		op.Result = v
		this.values[op.Key] = v
	} else {
		op.Type = operator.Types_Drop
	}
	return
}

func documentParseMin(this *Document, op *operator.Operator) (err error) {
	v := ParseInt64(op.Value)
	if d := this.val(op.Key); v < d {
		op.Result = v
		this.values[op.Key] = v
	} else {
		op.Type = operator.Types_Drop
	}
	return
}
