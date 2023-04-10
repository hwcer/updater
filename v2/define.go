package updater

import (
	"github.com/hwcer/updater/v2/operator"
)

const ZeroInt64 = int64(0)

type Handle interface {
	Del(k any)            //删除道具
	Get(k any) any        //获取值
	Val(k any) int64      //获取val值
	Add(k int32, v int32) //自增v
	Sub(k int32, v int32) //扣除v
	Max(k int32, v int32) //如果大于原来的值就写入
	Min(k int32, v int32) //如果小于于原来的值就写入
	Set(k any, v ...any)  //设置v值

	Data() error        //非内存模式获取数据库中的数据
	Save() error        //即时同步
	Verify() error      //验证数据
	Select(keys ...any) //非内存模式时获取特定道具
	Parser() Parser     //解析模型

	init() error                  //构造方法
	flush() error                 //析构方法
	reset()                       //运行时开始时
	submit() []*operator.Operator //将执行结果发送给前端
	release()                     //运行时释放缓存信息
}

type HandleNew interface {
	New(op *operator.Operator) error
}

var Config = struct {
	IMax    func(iid int32) int64                                     //通过道具iid查找上限
	IType   func(iid int32) int32                                     //通过道具iid查找IType ID
	ParseId func(adapter *Updater, oid string) (iid int32, err error) //解析OID获得IID
}{}

// IType 一个IType对于一种数据类型·
// 多种数据类型 可以用一种数据模型(model,一张表结构)
type IType interface {
	Id() int32 //IType 唯一标志
}

type ITypeCollection interface {
	IType
	New(u *Updater, op *operator.Operator) (item any, err error) //根据Operator信息生成新对象
	ObjectId(u *Updater, iid int32) (oid string, err error)      //使用IID创建OID
	Multiple() bool                                              //道具是否可以堆叠
}

// ITypeResolve 自动分解,如果没有分解方式超出上限则使用系统默认方式（丢弃）处理
// Verify执行的一部分(Data之后Save之前)
// 使用Resolve前，需要使用ITypeListener监听将可能分解成的道具ID使用adapter.Select预读数据
// 使用Resolve时需要关联IMax指定道具上限
type ITypeResolve interface {
	Resolve(u *Updater, op *operator.Operator) error
}

// ModelIType 获取默认IType,仅仅doc模型使用
//type ModelIType interface {
//	IType() int32
//}

// ModelListener 监听数据变化
type ModelListener interface {
	Listener(u *Updater, op *operator.Operator)
}
