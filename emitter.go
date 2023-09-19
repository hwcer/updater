package updater

type EventType int32

// listener 返回false时将移除,全局事件无法移除
type listener func(u *Updater, t EventType, v int32, args ...int32) bool

// EmitterValues 第一个
type emitterValues []int32

type emitter struct {
	events    map[EventType][]emitterValues
	listeners map[EventType][]listener
}

// On 添加即时任务类监控
func (u *Updater) On(name EventType, handle listener) {
	u.Emitter.On(name, handle)
}

// Emit Updater.Save之后统一触发
func (u *Updater) Emit(name EventType, v ...int32) {
	u.Emitter.Emit(name, v...)
}

func (e *emitter) On(name EventType, handle listener) {
	if e.listeners == nil {
		e.listeners = map[EventType][]listener{}
	}
	e.listeners[name] = append(e.listeners[name], handle)
}

func (e *emitter) Emit(name EventType, v ...int32) {
	if len(v) == 0 {
		return
	}
	if e.events == nil {
		e.events = map[EventType][]emitterValues{}
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
	for k, v := range e.events {
		next := make([]listener, 0, len(e.listeners[k]))
		for _, h := range e.listeners[k] {
			if e.doTask(u, k, v, h) {
				next = append(next, h)
			}
		}
		e.listeners[k] = next
	}
	return
}

func (e *emitter) doTask(u *Updater, t EventType, vs []emitterValues, h listener) bool {
	for _, v := range vs {
		if !h(u, t, v[0], v[1:]...) {
			return false
		}
	}
	return true
}
