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

//ModelHash Hash必须有具备的方法
type ModelHash interface {
	New() interface{}
	ObjectId(u *Updater) string //HASH KEY
}

//ModelTable Table必须具备的方法
type ModelTable interface {
	Copy() interface{}
	MakeSlice() interface{} //[]ModelTable
}

//ModelGetVal 获取属性
type ModelGetVal interface {
	GetVal(key string) (interface{}, bool)
}

//ModelSetVal 设置属性
type ModelSetVal interface {
	SetVal(key string, val interface{}) error
}

//ModelAddVal 增加属性
type ModelAddVal interface {
	AddVal(key string, val int64) (r int64, err error)
}
