package updater

// EventType int16：负数分段——-1~-7 请求生命周期（本文件），-100~-104 框架玩家
// 生命周期（yyds players/events.go），业务自定义系统事件再往下取段（如 -200，
// int8 装不下才用 int16；纯运行时 map 键，无存储/序列化依赖）。
type EventType int16

// EventFunc 全局事件无法移除
type EventFunc func(u *Updater)

const (
	EventTypeLoaded  EventType = -iota // 从数据库完整初始化成功时抛出：首次 Loading、懒加载转正（tryLoad）、Reload。按需拉取的局部数据（RAMTypeNone / 懒加载下 Select+Data）与跨天 ModelReset 重载不算；懒加载实例不抛。依赖整档数据的初始化（建索引、存量对账）挂这里，须能承受重复触发（Reload 会再抛，重建前先清空）
	EventTypeReset                     // Reset 时必定触发，每次请求开始
	EventTypeData                      // Data 阶段前触发，仅 StatusChanged 时执行，可能多次
	EventTypeVerify                    // Verify 阶段前触发，仅 StatusOperated 时执行，可能多次，可安全追加操作
	EventTypeSubmit                    // Submit 收敛循环每轮触发，无错误时至少触发一次，可安全追加操作
	EventTypeSuccess                   // Submit 成功且 BulkWrite 提交后触发，数据已持久化，不可再追加操作
	EventTypeRelease                   // Release 时必定触发，无论成功或失败，需自行检查 Updater.Error
	EventTypeInit                      // 句柄与域级缓存创建完成（首次 Loading，在线/懒加载都发一次）。🔴 不保证数据已加载（懒加载此时为空），要数据挂 EventTypeLoaded
)

// Listener 事件监听器，返回 true 继续监听，false 从列表中移除
type Listener func(u *Updater) (next bool)

// Events 生命周期事件
type Events map[EventType][]Listener

func (e Events) On(t EventType, handle Listener) {
	e[t] = append(e[t], handle)
}

func (e Events) Emit(u *Updater, t EventType) {
	//错误闸门只作用于请求生命周期事件（Reset~Success）：错误一旦发生，请求流程
	//中断，后续生命周期事件本来就走不到。系统事件（调用方自定义段，如玩家生命
	//周期 players/events.go 的 -100 段）在请求流程之外，不受任何 Error 影响。
	if u.Error != nil && t > EventTypeRelease && t <= EventTypeLoaded {
		return
	}
	//域级事件（原包级全局事件）：对该域所有实例生效，永不取消
	if u.manage != nil {
		for _, h := range u.manage.events[t] {
			h(u)
		}
	}
	events := e[t]
	if len(events) > 0 {
		es := make([]Listener, 0, len(events))
		for _, h := range events {
			if h(u) {
				es = append(es, h)
			}
		}
		e[t] = es
	}
}
