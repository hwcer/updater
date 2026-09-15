package hamster

import (
	"fmt"

	"github.com/hwcer/updater/dataset"
	"github.com/hwcer/updater/operator"
)

// parse 核心版集合只认这六种操作：
// Set/Unset/Del/New 与 Mount 同款；Add/Sub 是字段级数值增减（无溢出检查 —— 核心版没有溢出概念，
// 没有 Drop/Resolve 分支，那是溢出分解的事）。
func (this *Collection) parse(op *operator.Operator) error {
	switch op.OType {
	case operator.TypesSet:
		return this.parseSet(op)
	case operator.TypesUnset:
		return this.parseUnset(op)
	case operator.TypesDel:
		return this.parseDel(op)
	case operator.TypesNew:
		return this.parseNew(op)
	case operator.TypesAdd:
		return this.parseAdd(op)
	case operator.TypesSub:
		return this.parseSub(op)
	}
	return fmt.Errorf("hamster collection[%s] operator type not supported:%v", this.name, op.OType.ToString())
}

func (this *Collection) parseSet(op *operator.Operator) error {
	update, ok := op.Result.(dataset.Update)
	if !ok {
		return ErrArgsIllegal(op.OID, op.Result)
	}
	if !this.dataset.Has(op.OID) {
		if ok := this.model.Upsert(this.Store, op); !ok {
			return ErrItemNotExist(op.OID)
		}
	}
	return this.dataset.Update(op.OID, update)
}

func (this *Collection) parseUnset(op *operator.Operator) error {
	doc := this.dataset.Val(op.OID)
	if doc == nil {
		return ErrItemNotExist(op.OID)
	}
	fields, _ := op.Result.(dataset.Update)
	for k := range fields {
		doc.Unset(k)
	}
	this.dataset.Dirty().Update(op.OID)
	return nil
}

func (this *Collection) parseDel(op *operator.Operator) error {
	if !this.dataset.Has(op.OID) {
		return ErrItemNotExist(op.OID)
	}
	this.dataset.Delete(op.OID)
	return nil
}

func (this *Collection) parseNew(op *operator.Operator) error {
	items, ok := op.Result.([]any)
	if !ok {
		return ErrArgsIllegal(op.OID, op.Result)
	}
	for _, v := range items {
		if err := this.dataset.Insert(v); err != nil {
			return err
		}
	}
	return nil
}

// parseAdd 字段级数值增
func (this *Collection) parseAdd(op *operator.Operator) error {
	if op.Value <= 0 {
		return ErrArgsIllegal(op.OID, op.Value)
	}
	doc := this.dataset.Val(op.OID)
	if doc == nil {
		return ErrItemNotExist(op.OID)
	}
	r := doc.GetInt64(op.Field) + op.Value
	if err := this.dataset.Set(op.OID, op.Field, r); err != nil {
		return err
	}
	op.Result = map[string]any{op.Field: r}
	return nil
}

// parseSub 字段级数值减。余额不足打脏 Error（CreditAllowed 放行扣负）。
func (this *Collection) parseSub(op *operator.Operator) error {
	if op.Value <= 0 {
		return ErrArgsIllegal(op.OID, op.Value)
	}
	doc := this.dataset.Val(op.OID)
	if doc == nil {
		return ErrNotEnough(op.OID, op.Value, 0)
	}
	d := doc.GetInt64(op.Field)
	r := d - op.Value
	if d < op.Value && !this.Store.CreditAllowed {
		return ErrNotEnough(op.OID, op.Value, d)
	}
	if err := this.dataset.Set(op.OID, op.Field, r); err != nil {
		return err
	}
	op.Result = map[string]any{op.Field: r}
	return nil
}
