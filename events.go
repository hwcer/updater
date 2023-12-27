package updater

type EventType int8

const (
	EventTypePreData    EventType = iota //Data前
	EventTypePreVerify                   //verify前
	EventTypePreSubmit                   //submit 前
	EventTypePreRelease                  //Release 释放前
)

type Event func(u *Updater) bool

type Middleware interface {
	Emit(u *Updater, t EventType) bool
}

type Events struct {
	events      map[EventType][]Event
	middlewares map[string]Middleware
}

func (e *Events) On(t EventType, handle Event) {
	if e.events == nil {
		e.events = map[EventType][]Event{}
	}
	e.events[t] = append(e.events[t], handle)
}

// Get 获取plug
func (e *Events) Get(name string) any {
	if e.middlewares != nil && e.middlewares[name] != nil {
		return e.middlewares[name]
	}
	return nil
}

// Set 设置插件,如果已存在(全局,当前)返回false
func (e *Events) Set(name string, handle Middleware) bool {
	if e.middlewares == nil {
		e.middlewares = map[string]Middleware{}
	}
	if _, ok := e.middlewares[name]; ok {
		return false
	}
	e.middlewares[name] = handle
	return true
}

func (e *Events) LoadOrStore(name string, handle Middleware) (v Middleware) {
	if e.middlewares == nil {
		e.middlewares = map[string]Middleware{}
	}
	if v = e.middlewares[name]; v == nil {
		v = handle
		e.middlewares[name] = handle
	}
	return
}

func (e *Events) LoadOrCreate(name string, creator func() Middleware) (v Middleware) {
	if e.middlewares == nil {
		e.middlewares = map[string]Middleware{}
	}
	if v = e.middlewares[name]; v == nil {
		v = creator()
		e.middlewares[name] = v
	}
	return
}

func (e *Events) emit(u *Updater, t EventType) {
	if u.Error != nil {
		return
	}
	e.emitEvents(u, t)
	e.emitMiddleware(u, t)
}

func (e *Events) release() {
	e.events = nil
	e.middlewares = nil
}

func (e *Events) emitMiddleware(u *Updater, t EventType) {
	if len(e.middlewares) == 0 {
		return
	}
	mw := map[string]Middleware{}
	for k, p := range e.middlewares {
		if p.Emit(u, t) {
			mw[k] = p
		}
	}
	e.middlewares = mw
}

func (e *Events) emitEvents(u *Updater, t EventType) {
	if events := e.events[t]; len(events) > 0 {
		var es []Event
		for _, h := range events {
			if h(u) {
				es = append(es, h)
			}
		}
		e.events[t] = es
	}
	return
}
