package updater

import "github.com/hwcer/updater/emitter"

// listener 业务逻辑层面普通任务事件,返回false时将移除
//type listener func(u *Updater, t int32, v int32, args ...int32) bool

// EmitterValues 第一个
type emitterValues []int32

type Emitter struct {
	events map[int32][]*emitter.Listener
	values map[int32][]emitterValues
	//listeners map[int32][]listener
}

// On 添加即时任务类监控
//
// handle : process or listener
//func (u *Updater) On(name int32, handle listener) {
//	u.Emitter.On(name, handle)
//}

// Emit Updater.Save之后统一触发
//func (u *Updater) Emit(name int32, v int32, args ...int32) {
//	u.Emitter.Emit(name, v, args...)
//}

//func (u *Updater) Listener(k any, t int32, args []int32, handle events.Handle) *events.Listener {
//	return u.Emitter.Listener(k, t, args, handle)
//}

//func (e *Emitter) On(name int32, handle listener) {
//	if e.listeners == nil {
//		e.listeners = map[int32][]listener{}
//	}
//	e.listeners[name] = append(e.listeners[name], handle)
//}

func (e *Emitter) Emit(name int32, v int32, args ...int32) {
	vs := make([]int32, 0, len(args)+1)
	vs = append(vs, v)
	vs = append(vs, args...)
	if e.values == nil {
		e.values = map[int32][]emitterValues{}
	}
	e.values[name] = append(e.values[name], vs)
}

// Listener 监听事件,并比较args 如果成功,则回调handle更新val
//
// 可以通过 emitter.Register 注册全局过滤器,默认参数一致通过比较
func (e *Emitter) Listener(k any, t int32, args []int32, handle emitter.Handle) (r *emitter.Listener) {
	r = emitter.New(k, args, handle)
	if e.events == nil {
		e.events = map[int32][]*emitter.Listener{}
	}
	e.events[t] = append(e.events[t], r)
	return
}

func (e *Emitter) emit(u *Updater) {
	if len(e.values) == 0 {
		return
	}
	for et, vs := range e.values {
		for _, v := range vs {
			e.doEvents(u, et, v[0], v[1:])
			//e.doListener(u, et, v[0], v[1:])
		}
	}
	e.values = nil
}

// doEvents
func (e *Emitter) doEvents(_ *Updater, t int32, v int32, args []int32) {
	if len(e.events[t]) == 0 {
		return
	}
	var dict []*emitter.Listener
	for _, l := range e.events[t] {
		if l.Handle(t, v, args) {
			dict = append(dict, l)
		}
	}
	e.events[t] = dict
}

// doListener
//func (e *Emitter) doListener(u *Updater, t int32, v int32, args []int32) {
//	if len(e.listeners[t]) == 0 {
//		return
//	}
//	var dict []listener
//	for _, l := range e.listeners[t] {
//		if l(u, t, v, args...) {
//			dict = append(dict, l)
//		}
//	}
//	e.listeners[t] = dict
//
//}
