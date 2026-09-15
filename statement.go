package updater

import (
	"github.com/hwcer/updater/hamster"
	"github.com/hwcer/updater/operator"
)

// itemResultFill handleResult 钩子：按 op.IType 查注册表填充 ITypeResult.Result。
//
// 🔴 这是拆包第一硬阻（statement 基类硬引用 itypesDict）的解法：机制回调扩展层
// 一律走构造期注入的函数字段，机制不得反向引用道具注册表（HAMSTER_PLAN.md 第六节③）。
func itemResultFill(s *hamster.Store, op *operator.Operator) {
	it := itypesDict[op.IType]
	if it == nil {
		return
	}
	itr, ok := it.(ITypeResult)
	if !ok {
		return
	}
	if u := updaterOf(s); u != nil {
		op.Result = itr.Result(u, op)
	}
}
