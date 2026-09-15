package updater

import (
	"github.com/hwcer/updater/dataset"
	"github.com/hwcer/updater/hamster"
	"github.com/hwcer/updater/operator"
)

// overflowItem 溢出检查（仅对 Add/New 且 iid≠0 的操作生效）：
// 1. 截断 op.Value 到 IMax 上限可容纳的量
// 2. 溢出部分交给 ITypeResolve.Resolve 处理（如分解成其他道具）
// 3. 若无 Resolve 则生成 TypesOverflow 操作通知前端（可用于邮件等替代发放）
// 4. 截断后 Value==0 时标记为 TypesResolve（不再执行实际 Add/New）
// 5. Resolve 返回的分解材料记进 op.Attach,业务层 Submit 后用 op.GetResolve() 取回
//
// 句柄并入核心后由 ParseDecorator 钩子调用；countOf 是该 iid 的当前持有量
// （各句柄口径不同：Values/Document 为数值本身，Collection 可叠加点查、不可叠加扫描计数）。
func overflowItem(u *Updater, m any, countOf func(iid int32) int64, op *operator.Operator) error {
	if !op.OType.IsAdd() || op.IID == 0 {
		return nil //Document 等按 field 定位的操作 IID 恒为 0,不参与溢出检查
	}
	return overflowValue(u, modelIType(m, op.IID), func() int64 { return countOf(op.IID) }, op, func() int64 {
		return modelIMax(m, op.IID)
	})
}

// overflowValue 溢出检查的实现体。imaxOf 延迟取上限（调用方可能先行短路）。
func overflowValue(u *Updater, it IType, countOf func() int64, op *operator.Operator, imaxOf func() int64) error {
	imax := imaxOf()
	if imax <= 0 {
		return nil //无上限,无需查询持有量
	}
	if it == nil {
		return ErrITypeNotExist(op.IID)
	}

	val := op.Value
	num := countOf()
	tot := val + num
	if tot <= imax {
		return nil
	}

	n := min(tot-imax, val)
	val -= n
	op.Value = val
	if resolve, ok := it.(ITypeResolve); ok {
		var items map[int32]int64
		var err error
		if items, err = resolve.Resolve(u, op.IID, n); err != nil {
			return err
		}
		n = 0
		//val>0 是「只溢出一部分」,本条仍会真实入袋,不能打成 TypesResolve(那会让整条不落库),
		//故只记材料、不动 OType;val==0 才是整件被分解,交给 SetResolve 一并置位
		if val == 0 {
			op.SetResolve(items)
		} else if len(items) > 0 {
			op.SetAttach(operator.AttachResolve, items)
		}
	}
	if n > 0 {
		ov := operator.New(operator.TypesOverflow, "", n, nil)
		ov.IID = op.IID
		ov.IType = it.ID()
		u.Dirty(ov)
	}
	if val == 0 {
		op.SetOType(operator.TypesResolve)
	}
	return nil
}

// docIID 取文档的 iid（道具路由的统计口径）
// 优先走 dataset.Model 接口(编译期约束),模型未实现时回落到 Fields.IID 字段名约定;
// 两者都取不到时返回 0
func docIID(doc *dataset.Document) int32 {
	if m, ok := doc.Any().(dataset.Model); ok {
		return m.GetIID()
	}
	return doc.GetInt32(dataset.Fields.IID)
}

var _ = hamster.DiscardReceiver
