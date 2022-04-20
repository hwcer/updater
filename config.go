package updater

import "github.com/hwcer/cosmo"

var db *cosmo.DB

func SetDB(v *cosmo.DB) {
	db = v
}

//一个IType对于一种数据模型
type IType interface {
	Model() string                                          //对应数据库model名字(Table Name)
	Stackable() bool                                        //unique=true 一个玩家角色只生成一条数据(可堆叠)
	CreateId(u *Updater, iid int32) (oid string, err error) //生成OID或者字段名
}

//ITypeResolve 分解方式,如果没有分解方式超出上限则使用系统默认方式（邮件）处理
type ITypeResolve interface {
	Resolve(id int32, num int32) (newId int32, newNum int32, ok bool)
}

//ITypeOnCreate 生成新道具时
type ITypeOnCreate interface {
	OnCreate(u *Updater, item interface{})
}

//ITypeOnChange 道具即将改变时(add sub),num 是负数为扣除
type ITypeOnChange interface {
	OnChange(u *Updater, iid int32, num int32)
}

var Config = struct {
	IMax    func(iid int32) int64
	IType   func(iid int32) IType                   //通过道具ID查找数据模型
	ParseId func(oid string) (iid int32, err error) //解析OID获得IID
}{}
