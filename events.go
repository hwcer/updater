package updater

type EventsType int32
type EventsHandle func(*Updater)

const (
	EventsTypeBeforeData EventsType = iota
	EventsTypeFinishData
	EventsTypeBeforeVerify
	EventsTypeFinishVerify
	EventsTypeBeforeSave
	EventsTypeFinishSave
)

var Events = &events{dict: make(map[EventsType][]EventsHandle)}

type events struct {
	dict map[EventsType][]EventsHandle
}

func (e *events) On(t EventsType, f EventsHandle) {
	e.dict[t] = append(e.dict[t], f)
}

func (e *events) Emit(u *Updater, t EventsType) {
	for _, f := range e.dict[t] {
		f(u)
	}
}
