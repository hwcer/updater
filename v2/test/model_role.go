package test

import (
	"fmt"
	"github.com/hwcer/updater/v2"
)

var roleData = &Role{}

var ITypeRole = &iTypeRole{iType: iType{id: 11, unique: true}, fields: map[int32]string{}}

func init() {
	ITypeRole.Register(1100, "uid")
	ITypeRole.Register(1101, "name")
	ITypeRole.Register(1102, "level")
	ITypeRole.Register(1103, "money")
	if err := updater.Register(updater.ParserTypeDocument, updater.RAMTypeAlways, roleData, ITypeRole); err != nil {
		fmt.Printf("%v\n", err)
	}
}

type Role struct {
	Uid   string `bson:"uid"`
	Name  string `bson:"name"`
	Level int32  `bson:"level"`
	Money int64  `bson:"money"`
}

func (this *Role) Init(u *updater.Updater, init bool) (any, error) {
	if init {
		roleData.Uid = u.Uid()
		roleData.Name = "test"
	}
	return roleData, nil
}

func (this *Role) Getter(update *updater.Updater, model any, keys []string) error {
	fmt.Printf("需要从DB中获取以下值填充到model.Role:%v\n", keys)
	return nil
}

func (this *Role) Setter(update *updater.Updater, model any, data map[string]any) error {
	fmt.Printf("需要将以下值保存到db.Role:%v\n", data)
	return nil
}

type iTypeRole struct {
	iType
	fields map[int32]string
}

func (this *iTypeRole) CreateId(_ *updater.Updater, iid int32) (string, error) {
	if oid, ok := this.fields[iid]; ok {
		return oid, nil
	}
	return "", fmt.Errorf("未知的IID:%v", iid)
}

func (this *iTypeRole) Register(iid int32, key string) {
	this.fields[iid] = key
}
