package updater

import (
	"github.com/hwcer/updater/hamster"
	"github.com/hwcer/updater/operator"
)

// newStatement 构造扩展层句柄的流水线基类。
//
// 句柄以**具名字段** statement hamster.Statement 使用它（不直接嵌入）——
// 一是保持既有代码 this.statement.xxx 的调用形态，二是避免把 Statement 的
// 导出方法提升到句柄公开面上。
//
// 不设接收器：statement 默认把已校验操作插进 store.dirty，
// 与根包的变更流水是同一份（Updaters 内嵌 Store 后 u.dirty 即 store.dirty）。
func newStatement(u *Updater, m *Model, exist func(any) bool) *hamster.Statement {
	return hamster.NewStatement(u.Store, m.ram, exist)
}

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
