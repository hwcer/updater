package updater

import (
	"errors"
	"github.com/hwcer/updater/v2/dirty"
)

var documentParseHandle = make(map[dirty.Operator]func(*Document, *dirty.Cache) error)

func init() {
	documentParseHandle[dirty.OperatorTypeAdd] = documentParseAdd
	documentParseHandle[dirty.OperatorTypeSet] = documentParseSet
	documentParseHandle[dirty.OperatorTypeSub] = documentParseSub
	documentParseHandle[dirty.OperatorTypeMax] = documentParseMax
	documentParseHandle[dirty.OperatorTypeMin] = documentParseMin
}

func (this *Document) Parse(act *dirty.Cache) error {
	if this.Updater.strict && act.Operator == dirty.OperatorTypeSub {
		v, d := ParseInt64(act.Value), this.val(act.Field)
		if v > d {
			return ErrItemNotEnough(act.IID, v, v)
		}
	}
	if f, ok := documentParseHandle[act.Operator]; ok {
		return f(this, act)
	}
	return errors.New("hash_act_parser not exist")
}

func documentParseAdd(doc *Document, act *dirty.Cache) (err error) {
	v, d := ParseInt64(act.Value), doc.val(act.Field)
	act.Result = d + v
	return
}

func documentParseSub(doc *Document, act *dirty.Cache) (err error) {
	v, d := ParseInt64(act.Value), doc.val(act.Field)
	act.Result = d - v
	return
}

func documentParseSet(_ *Document, act *dirty.Cache) (err error) {
	act.Result = act.Value
	return
}

func documentParseMax(doc *Document, act *dirty.Cache) (err error) {
	if v, d := ParseInt64(act.Value), doc.val(act.Field); v > d {
		act.Result = v
		act.Operator = dirty.OperatorTypeSet
	} else {
		act.Operator = dirty.OperatorTypeDrop
	}
	return
}

func documentParseMin(doc *Document, act *dirty.Cache) (err error) {
	if v, d := ParseInt64(act.Value), doc.val(act.Field); v < d {
		act.Result = v
		act.Operator = dirty.OperatorTypeSet
	} else {
		act.Operator = dirty.OperatorTypeDrop
	}
	return
}
