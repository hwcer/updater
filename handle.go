package updater

import "github.com/hwcer/updater/hamster"

// Handle 数据句柄接口（核心版接口，别名保持 API 冻结）。
//
// 与拆分前的差异：Count(iid)/increase/decrease 三个道具方法已从接口摘除 ——
// 它们只服务溢出检查与 u.Add/Sub 路由，留在具体句柄上、经小接口断言调用
// （与 funcs.go overflowHandle 的收窄同款手术，见 HAMSTER_PLAN.md 第二节 4）。
// 生命周期方法从私有改为导出（Save/Reset/Reload/Loading/Release/Destroy/Commit/Verify）：
// 🔴 Go 的接口未导出方法只能由接口所在包的类型实现，接口要跨包（hamster ↔ updater）
// 实现就必须导出；该接口本来就是密封的（外部无法实现含未导出方法的接口），无人受影响。
type Handle = hamster.Handle

// itemHandle 道具句柄小接口：u.Add/Sub 经 IType 路由后调用
type itemHandle interface {
	increase(k int32, v int64) //由 Updater.Add 调用
	decrease(k int32, v int64) //由 Updater.Sub 调用
}
