package model

import (
	"github.com/hwcer/updater/demo/config"
)

var RoleIType = &roleIType{IType: *NewIType(config.ITypeRole), Fields: map[int32]string{}}

func init() {
	RoleIType.Register(1000, "uid")
	RoleIType.Register(1001, "name")
	RoleIType.Register(1002, "level")
	RoleIType.Register(1003, "money")
}

type Role struct {
	Id    string `bson:"_id" json:"id"`
	Guid  string `bson:"guid" bson:"guid"`
	Name  string `bson:"name"`
	Level int32  `bson:"level"`
	Money int64  `bson:"money"`
}

type roleIType struct {
	IType
	Fields map[int32]string
}

func (this *roleIType) Register(iid int32, key string) {
	this.Fields[iid] = key
	config.Register(iid, config.ITypeRole, 0)
}
