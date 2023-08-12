package updater

type EventType int32

// listener 返回false时将移除,全局事件无法移除
type listener func(*Updater, any) bool

type emitter struct {
	events    map[EventType][]any
	listeners map[EventType][]listener
}

// On 添加即时任务类监控
func (u *Updater) On(name EventType, handle listener) {
	u.Emitter.On(name, handle)
}

// Emit Updater.Save之后统一触发
func (u *Updater) Emit(name EventType, v any) {
	u.Emitter.Emit(name, v)
}

func (e *emitter) On(name EventType, handle listener) {
	if e.listeners == nil {
		e.listeners = map[EventType][]listener{}
	}
	e.listeners[name] = append(e.listeners[name], handle)
}

func (e *emitter) Emit(name EventType, v any) {
	if e.events == nil {
		e.events = map[EventType][]any{}
	}
	e.events[name] = append(e.events[name], v)
}

func (e *emitter) emit(u *Updater, t PlugsType) (err error) {
	if t == PlugsTypeRelease {
		defer func() {
			e.events = nil
		}()
	}
	if t != PlugsTypeSubmit || len(e.events) == 0 || len(e.listeners) == 0 {
		return
	}
	for name, args := range e.events {
		next := make([]listener, 0, len(e.listeners[name]))
		for _, handle := range e.listeners[name] {
			if e.doTask(u, handle, args) {
				next = append(next, handle)
			}
		}
		e.listeners[name] = next
	}
	return
}

func (e *emitter) doTask(u *Updater, handle listener, args []any) bool {
	for _, arg := range args {
		if !handle(u, arg) {
			return false
		}
	}
	return true
}
