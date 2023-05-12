package updater

type PlugsType int8

const (
	PlugsTypeInit    PlugsType = iota //初始化后
	PlugsTypeData                     //Data前
	PlugsTypeVerify                   //verify前
	PlugsTypeSubmit                   //submit 前
	PlugsTypeRelease                  //Release 释放前
	PlugsTypeDestroy                  //销毁前,需要实例化数据
)

// plugs 每个EventType 调用一次
type plugs interface {
	Emit(u *Updater, t PlugsType) error
}

type NewPlugsHandle func(updater *Updater) plugs

var globalPlugs = map[string]NewPlugsHandle{}

func AddGlobalPlugs(name string, handle NewPlugsHandle) {
	globalPlugs[name] = handle
}

type Plugs struct {
	plugs  map[string]plugs
	global map[string]plugs
}

func (this *Plugs) Get(name string) any {
	if this.global != nil && this.global[name] != nil {
		return this.global[name]
	}
	if this.plugs != nil && this.plugs[name] != nil {
		return this.plugs[name]
	}
	return nil
}

// Set 设置插件,如果已存在(全局,当前)返回false
func (this *Plugs) Set(name string, handle plugs) bool {
	if this.global != nil && this.global[name] != nil {
		return false
	}
	if this.plugs != nil && this.plugs[name] != nil {
		return false
	}
	if this.plugs == nil {
		this.plugs = map[string]plugs{}
	}
	this.plugs[name] = handle
	return true
}

func (this *Plugs) LoadOrStore(name string, handle plugs) any {
	if this.global != nil && this.global[name] != nil {
		return this.global[name]
	}
	if this.plugs != nil && this.plugs[name] != nil {
		return this.plugs[name]
	}
	if this.plugs == nil {
		this.plugs = map[string]plugs{}
	}
	this.plugs[name] = handle
	return handle
}

func (this *Plugs) emit(u *Updater, t PlugsType) (err error) {
	if t == PlugsTypeRelease {
		defer func() {
			this.plugs = nil
		}()
	}

	if t == PlugsTypeInit && len(globalPlugs) > 0 {
		this.global = map[string]plugs{}
		for k, f := range globalPlugs {
			this.global[k] = f(u)
		}
	}

	if u.Error != nil {
		return
	}
	for _, p := range this.global {
		if err = p.Emit(u, t); err != nil {
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
