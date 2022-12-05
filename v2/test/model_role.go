package test

import (
	"fmt"
	"github.com/hwcer/adapter"
	"github.com/hwcer/cosmo/update"
	"github.com/hwcer/logger"
)

var ITypeRole = &iTypeRole{iType: iType{id: 11, unique: true}, fields: map[int32]string{}}

func init() {
	ITypeRole.Register(1100, "uid")
	ITypeRole.Register(1101, "name")
	ITypeRole.Register(1102, "level")
	ITypeRole.Register(1103, "money")
	if err := updater.Register(&Role{}, ITypeRole); err != nil {
		fmt.Printf("%v\n", err)
	}
}

type Role struct {
	Uid   string `bson:"uid"`
	Name  string `bson:"name"`
	Level int32  `bson:"level"`
	Money int64  `bson:"money"`
}

func (this *Role) Parser() updater.Parser {
	return updater.ParserTypeDocument
}
func (this *Role) Getter(adapter *updater.Updater, keys []string) (any, error) {
	r := &Role{Uid: adapter.Uid(), Name: "test"}
	return r, nil
}

func (this *Role) Setter(_ *updater.Updater, update update.Update) error {
	logger.Info("role Setter:%+v", update)
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
	this.iType.Register(iid)
	this.fields[iid] = key
}
