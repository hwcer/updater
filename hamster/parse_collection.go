package hamster

import (
	"fmt"

	"github.com/hwcer/updater/dataset"
	"github.com/hwcer/updater/operator"
)

// parse 核心版集合的数据流：Set/Unset/Del/New/Add/Sub 全量在核心，
// 含数值上限（溢出控制，经 Limiter/Overflower 可选接口）与缺失文档生成（DocFactory）。
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
	u := this.statement.Store
	if !this.dataset.Has(op.OID) {
		if !this.model.Upsert(u, op) {
			return ErrItemNotExist(op.OID)
		}
		//Upsert 允许对不存在的文档"改即建"：经 DocFactory 生成初始对象后套用变更
		if f, ok := this.model.(DocFactory); ok {
			obj, err := f.NewDoc(u, op)
			if err != nil {
				return err
			}
			doc := dataset.NewDoc(obj)
			doc.Update(update)
			doc.Save()
			if err := this.dataset.Insert(doc); err != nil {
				return err
			}
			op.OType = operator.TypesNew
			op.Value = doc.GetInt64(this.Field())
			return nil
		}
		return ErrItemNotExist(op.OID)
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
	doc := this.dataset.Val(op.OID)
	if doc == nil {
		return ErrItemNotExist(op.OID)
	}
	//删除数值随通知带给客户端（0 视作 1）
	op.Value = doc.GetInt64(this.Field())
	if op.Value == 0 {
		op.Value = 1
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

// parseAdd 字段级数值增。
// 数值键（IID）非 0 且模型声明多文档（Stacked=false）时：按件经 DocFactory 生成，
// 一件一条 New（装备类语义）；否则定点累加，超限截断（溢出控制），缺失文档可经
// DocFactory 生成后累加。
func (this *Collection) parseAdd(op *operator.Operator) error {
	if op.Value <= 0 {
		return ErrArgsIllegal(op.OID, op.Value)
	}
	u := this.statement.Store
	if op.IID != 0 {
		if st, ok := this.model.(Stacker); ok && !st.Stacked(u, op.IID) {
			f, ok := this.model.(DocFactory)
			if !ok {
				return fmt.Errorf("collection[%s] 多文档 key 需要 DocFactory", this.name)
			}
			n := op.Value
			op.OType = operator.TypesNew
			items := make([]any, 0, n)
			for i := int64(0); i < n; i++ {
				obj, err := f.NewDoc(u, op)
				if err != nil {
					return err
				}
				items = append(items, obj)
			}
			var oid string
			var err error
			op.Result, oid, err = this.InsertValues(items...)
			op.OID = oid
			return err
		}
	}
	if op.OID == "" {
		return ErrObjectIdEmpty(op.IID)
	}
	//数值上限（溢出控制）：分组键优先 IID，缺失文档视为持有 0
	if err := overflowAdd(this.model, u, overflowKey(op), op, func() int64 {
		if doc := this.dataset.Val(op.OID); doc != nil {
			return doc.GetInt64(this.Field())
		}
		return 0
	}); err != nil {
		return err
	}
	if !this.dataset.Has(op.OID) {
		f, ok := this.model.(DocFactory)
		if !ok {
			return ErrItemNotExist(op.OID)
		}
		obj, err := f.NewDoc(u, op)
		if err != nil {
			return err
		}
		if _, _, err := this.InsertValues(obj); err != nil {
			return err
		}
	}
	doc := this.dataset.Val(op.OID)
	r := doc.GetInt64(this.Field()) + op.Value
	if err := this.dataset.Set(op.OID, this.Field(), r); err != nil {
		return err
	}
	op.Result = map[string]any{this.Field(): r}
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
	if d < op.Value && !this.statement.Store.CreditAllowed {
		return ErrNotEnough(op.OID, op.Value, d)
	}
	if err := this.dataset.Set(op.OID, op.Field, r); err != nil {
		return err
	}
	op.Result = map[string]any{op.Field: r}
	return nil
}
