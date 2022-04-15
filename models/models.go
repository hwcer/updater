package models

import (
	"github.com/hwcer/cosmo/schema"
)

type ParseType int8

const (
	ParseTypeHash  ParseType = 1
	ParseTypeTable           = 2
)

var sort []string
var dict = make(map[string]*Model)

//
//type Interface interface {
//	ObjectID(uid string, iid int32, now time.Time) (oid string, err error) //创建OID
//}

type Model struct {
	Name   string
	Parse  ParseType
	Model  interface{}
	Schema *schema.Schema
}

func Register(pt ParseType, mod interface{}, sc *schema.Schema) {
	i := &Model{Parse: pt, Model: mod, Schema: sc}
	i.Name = sc.Table
	sort = append(sort, i.Name)
	dict[i.Name] = i
}

func Get(name string) *Model {
	return dict[name]
}

func All() (r []string) {
	r = sort[0:]
	return
}
