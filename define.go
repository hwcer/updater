package updater

import (
	"github.com/hwcer/updater/hamster"
	"github.com/hwcer/updater/operator"
)

// Config 全局配置（扩展层）。
//
// IMax/IType/ParseId 是道具概念，归扩展层；BulkWrite 是核心存储概念，
// hamster 侧经 init 桥接转发（见本文件底部），用户注册代码不变。
var Config = struct {
	IMax      func(iid int32) int64                                     //通过道具iid查找上限
	IType     func(iid int32) int32                                     //通过道具iid查找IType ID
	ParseId   func(adapter *Updater, oid string) (iid int32, err error) //解析OID获得IID
	BulkWrite func(u *Updater) BulkWrite                                //全局 BulkWrite 工厂
}{}

func init() {
	// 🔴 BulkWrite 桥接：hamster.Store.BulkWrite() 走 hamster.Config，
	// 这里把它指回 updater.Config（用户配置点不变）。工厂在调用时才解引用，
	// 所以 Config.BulkWrite 的赋值时机不受 init 顺序影响。
	// ⚠️ 同一进程里导入了 updater 后，hamster 独立实例的落库也须经 updater.Config 配置
	// （bridge 找不到对应 Updater 时返回 nil，触发 ErrBulkWriteNotInit 而不是静默失效）。
	hamster.Config.BulkWrite = func(s *hamster.Store) hamster.BulkWrite {
		if Config.BulkWrite == nil {
			return nil
		}
		u := updaterOf(s)
		if u == nil {
			return nil
		}
		return Config.BulkWrite(u)
	}
}

// Status 状态位标记（核心版类型，别名保持 API 冻结）
type Status = hamster.Status

const (
	StatusInit     = hamster.StatusInit     // 已初始化，按模块预设加载数据
	StatusSubmit   = hamster.StatusSubmit   // 需要触发提交
	StatusChanged  = hamster.StatusChanged  // 数据变动，需要 Data 更新
	StatusOperated = hamster.StatusOperated // 新操作，需要 Verify 检查
	StatusTesting  = hamster.StatusTesting  // 测试模式，不写库
	StatusDevelop  = hamster.StatusDevelop  // 开发者模式，业务层自取
)

// RAMType 内存策略（核心版类型，别名保持 API 冻结）
type RAMType = hamster.RAMType

const (
	RAMTypeNone   = hamster.RAMTypeNone   //实时读写数据
	RAMTypeMaybe  = hamster.RAMTypeMaybe  //按需读写
	RAMTypeAlways = hamster.RAMTypeAlways //内存运行
)

// BulkWrite 跨集合批量写入接口（核心版类型，别名保持 API 冻结）
type BulkWrite = hamster.BulkWrite

// Keys 待拉取 key 集合（核心版类型，别名保持 API 冻结）
type Keys = hamster.Keys

// Parser 解析器类型（核心版枚举，别名保持数值不变）
type Parser = hamster.Parser

const (
	ParserTypeValues     = hamster.ParserTypeValues     //Map[string]int64模式
	ParserTypeDocument   = hamster.ParserTypeDocument   //Document 单文档模式
	ParserTypeCollection = hamster.ParserTypeCollection //Collection 文档集合模式
	ParserTypeVirtual    = hamster.ParserTypeVirtual    //Virtual 虚拟模式,本身不会存储数据，依赖于其他模块数据
)

// TableOrder 可选接口:控制模型加载顺序,值大的先加载（主档模式支柱，见 HAMSTER_PLAN.md D6）
type TableOrder = hamster.TableOrder

// IType 一个IType对于一种数据类型·
// 多种数据类型 可以用一种数据模型(model,一张表结构)
type IType interface {
	ID() int32 //IType 唯一标志
}
type ITypeOID interface {
	GetOID(u *Updater, iid int32) (oid string) //使用IID创建OID,仅限于可以叠加道具,不可以叠加道具返回空,使用NEW来创建
}

type ITypeCollection interface {
	IType
	ITypeOID
	New(u *Updater, op *operator.Operator) (item any, err error) //根据Operator信息生成新对象
	Stacked(int32) bool                                          //是否可以叠加
}

// ITypeResolve 自动分解,如果没有分解方式超出上限则使用系统默认方式（丢弃）处理
// Verify执行的一部分(Data之后Save之前)
// 使用Resolve前，需要使用ITypeListener监听将可能分解成的道具ID使用adapter.Select预读数据
// 使用Resolve时需要关联IMax指定道具上限
//
// 返回值为本次分解产出的材料(key 道具ID,value 数量),发放仍由实现方自己 u.Add;
// 框架只负责把它记进 op.Attach[operator.AttachResolve],供业务层在 Submit 后读取
// (op.GetResolve()),不必再写一份「纯查询版」重算产物。无产物返回 nil 即可。
type ITypeResolve interface {
	Resolve(u *Updater, iid int32, val int64) (map[int32]int64, error)
}

// ITypeResult 设置返回结果
type ITypeResult interface {
	Result(u *Updater, opt *operator.Operator) any
}
type ITypeListener interface {
	Listener(u *Updater, op *operator.Operator)
}
