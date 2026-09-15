package hamster

import (
	"fmt"

	"github.com/hwcer/updater/operator"
)

// documentParseHandle 核心版 Document 的操作分发表。
// 与扩展层的差异：入口**没有 overflow**（核心版没有溢出概念），
// 没有 Drop/Resolve 分支（那是溢出分解）。
var documentParseHandle = make(map[operator.Types]func(*Document, *operator.Operator) error)

func init() {
	documentParseHandle[operator.TypesAdd] = documentParseAdd
	documentParseHandle[operator.TypesSet] = documentParseSet
	documentParseHandle[operator.TypesSub] = documentParseSub
	documentParseHandle[operator.TypesUnset] = documentParseUnset
}

func (this *Document) Parse(op *operator.Operator) (err error) {
	if f, ok := documentParseHandle[op.OType]; ok {
		return f(this, op)
	}
	return fmt.Errorf("document operator type not exist:%v", op.OType.ToString())
}

func documentParseAdd(this *Document, op *operator.Operator) (err error) {
	if op.Value <= 0 {
		return ErrArgsIllegal(op.Field, op.Value)
	}
	r, _ := this.val(op.Field)
	r += op.Value
	this.dataset.Set(op.Field, r)
	op.Result = map[string]any{op.Field: r}
	return
}

func documentParseSub(this *Document, op *operator.Operator) error {
	if op.Value <= 0 {
		return ErrArgsIllegal(op.Field, op.Value)
	}
	d, _ := this.val(op.Field)
	r := d - op.Value
	if d < op.Value && !this.statement.Store.CreditAllowed {
		return ErrNotEnough(op.Field, op.Value, d)
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

func documentParseUnset(this *Document, op *operator.Operator) (err error) {
	this.dataset.Unset(op.Field)
	op.Result = map[string]any{op.Field: nil}
	return
}
