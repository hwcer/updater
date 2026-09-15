package hamster

import (
	"fmt"

	"github.com/hwcer/updater/operator"
)

// valuesParseHandle 核心版 Values 的操作分发表。
// 与扩展层的差异：入口没有 overflow（核心版没有溢出概念），没有 Drop/Resolve 分支。
var valuesParseHandle = make(map[operator.Types]func(*Values, *operator.Operator) error)

func init() {
	valuesParseHandle[operator.TypesAdd] = valuesParseAdd
	valuesParseHandle[operator.TypesSub] = valuesParseSub
	valuesParseHandle[operator.TypesSet] = valuesParseSet
	valuesParseHandle[operator.TypesUnset] = valuesParseUnset
}

func (this *Values) Parse(op *operator.Operator) (err error) {
	if f, ok := valuesParseHandle[op.OType]; ok {
		return f(this, op)
	}
	return fmt.Errorf("values operator type not exist:%v", op.OType.ToString())
}

func valuesParseAdd(this *Values, op *operator.Operator) (err error) {
	if op.Value <= 0 {
		return ErrArgsIllegal(op.IID, op.Value)
	}
	r := this.dataset.Val(op.IID) + op.Value
	op.Result = map[int32]int64{op.IID: r}
	this.dataset.Set(op.IID, r)
	return
}

func valuesParseSub(this *Values, op *operator.Operator) error {
	if op.Value <= 0 {
		return ErrArgsIllegal(op.IID, op.Value)
	}
	d := this.dataset.Val(op.IID)
	r := d - op.Value
	if d < op.Value && !this.Store.CreditAllowed {
		return ErrNotEnough(op.IID, op.Value, d)
	}
	op.Result = map[int32]int64{op.IID: r}
	this.dataset.Set(op.IID, r)
	return nil
}

func valuesParseSet(this *Values, op *operator.Operator) (err error) {
	r := op.Value
	op.Result = map[int32]int64{op.IID: r}
	this.dataset.Set(op.IID, r)
	return
}

func valuesParseUnset(this *Values, op *operator.Operator) (err error) {
	op.Result = map[int32]int64{op.IID: 0}
	this.dataset.Unset(op.IID)
	return
}
