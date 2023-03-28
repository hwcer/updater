package updater

type plugsType int8
type plugsHandle func(*Updater) error

const (
	PlugsTypePreData plugsType = iota
	PlugsTypePreVerify
	PlugsTypePreSave
	PlugsTypePreSubmit
)

func (u *Updater) doPlugs(t plugsType) (err error) {
	if u.plugs == nil {
		return nil
	}
	for _, f := range u.plugs[t] {
		if err = f(u); err != nil {
			return
		}
	}
	return
}
func (u *Updater) Plugs(t plugsType, f plugsHandle) {
	if u.plugs == nil {
		u.plugs = map[plugsType][]plugsHandle{}
	}
	u.plugs[t] = append(u.plugs[t], f)
}

// PreData 拉取数据前
func (u *Updater) PreData(f plugsHandle) {
	u.Plugs(PlugsTypePreData, f)
}

// PreVerify 数据检查前
func (u *Updater) PreVerify(f plugsHandle) {
	u.Plugs(PlugsTypePreVerify, f)
}

// PreSave 保存数据前
func (u *Updater) PreSave(f plugsHandle) {
	u.Plugs(PlugsTypePreSave, f)
}

// PreSubmit 返回到客户端前
func (u *Updater) PreSubmit(f plugsHandle) {
	u.Plugs(PlugsTypePreSubmit, f)
}
