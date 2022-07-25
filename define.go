package updater

/*
UGet 统一返回[]bson.M
*/
type ActType uint8

const (
	ActTypeAdd      ActType = 1  //添加
	ActTypeSub              = 2  //扣除
	ActTypeSet              = 3  //set
	ActTypeDel              = 4  //del
	ActTypeNew              = 5  //新对象
	ActTypeResolve          = 6  //自动分解
	ActTypeOverflow         = 7  //道具已满使用其他方式(邮件)转发
	ActTypeMax              = 8  //最大值写入，最终转换成set或者drop
	ActTypeMin              = 9  //最小值写入，最终转换成set或者drop
	ActTypeDrop             = 99 //抛弃不执行任何操作
)

var (
	ItemNameOID = "_id"
	ItemNameIID = "id"
	ItemNameVAL = "val"
	ItemNameUID = "uid"
)

type Cache struct {
	OID string      `json:"_id"`
	IID int32       `json:"id"`
	Key string      `json:"k"`
	Val interface{} `json:"v"`
	//Bag   int32       `json:"b"`
	Ret   interface{} `json:"r"`
	AType ActType     `json:"t"`
	IType IType       `json:"-"`
}

type Handle interface {
	Add(iid int32, num int32)
	Sub(iid int32, num int32)
	Max(iid int32, num int32) //如果大于原来的值就写入
	Min(iid int32, num int32) //如果小于于原来的值就写入
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
