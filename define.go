package updater

import "time"

/*
UGet 统一返回[]bson.M
*/
type ActType uint8

const (
	ActTypeAdd      ActType = 1 //添加
	ActTypeSub              = 2 //扣除
	ActTypeSet              = 3 //set
	ActTypeDel              = 4 //del
	ActTypeNew              = 5 //新对象
	ActTypeResolve          = 6 //自动分解
	ActTypeOverflow         = 7 //道具已满使用其他方式(邮件)转发
)

var (
	ItemNameOID = "_id"
	ItemNameIID = "id"
	ItemNameVAL = "val"
	ItemNameUID = "uid"
)

type Cache struct {
	OID   string      `json:"_id"`
	IID   int32       `json:"id"`
	AType ActType     `json:"t"`
	Key   string      `json:"k"`
	Val   interface{} `json:"v"`
	//Bag   int32       `json:"b"`
	Ret interface{} `json:"r"`
}

type ModelNew interface {
	New() interface{}
}

type ModelMakeSlice interface {
	MakeSlice() interface{}
}

//获取属性
type ModelGetVal interface {
	GetVal(key string) (interface{}, bool)
}

//设置属性
type ModelSetVal interface {
	SetVal(key string, val interface{}) error
}

//增加属性
type ModelAddVal interface {
	AddVal(key string, val int64) (r int64, err error)
}

//ModelSetOnInert 仅仅HASH需要
type ModelSetOnInert interface {
	SetOnInert(uid string, time time.Time) map[string]interface{}
}

type Handle interface {
	Add(iid int32, num int32)
	Sub(iid int32, num int32)
	Set(id interface{}, val interface{})
	Val(id interface{}) int64
	Get(id interface{}) (r interface{}, ok bool)
	Del(id interface{})
	Keys(id ...interface{})
	Data() error
	Save() ([]*Cache, error)
	Verify() error
	release()
}

//
//type modelHash interface {
//	USet(oid string, update mongo.Update) error     //使用主键更新
//	UGet(oid string, keys []string) (bson.M, error) //使用主键初始化数据
//	NewId(uid string, now time.Time) (oid string, err error)
//}
//
//type modelTable interface {
//	New(uid string, iid int32, val int64, bag int32) (interface{}, error) //新对象
//	UGet(uid string, query mongo.Query) ([]interface{}, error)            //使用主键初始化数据
//	parseHMap(oid string) (iid int32, err error)
//	NewId(uid string, iid int32) (oid string, err error)
//	BulkWrite() *mongo.BulkWrite
//}
//

//

//type binder interface {
//	Keys(keys ...interface{})
//	Fields(keys ...string)
//	Dataset() *Dataset
//	BulkWrite() *mongo.BulkWrite
//}
