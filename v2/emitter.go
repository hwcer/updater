package updater

type EventType int32
type EventArgs map[any]any

var events = make(map[EventType][]listener)

// listener 返回false时将移除,全局事件无法移除
type listener func(*Updater, EventArgs) bool

type emitter struct {
	events    map[EventType][]EventArgs
	listeners map[EventType][]listener
}

// On 全局监控对象,在文件init()中注册避免并发
func On(name EventType, fn listener) {
	events[name] = append(events[name], fn)
}

// On 添加即时任务类监控
func (u *Updater) On(name EventType, handle listener) {
	if u.emitter.listeners == nil {
		u.emitter.listeners = map[EventType][]listener{}
	}
	u.emitter.listeners[name] = append(u.emitter.listeners[name], handle)
}

// Emit Updater.Save之后统一触发
func (u *Updater) Emit(name EventType, args EventArgs) {
	if u.emitter.events == nil {
		u.emitter.events = map[EventType][]EventArgs{}
	}
	u.emitter.events[name] = append(u.emitter.events[name], args)
}

func (u *Updater) doEvents() {
	if len(u.emitter.events) == 0 {
		return
	}
	for name, args := range u.emitter.events {
		//全局事件
		for _, handle := range events[name] {
			for _, arg := range args {
				handle(u, arg)
			}
		}
		//即时事件
		var next []listener
		for _, handle := range u.emitter.listeners[name] {
			for _, arg := range args {
				if handle(u, arg) {
					next = append(next, handle)
				} else {
					break
				}
			}
		}
		u.emitter.listeners[name] = next
	}
	u.emitter.events = nil
}
