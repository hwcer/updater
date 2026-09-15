package hamster

import (
	"github.com/hwcer/updater/operator"
)

// hamster 核心版：GET/SET/DEL + 脏标记 + 批量落库的文档/集合存储引擎。
// 颊囊预载（内存缓存）→ 囤货入仓（批量落库）→ 记得囤了什么（脏标记）。
// 设计方案见仓库根 HAMSTER_PLAN.md；updater 玩家数据版是其上的道具扩展层。

// Entity 数据属主：玩家 uid / 公会 gid / 临时副本 id。
// 取代扩展层的 Player/Uid 用词 —— 核心版不绑玩家域。
type Entity interface {
	Id() string
}

// Config 核心版全局配置。
//
// 与扩展层 updater.Config{IMax,IType,ParseId} 的分界：这里只允许出现存储概念。
// 🔴 同一进程里若导入了 updater 包，BulkWrite 工厂会被 updater 的桥接接管
// （见 updater 包 init），hamster 独立实例的落库也须经 updater.Config 配置。
var Config = struct {
	BulkWrite func(s *Store) BulkWrite //全局 BulkWrite 工厂
}{}

// Status 状态位标记
type Status uint8

const (
	StatusInit     Status = 1 << iota // 已初始化，按模块预设加载数据
	StatusSubmit                      // 需要触发提交
	StatusChanged                     // 数据变动，需要 Data 更新
	StatusOperated                    // 新操作，需要 Verify 检查
	StatusTesting                     // 测试模式，不写库
	StatusDevelop                     // 开发者模式，业务层自取
)

func (s *Status) Has(flags ...Status) bool {
	for _, f := range flags {
		if *s&f != 0 {
			return true
		}
	}
	return false
}
func (s *Status) Set(flags ...Status) {
	for _, f := range flags {
		*s |= f
	}
}
func (s *Status) Unset(flags ...Status) {
	for _, f := range flags {
		*s &^= f
	}
}

// BulkWrite 跨集合批量写入接口
type BulkWrite interface {
	Submit() error
	Update(model any, data any, where ...any)
	Insert(model any, documents ...any)
	Delete(model any, where ...any)
	String() string
}

// RAMType 内存策略：数据是否驻留内存
type RAMType int8

const (
	RAMTypeNone   RAMType = iota //实时读写数据
	RAMTypeMaybe                 //按需读写
	RAMTypeAlways                //内存运行
)

// Parser 解析器类型枚举。
//
// 核心版只服务 Document/Collection 两类句柄；Values/Virtual 是扩展层（道具）概念，
// 枚举值原样保留作兼容锚点（updater 侧 type 别名引用，数值不得变动 —— 序列化协议稳定性的底线）。
type Parser int8

const (
	ParserTypeValues     Parser = iota //Map[string]int64模式（扩展层）
	ParserTypeDocument                 //Document 单文档模式
	ParserTypeCollection               //Collection 文档集合模式
	ParserTypeVirtual                  //Virtual 虚拟模式（扩展层）
)

// Keys 待拉取 key 集合
type Keys map[any]struct{}

func (this Keys) Has(k any) (ok bool) {
	_, ok = this[k]
	return
}

func (this Keys) Remove(k any) {
	delete(this, k)
}

func (this Keys) ToString() (r []string) {
	for k := range this {
		if sk, ok := k.(string); ok {
			r = append(r, sk)
		}
	}
	return
}

func (this Keys) ToInt32() (r []int32) {
	for k := range this {
		if ik, ok := k.(int32); ok {
			r = append(r, ik)
		}
	}
	return
}

func (this Keys) Merge(src Keys) {
	for k := range src {
		this[k] = struct{}{}
	}
}

func (this Keys) Select(ks ...any) {
	for _, k := range ks {
		this[k] = struct{}{}
	}
}

// DiscardReceiver 不下发变更时用的接收器：把 operator 从"默认进 Store.dirty"那条路上摘下来。
//
// 故意什么都不做、**不 Release**：业务可能刚从 Operators() 里把同一批对象拿在手上。
// 不还池子只是少一点复用，交给 GC 就好 —— 拿"省一次分配"去换悬垂引用不值。
//
// 核心版句柄默认挂这个：hamster 产出的 operator IType 恒 0，客户端按 IType 分发、
// 0 即无主数据（物理事实，见 HAMSTER_PLAN.md 第八节），默认不该进通用更新通道。
// 需要变更记录的业务装自己的接收器，或用 Operators()/Submit() 返回值自组协议。
func DiscardReceiver(*Store, []*operator.Operator) {}
