package updater

type EventsType int32
type EventsHandle func(adapter *Updater) error

const (
	EventsPreData   EventsType = iota //执行data前
	EventsPreVerify                   //执行Verify前
	EventsPreSubmit                   //执行Save前
)

//EventsTypeOverflow
//
////全局事件
//var Events = NewEvents()
//
//func NewEvents() *events {
//	return &events{dict: make(map[EventsType][]EventsHandle)}
//}
//
//type events struct {
//	dict map[EventsType][]EventsHandle
//}
//
//func (e *events) On(t EventsType, f EventsHandle) {
//	e.dict[t] = append(e.dict[t], f)
//}
//
//func (e *events) Emit(u *Updater, t EventsType) {
//	for _, f := range e.dict[t] {
//		f(u)
//	}
//}
