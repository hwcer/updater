package updater

type ProcessType int8
type ProcessHandle func(*Updater) error

const (
	ProcessTypeInit ProcessType = iota //完成初始化时
	ProcessTypePreData
	ProcessTypePreVerify
	ProcessTypePreSave
	ProcessTypePreSubmit
)

// Listen 全局监听事件
var globalProcessListeners = &process{}

func Listen(t ProcessType, f ProcessHandle) {
	globalProcessListeners.Listen(t, f)
}

// Process 监听updater流程
type process struct {
	listeners map[ProcessType][]ProcessHandle
}

func (this *process) release() {
	this.listeners = nil
}

func (this *process) emit(u *Updater, t ProcessType) (err error) {
	if this.listeners == nil {
		return nil
	}
	for _, f := range this.listeners[t] {
		if err = f(u); err != nil {
			return
		}
	}
	return
}
func (this *process) Listen(t ProcessType, f ProcessHandle) {
	if this.listeners == nil {
		this.listeners = map[ProcessType][]ProcessHandle{}
	}
	this.listeners[t] = append(this.listeners[t], f)
}

func (u *Updater) Listen(t ProcessType, f ProcessHandle) {
	u.process.Listen(t, f)
}

type updaterProcess struct {
	process
}

func (this *updaterProcess) emit(u *Updater, t ProcessType) (err error) {
	if err = globalProcessListeners.emit(u, t); err != nil {
		return
	}
	return this.process.emit(u, t)
}
