package updater

import (
	"github.com/coreos/etcd/proxy/grpcproxy/cache"
	"github.com/hwcer/updater/v2/dirty"
)

//
////var db *cosmo.DB
////
////func SetDB(v *cosmo.DB) {
////	db = v
////}
//
////type ikey interface{} //OID或者IID,  int|int32|int64|string
////type ival interface{} //int,int32,int64
//
//type ActType uint8 //Cache act type
////var (
////	ItemNameOID = "_id"
////	ItemNameIID = "iid"
////	ItemNameVAL = "val"
////	ItemNameUID = "uid"
////)
////
////const CacheKeyWildcard = "*"
//
//const (
//	ActTypeAdd      ActType = 1  //添加
//	ActTypeSub              = 2  //扣除
//	ActTypeSet              = 3  //set
//	ActTypeDel              = 4  //del
//	ActTypeNew              = 5  //新对象
//	ActTypeResolve          = 6  //自动分解
//	ActTypeOverflow         = 7  //道具已满使用其他方式(邮件)转发
//	ActTypeMax              = 8  //最大值写入，最终转换成set或者drop
//	ActTypeMin              = 9  //最小值写入，最终转换成set或者drop
//	ActTypeDrop             = 99 //抛弃不执行任何操作
//)
//
//func (at ActType) isValid() bool {
//	return at == ActTypeAdd || at == ActTypeSub || at == ActTypeSet || at == ActTypeDel || at == ActTypeNew
//}
//
//func (at ActType) MustSelect() bool {
//	return at == ActTypeAdd || at == ActTypeSub || at == ActTypeMax || at == ActTypeMin
//}
//
//// MustNumber 必须是正整数的操作
//func (at ActType) MustNumber() bool {
//	return at == ActTypeAdd || at == ActTypeSub || at == ActTypeMax || at == ActTypeMin
//}
//
//func (at ActType) ToString() string {
//	switch at {
//	case ActTypeAdd:
//		return "Add"
//	case ActTypeSub:
//		return "Sub"
//	case ActTypeSet:
//		return "Set"
//	case ActTypeDel:
//		return "Delete"
//	case ActTypeNew:
//		return "Create"
//	case ActTypeResolve:
//		return "Resolve"
//	case ActTypeMax:
//		return "Max"
//	case ActTypeMin:
//		return "Min"
//	case ActTypeDrop:
//		return "Drop"
//	default:
//		return "unknown"
//	}
//}
//
//type cacheDict map[any]any
//
//type Cache struct {
//	OID   string  `json:"_id"`
//	IID   int32   `json:"id"`
//	Field   string  `json:"k"`
//	Val   any     `json:"v"`
//	Ret   any     `json:"r"`
//	AType ActType `json:"t"`
//	IType IType   `json:"-"`
//
//	update    map[string]any
//	effective bool //立即生效,仅在需要最终一致时使用,比如体力自动回复
//}
//
//func (this *Cache) Update() map[string]any {
//	if this.update == nil {
//		this.update = ParseMap(this.Field, this.Val)
//	}
//	return this.update
//}
//
//func (this *Cache) GetIType() IType {
//	if this.IType == nil {
//		k := Config.IType(this.IID)
//		this.IType = itypesDict[k]
//	}
//	return this.IType
//}

// Effective 设置成立即生效
//func (this *Cache) Effective() {
//	this.effective = true
//}

type Handle interface {
	Del(k any)            //删除道具
	Get(k any) any        //获取值
	Val(k any) int64      //获取val值
	Add(k int32, v int32) //自增v
	Sub(k int32, v int32) //扣除v
	Max(k int32, v int64) //如果大于原来的值就写入
	Min(k int32, v int64) //如果小于于原来的值就写入
	Set(k any, v ...any)  //设置v值

	Data() error        //非内存模式获取数据库中的数据
	Save() error        //即时同步
	Verify() error      //验证数据
	Select(keys ...any) //非内存模式时获取特定道具

	reset()          //运行时开始时
	release()        //运行时释放缓存信息
	destruct() error //关闭
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
	Unique() bool                                                 //unique=true 一个玩家角色只生成一条数据(可堆叠,oid=uid+iid),unique=false时oid=uid+iid+random
	CreateId(adapter *Updater, iid int32) (oid string, err error) //使用IID创建OID,或者查找Field
}
type ITypeDocument interface {
	IType
}
type ITypeCollection interface {
	IType
	New(u *Updater, act *cache.Cache) (item any, err error) //生成空对象和默认字段,新对象中必须对oid,uid,iid,val进行赋值
}

// ITypeResolve 自动分解,如果没有分解方式超出上限则使用系统默认方式（丢弃）处理
// Verify执行的一部分(Data之后Save之前)
// 使用Resolve前，需要使用ITypeListener监听将可能分解成的道具ID使用adapter.Select预读数据
// 使用Resolve时需要关联IMax指定道具上限
type ITypeResolve interface {
	Resolve(adapter *Updater, cache *dirty.Cache) error
}

// ITypeListener 数据变化监听器，道具即将改变时(add sub,set,max,min)
// 生成 Cache时执行(Data,Verify,Save之前)
type ITypeListener interface {
	Listener(u *Updater, c *dirty.Cache) error
}

//func IsIID(k any) bool {
//	switch k.(type) {
//	case int, int32, int64:
//		return true
//	default:
//		return false
//	}
//}
//func IsOID(k any) bool {
//	switch k.(type) {
//	case string:
//		return true
//	default:
//		return false
//	}
//}
