package updater

import (
	"github.com/hwcer/updater/operator"
)

// overflow 仅对 Add 操作生效。当 val+num 超过 IMax 上限时：
// 1. 截断 op.Value 到上限可容纳的量
// 2. 溢出部分交给 ITypeResolve.Resolve 处理（如分解成其他道具）
// 3. 若无 Resolve 实现则生成 TypesOverflow 操作通知前端（可用于邮件等替代发放）
// 4. 截断后 Value==0 时标记为 TypesResolve（不再执行实际 Add）
func overflow(update *Updater, handle Handle, op *operator.Operator) (err error) {
	if op.OType != operator.TypesAdd || op.IID == 0 {
		return nil
	}
	it := handle.IType(op.IID)
	if it == nil {
		return ErrITypeNotExist(op.IID)
	}
	val := op.Value
	num := handle.Val(op.IID)
	tot := val + num
	imax := Config.IMax(op.IID)
	if imax > 0 && tot > imax {
		n := min(tot-imax, val)
		val -= n
		op.Value = val
		if resolve, ok := it.(ITypeResolve); ok {
			if err = resolve.Resolve(update, op.IID, n); err != nil {
				return
			}
			n = 0
		}
		if n > 0 {
			ov := operator.New(operator.TypesOverflow, "", n, nil)
			ov.IID = op.IID
			ov.IType = it.ID()
			update.Dirty(ov)
		}
	}
	if val == 0 {
		op.OType = operator.TypesResolve
	}
	return
}
