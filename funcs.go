package updater

import (
	"github.com/hwcer/updater/operator"
)

// overflow 仅对 Add/New 操作生效(Types.IsAdd)。当 val+num 超过 IMax 上限时：
// 1. 截断 op.Value 到上限可容纳的量
// 2. 溢出部分交给 ITypeResolve.Resolve 处理（如分解成其他道具）
// 3. 若无 Resolve 实现则生成 TypesOverflow 操作通知前端（可用于邮件等替代发放）
// 4. 截断后 Value==0 时标记为 TypesResolve（不再执行实际 Add/New）
// 5. Resolve 返回的分解材料记进 op.Attach,业务层 Submit 后用 op.GetResolve() 取回「换成了什么」
// 在 Parse 分发前执行,此时 Result 尚未填充,仅调整 op.Value
// overflowHandle overflow 需要的全部能力。
//
// 🔴 不收 Handle：IMax/IType 在那个接口上**只服务于这一个调用点**，
// 而不进 IType 体系的句柄（Mount）被迫实现两个恒空桩才能满足接口，纯属噪音。
// 收窄到这里之后，谁真的参与溢出检查谁才需要它们。
type overflowHandle interface {
	IMax(int32) int64
	IType(int32) IType
	Count(int32) int64
}

func overflow(update *Updater, handle overflowHandle, op *operator.Operator) (err error) {
	if !op.OType.IsAdd() || op.IID == 0 {
		return nil //Document 等按 field 定位的操作 IID 恒为 0,不参与溢出检查
	}
	imax := handle.IMax(op.IID)
	if imax <= 0 {
		return nil //无上限,无需查询持有量
	}
	it := handle.IType(op.IID)
	if it == nil {
		return ErrITypeNotExist(op.IID)
	}

	val := op.Value
	num := handle.Count(op.IID)
	tot := val + num
	if tot <= imax {
		return nil
	}

	n := min(tot-imax, val)
	val -= n
	op.Value = val
	if resolve, ok := it.(ITypeResolve); ok {
		var items map[int32]int64
		if items, err = resolve.Resolve(update, op.IID, n); err != nil {
			return
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
		update.Dirty(ov)
	}
	if val == 0 {
		op.SetOType(operator.TypesResolve)
	}
	return
}
