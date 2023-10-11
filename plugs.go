package updater

type PlugsType int8

const (
	PlugsTypeInit    PlugsType = iota //初始化后
	PlugsTypeData                     //Data前
	PlugsTypeVerify                   //verify前
	PlugsTypeSubmit                   //submit 前
	PlugsTypeSuccess                  //全部执行结束
	PlugsTypeRelease                  //Release 释放前
	PlugsTypeDestroy                  //销毁前,需要实例化数据
)

type Event func(u *Updater) error
type Process interface {
	Emit(u *Updater, t PlugsType) error
}

type Plugs struct {
	events    map[PlugsType][]Event
	processes map[string]Process
}

func (this *Plugs) On(t PlugsType, handle Event) {
	if this.events == nil {
		this.events = map[PlugsType][]Event{}
	}
	this.events[t] = append(this.events[t], handle)
}

// Get 获取plug
func (this *Plugs) Get(name string) any {
	if this.processes != nil && this.processes[name] != nil {
		return this.processes[name]
	}
	return nil
}

// Set 设置插件,如果已存在(全局,当前)返回false
func (this *Plugs) Set(name string, handle Process) bool {
	if this.processes == nil {
		this.processes = map[string]Process{}
	}
	if _, ok := this.processes[name]; ok {
		return false
	}
	this.processes[name] = handle
	return true
}

func (this *Plugs) LoadOrStore(name string, handle Process) (v Process) {
	if this.processes == nil {
		this.processes = map[string]Process{}
	}
	if v = this.processes[name]; v == nil {
		v = handle
		this.processes[name] = handle
	}
	return
}

func (this *Plugs) LoadOrCreate(name string, creator func() Process) (v Process) {
	if this.processes == nil {
		this.processes = map[string]Process{}
	}
	if v = this.processes[name]; v == nil {
		v = creator()
		this.processes[name] = v
	}
	return
}

func (this *Plugs) emit(u *Updater, t PlugsType) (err error) {
	if t == PlugsTypeRelease {
		defer func() {
			this.events = nil
			this.processes = nil
		}()
	}

	if u.Error != nil {
		return
	}
	//通用事件

	for _, h := range this.events[t] {
		if err = h(u); err != nil {
			return
		}
	}

	for _, p := range this.processes {
		if err = p.Emit(u, t); err != nil {
			return
		}
	}

	return
}
