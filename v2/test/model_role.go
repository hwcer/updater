package test

import (
	"fmt"
	"github.com/hwcer/updater/v2"
)

var ITypeRole = &iTypeRole{iType: iType{id: 11, unique: true}, fields: map[int32]string{}}

func init() {
	ITypeRole.Register(1100, "uid")
	ITypeRole.Register(1101, "name")
	ITypeRole.Register(1102, "level")
	ITypeRole.Register(1103, "money")
	if err := updater.Register(updater.ParserTypeDocument, updater.RAMTypeAlways, &Role{}, ITypeRole); err != nil {
		fmt.Printf("%v\n", err)
	}
}

type Role struct {
	Uid    string `bson:"uid"`
	Name   string `bson:"name"`
	Level  int32  `bson:"level"`
	Money  int64  `bson:"money"`
	Online int64  `bson:"online"` //累计在线时间
}

func (this *Role) Model(u *updater.Updater) any {
	return &Role{}
}

func (this *Role) Getter(update *updater.Updater, model any, keys []string) error {
	fmt.Printf("====== Role Getter:%v\n", keys)
	return nil
}

func (this *Role) Setter(update *updater.Updater, model any, data map[string]any) error {
	fmt.Printf("====== Role Setter:%v\n", data)
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
