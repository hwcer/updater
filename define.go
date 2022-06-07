package updater

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
	Key   string      `json:"k"`
	Val   interface{} `json:"v"`
	Bag   int32       `json:"b"`
	Ret   interface{} `json:"r"`
	AType ActType     `json:"t"`
	IType IType       `json:"-"`
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
