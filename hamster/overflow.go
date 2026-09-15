package hamster

import (
	"github.com/hwcer/updater/dataset"
	"github.com/hwcer/updater/operator"
)

// 数值上限（溢出控制）：Add 类操作落账前的可选上限校验。
//
// 模型实现 Limiter 提供上限（0=不限）、Overflower 提供超限转化物
// （键值对原样记进 op 附件，op.GetResolve() 取回，发放/消费由上层负责）；
// 未实现 Overflower 时仅截断，并产一条 TypesOverflow 通知进变更流水。
// key 形态随句柄：Document 为字段名、Values 为数值键、Collection 为分组键。

// overflowKey 溢出分组键：优先数值键（IID），否则 OID
func overflowKey(op *operator.Operator) any {
	if op.IID != 0 {
		return op.IID
	}
	return op.OID
}

// overflowAdd 对 Add 类操作做上限截断。count 返回该 key 的当前持有量。
func overflowAdd(m any, s *Store, key any, op *operator.Operator, count func() int64) error {
	lim, ok := m.(Limiter)
	if !ok {
		return nil
	}
	imax := lim.Limit(s, key)
	if imax <= 0 {
		return nil //无上限
	}
	tot := op.Value + count()
	if tot <= imax {
		return nil
	}
	n := min(tot-imax, op.Value)
	op.Value -= n
	if of, ok := m.(Overflower); ok {
		items, err := of.Overflow(s, key, n)
		if err != nil {
			return err
		}
		n = 0
		//op.Value>0 是「只溢出一部分」，本条仍真实入账，只记转化物；
		//op.Value==0 才是整笔被转化，SetResolve 一并置位（整条不再执行实际新增）
		if op.Value == 0 {
			op.SetResolve(items)
		} else if len(items) > 0 {
			op.SetAttach(operator.AttachResolve, items)
		}
	}
	if n > 0 {
		ov := operator.New(operator.TypesOverflow, "", n, nil)
		if iid, ok := key.(int32); ok {
			ov.IID = iid
		}
		s.Dirty(ov)
	}
	if op.Value == 0 {
		op.SetOType(operator.TypesResolve)
	}
	return nil
}

// docIID 取文档的数值分组键：优先 dataset.Model.GetIID()，回落 Fields.IID 字段名约定
func docIID(doc *dataset.Document) int32 {
	if m, ok := doc.Any().(dataset.Model); ok {
		return m.GetIID()
	}
	return doc.GetInt32(dataset.Fields.IID)
}
