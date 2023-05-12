package demo

import (
	"fmt"
	"github.com/hwcer/updater"
	"github.com/hwcer/updater/demo/model"
)

func init() {
	if err := updater.Register(updater.ParserTypeDocument, updater.RAMTypeAlways, &Role{}, model.RoleIType); err != nil {
		fmt.Printf("%v\n", err)
	}
}

type Role struct {
}

func (this *Role) Field(_ *updater.Updater, iid int32) (string, error) {
	if v, ok := model.RoleIType.Fields[iid]; ok {
		return v, nil
	}
	return "", fmt.Errorf("iid对应的字段不存在:%v", iid)
}

func (this *Role) New(u *updater.Updater) any {
	if p := Players.LoadWithUnlock(u.Uid()); p != nil {
		return p.Role
	} else {
		fmt.Printf("role.New(%v) player不存在\n", u.Uid())
		return nil
	}
}

func (this *Role) Getter(update *updater.Updater, doc any, keys []string) error {
	fmt.Printf("====== Role Getter:%v\n", keys)
	if v, ok := doc.(*model.Role); ok {
		v.Name = "天空一声巨响,老子闪亮登场"
		v.Level = 100
		v.Money = 999999
	}
	return nil
}

func (this *Role) Setter(update *updater.Updater, doc any, data map[string]any) error {
	fmt.Printf("====== Role Setter:%v\n", data)
	return nil
}
