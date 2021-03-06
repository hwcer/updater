package updater

import (
	"github.com/hwcer/cosmo"
	"github.com/hwcer/cosmo/schema"
)

type ParseType int8

const (
	ParseTypeHash  ParseType = iota //HASH模式
	ParseTypeTable                  //table模式
)

var modelsRank []*Model
var modelsDict = make(map[string]*Model)

//type ModelInterface interface {
//	ObjectID(uid string, iid int32, now time.Time) (oid string, err error) //创建OID
//}

type Model struct {
	Name   string
	Parse  ParseType
	Model  interface{}
	Schema *schema.Schema
}

func Register(pt ParseType, mod interface{}) (err error) {
	i := &Model{Parse: pt, Model: mod}
	i.Schema, err = schema.Parse(mod, cosmo.Options)
	if err != nil {
		return
	}
	i.Name = i.Schema.Table
	modelsRank = append(modelsRank, i)
	modelsDict[i.Name] = i
	return nil
}

//func (this *Model) ObjectID(u *Updater, iid int32) (string, error) {
//	return this.Model.ObjectID(u.Uid(), iid, u.Time())
//}
