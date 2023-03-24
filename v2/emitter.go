package updater

import "github.com/hwcer/updater"

type EventType uint8

const (
	EventTypeTaskDailySubmit EventType = 1
	EventTypeTaskRecord      EventType = 2
	EventTypeLeagueUpdate    EventType = 3
	EventTypeItemAdd                   = 4 // id,num
	EventTypeItemSub                   = 5 // id,num
)

type EventHandle func(*Updater, ...int32)

var events = make(map[EventType][]EventHandle)

// On 全局监控对象
func On(name EventType, fn EventHandle) {
	events[name] = append(events[name], fn)
}

// Listener 返回false时将移除
type Listener func(*Updater, ...int32) bool

func (u *Updater) trigger(_ *updater.Updater, cache *updater.Cache) {
	if cache.AType == updater.ActTypeSub {
		u.Emit(EventTypeItemSub, cache.IID, cache.Val.(int32))
	} else if cache.AType == updater.ActTypeAdd {
		u.Emit(EventTypeItemAdd, cache.IID, cache.Val.(int32))
	}
}

func (u *Updater) onAfterSave(_ *updater.Updater) error {
	for name, args := range u.emitter {
		//全局事件
		for _, handle := range events[name] {
			for _, v := range args {
				handle(u, v...)
			}
		}
		//即时事件
		var end bool
		var listener []Listener
		for _, handle := range u.listener[name] {
			end = false
			for _, v := range args {
				if !handle(u, v...) {
					end = true
					break
				}
			}
			if !end {
				listener = append(listener, handle)
			}
		}
		u.listener[name] = listener
	}
	u.emitter = map[EventType]emitterArgs{}
	return nil
}

// Emit Updater.Save之后统一触发
func (u *Updater) Emit(name EventType, args ...int32) {
	if len(u.emitter) == 0 {
		u.Updater.On(updater.EventsTypeFinishSave, u.onAfterSave)
	}
	u.emitter[name] = append(u.emitter[name], args)
}

// AddListener 添加即时任务类监控
func (this *Updater) AddListener(name EventType, handle Listener) {
	if this.listener == nil {
		this.listener = map[EventType][]Listener{}
	}
	this.listener[name] = append(this.listener[name], handle)
}

//func Emit(name uint8, u *updater2.Updater, id int32, val int32) {
//	if funcs, ok := events[name]; ok {
//		u.On(updater2.EventsTypeBeforeData, func(*updater2.Updater) error {
//			for _, f := range funcs {
//				if err := f(u, id, val); err != nil {
//					return err
//				}
//			}
//			return nil
//		})
//	}
//}
