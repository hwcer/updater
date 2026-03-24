package updater

import (
	"fmt"

	"github.com/hwcer/updater/operator"
)

var mappingParseHandle = make(map[operator.Types]func(*Mapping, *operator.Operator) error)

func init() {
	mappingParseHandle[operator.TypesAdd] = mappingParseAdd
	mappingParseHandle[operator.TypesSet] = mappingParseSet
	mappingParseHandle[operator.TypesSub] = mappingParseSub
	mappingParseHandle[operator.TypesDrop] = mappingParseResolve
	mappingParseHandle[operator.TypesResolve] = mappingParseResolve
}

func (this *Mapping) Parse(op *operator.Operator) (err error) {
	if err = overflow(this.Updater, this, op); err != nil {
		return
	}
	if f, ok := mappingParseHandle[op.Type]; ok {
		return f(this, op)
	}
	return fmt.Errorf("mapping operator type not exist:%v", op.Type.ToString())
}
func mappingParseResolve(this *Mapping, op *operator.Operator) (err error) {
	return
}
func getMappingOperatorKey(op *operator.Operator) any {
	if op.Key == "" {
		return op.Key
	}
	return op.IID
}
func mappingParseAdd(this *Mapping, op *operator.Operator) (err error) {
	k := getMappingOperatorKey(op)
	r := this.Val(k)
	r += op.Value
	op.Result = r
	this.model.Dirty(this.Updater, k, r)
	return
}

func mappingParseSub(this *Mapping, op *operator.Operator) error {
	k := getMappingOperatorKey(op)
	d := this.Val(k)
	r := d - op.Value
	if d < op.Value && !this.Updater.CreditAllowed {
		return ErrItemNotEnough(op.IID, op.Value, d)
	}
	op.Result = r
	this.model.Dirty(this.Updater, k, r)
	return nil
}

func mappingParseSet(this *Mapping, op *operator.Operator) (err error) {
	k := getMappingOperatorKey(op)
	op.Type = operator.TypesSet
	this.model.Dirty(this.Updater, k, op.Result)
	return
}
