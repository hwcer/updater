package updater

import (
	"github.com/hwcer/cosmo"
)

var db *cosmo.DB

func SetDB(v *cosmo.DB) {
	db = v
}

//一个IType对于一种数据模型
type IType struct {
	Bag      int32                                                          //准备废除
	Model    string                                                         //对应数据库model名字(Table Name)
	Unique   bool                                                           //unique=true 一个玩家角色只生成一条数据(可堆叠)
	Resolve  func(id int32, num int32) (newId int32, newNum int32, ok bool) //分解方式,如果没有分解方式超出上限则使用系统默认方式（邮件）处理
	OnCreate func(*Updater, interface{})                                    //生成新道具时
	OnChange func(u *Updater, iid int32, num int32)                         //道具即将改变时(add sub),num 是负数为扣除
}

var Config = struct {
	IMax  func(iid int32) int64
	IType func(iid int32) *IType       //通过道具ID查找数据模型
	Field func(iid int32) (key string) //hash模式下IID转换成对象属性名
}{}

var ObjectID = struct {
	Parse  func(oid string) (iid int32, err error)                  //解析OID获得IID
	Create func(u *Updater, iid int32, unique bool) (string, error) //通过IID获取OID,参考 IType.Unique
}{}
