package updater

//var db *cosmo.DB
//
//func SetDB(v *cosmo.DB) {
//	db = v
//}

type ikey interface{} //OID或者IID,  int|int32|int64|string
type ival interface{} //int,int32,int64
type ActType uint8    //Cache act type
var (
	ItemNameOID = "_id"
	ItemNameIID = "iid"
	ItemNameVAL = "val"
	ItemNameUID = "uid"
)

const CacheKeyWildcard = "*"

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

func (at ActType) MustSelect() bool {
	return at == ActTypeAdd || at == ActTypeSub || at == ActTypeMax || at == ActTypeMin
}

// MustNumber 必须是正整数的操作
func (at ActType) MustNumber() bool {
	return at == ActTypeAdd || at == ActTypeSub || at == ActTypeMax || at == ActTypeMin
}

func (at ActType) String() string {
	switch at {
	case ActTypeAdd:
		return "Add"
	case ActTypeSub:
		return "Sub"
	case ActTypeSet:
		return "Set"
	case ActTypeDel:
		return "Delete"
	case ActTypeNew:
		return "Create"
	case ActTypeResolve:
		return "Resolve"
	case ActTypeMax:
		return "Max"
	case ActTypeMin:
		return "Min"
	case ActTypeDrop:
		return "Drop"
	default:
		return "unknown"
	}
}

type Cache struct {
	OID   string  `json:"_id"`
	IID   int32   `json:"id"`
	Key   string  `json:"k"`
	Val   any     `json:"v"`
	Ret   any     `json:"r"`
	AType ActType `json:"t"`
	IType IType   `json:"-"`
}

func (this *Cache) GetIType() IType {
	if this.IType == nil {
		k := Config.IType(this.IID)
		this.IType = itypesDict[k]
	}
	return this.IType
}

type Handle interface {
	Del(k ikey)
	Add(k ikey, v ival) //自增v
	Sub(k ikey, v ival) //扣除v
	Max(k ikey, v ival) //如果大于原来的值就写入
	Min(k ikey, v ival) //如果小于于原来的值就写入
	Set(k ikey, v any)
	Get(k ikey) any
	Val(k ikey) int64
	Bind(id ikey, i any) (err error) //数据绑定
	Data() error
	Save() ([]*Cache, error)
	Select(fields ...ikey)
	Verify() error
	reset()
	release()
}

var Config = struct {
	IMax    func(iid int32) int64                                     //通过道具iid查找上限
	IType   func(iid int32) int32                                     //通过道具iid查找IType ID
	ParseId func(adapter *Updater, oid string) (iid int32, err error) //解析OID获得IID
}{}

// IType 一个IType对于一种数据类型·
// 多种数据类型 可以用一种数据模型(model,一张表结构)
type IType interface {
	Id() int32                                                    //IType 唯一标志
	New(u *Updater, act *Cache) (item any, err error)             //生成空对象和默认字段,新对象中必须对oid,uid,iid,val进行赋值
	Unique() bool                                                 //unique=true 一个玩家角色只生成一条数据(可堆叠,oid=uid+iid),unique=false时oid=uid+iid+random
	CreateId(adapter *Updater, iid int32) (oid string, err error) //生成OID或者字段名,hash,Document模式下iid转换成字段名
}

// ITypeResolve 自动分解,如果没有分解方式超出上限则使用系统默认方式（丢弃）处理
// Verify执行的一部分(Data之后Save之前)
// 使用Resolve前，需要使用ITypeListener监听将可能分解成的道具ID使用adapter.Select预读数据
// 使用Resolve时需要关联IMax指定道具上限
type ITypeResolve interface {
	Resolve(adapter *Updater, cache *Cache) error
}

// ITypeListener 数据变化监听器，道具即将改变时(add sub,set,max,min)
// 生成 Cache时执行(Data,Verify,Save之前)
type ITypeListener interface {
	Listener(u *Updater, c *Cache) error
}

func IsIID(k ikey) bool {
	switch k.(type) {
	case int, int32, int64:
		return true
	default:
		return false
	}
}
func IsOID(k ikey) bool {
	switch k.(type) {
	case string:
		return true
	default:
		return false
	}
}
