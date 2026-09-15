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
// 默认接收器在这里接进 updater.dirty：核心版 statement 的默认行为是插 store.dirty，
// 扩展层的 operator 流水（下发客户端）走根包这份。
func newStatement(u *Updater, m *Model, exist func(any) bool) *hamster.Statement {
	st := hamster.NewStatement(u.store, m.ram, exist)
	st.Receiver(u.pushDirty)
	return st
}

// pushDirty 默认接收器：把已校验操作追加进 Updater.dirty（u.Submit 返回给调用方）。
func (u *Updater) pushDirty(_ *hamster.Store, ops []*operator.Operator) {
	u.dirty = append(u.dirty, ops...)
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
