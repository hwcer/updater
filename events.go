package updater

type EventType int8

const (
	EventTypeInit    EventType = iota // Loading 完成后触发一次，可用于初始化业务数据
	EventTypeReset                    // Reset 时必定触发，每次请求开始
	EventTypeData                     // Data 阶段前触发，仅 StatusChanged 时执行，可能多次
	EventTypeVerify                   // Verify 阶段前触发，仅 StatusOperated 时执行，可能多次，可安全追加操作
	EventTypeSubmit                   // Submit 收敛循环每轮触发，无错误时至少触发一次，可安全追加操作
	EventTypeSuccess                  // Submit 成功且 BulkWrite 提交后触发，数据已持久化，不可再追加操作
	EventTypeRelease                  // Release 时必定触发，无论成功或失败，需自行检查 Updater.Error
)

// Listener 事件监听器，返回 true 继续监听，false 从列表中移除
type Listener func(u *Updater) (next bool)

// 全局事件，持续触发，不会取消
var globalEvents = map[EventType][]func(u *Updater){}

// RegisterGlobalEvent 注册全局事件，必须在初始化时调用
func RegisterGlobalEvent(t EventType, handle func(u *Updater)) {
	globalEvents[t] = append(globalEvents[t], handle)
}

// Events 生命周期事件
type Events map[EventType][]Listener

func (e Events) On(t EventType, handle Listener) {
	e[t] = append(e[t], handle)
}

func (e Events) emit(u *Updater, t EventType) {
	if u.Error != nil && t != EventTypeRelease {
		return
	}
	for _, h := range globalEvents[t] {
		h(u)
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
