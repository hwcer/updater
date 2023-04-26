package model

import (
	"github.com/hwcer/updater"
	"github.com/hwcer/updater/demo/config"
	"github.com/hwcer/updater/operator"
)

var EquipIType = &equipIType{}

func init() {
	EquipIType.id = config.ITypeEquip
	ItemIType.register(EquipIType)
}

type equipIType struct {
	IType
	seed int32
}

func (this *equipIType) New(u *updater.Updater, op *operator.Operator) (any, error) {
	i := &Item{}
	if oid, err := ObjectId(u, op.IID, true); err != nil {
		return nil, err
	} else {
		i.OID = oid
	}
	i.Uid = u.Uid()
	i.IID = op.IID
	i.Val = op.Value
	//i.Attach = "" todo
	return i, nil
}

func (this *equipIType) ObjectId(u *updater.Updater, iid int32) (string, error) {
	return "", nil
}

// Listener 自动分解前使用 Select(碎片ID) 预加载碎片信息
func (this *equipIType) Listener(u *updater.Updater, op *operator.Operator) {
	if op.Type == operator.Types_Add {
		//u.Select(1111)  碎片ID
	}
}
