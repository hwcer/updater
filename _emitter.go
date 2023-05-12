package updater

// Listener 监听,返回false时移除
type Listener func(*Updater) bool

type emitter struct {
	listeners map[EventType][]Listener
}

func (this *emitter) Listen(t EventType, f Listener) {
	if this.listeners == nil {
		this.listeners = map[EventType][]Listener{}
	}
	this.listeners[t] = append(this.listeners[t], f)
}

func (u *Updater) Listen(t EventType, f Listener) {
	u.Emitter.Listen(t, f)
}

func (this *emitter) emit(u *Updater, t EventType) (err error) {
	for _, f := range globalListeners[t] {
		if err = f(u); err != nil {
			return
		}
	}
	for _, f := range this.listeners[t] {
		if err = f(u); err != nil {
			return
		}
	}
	for _, p := range this.plugs {
		if err = p.Emit(u, t); err != nil {
			return
		}
	}
	return
}

func (this *emitter) release() {
	this.plugs = nil
	this.listeners = nil
}
