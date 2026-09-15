package hamster

import (
	"fmt"
	"sort"
	"time"

	"github.com/hwcer/updater/operator"
)

// TableOrder 可选接口：控制模型在 Loading 阶段的加载顺序，值大的先加载。
//
// 主档模式的支柱：主档模型返回最大值（或 0），其余模型返回负值，
// 保证主档先于依赖它的集合加载完成（HAMSTER_PLAN.md 第四节 D6）。
type TableOrder interface {
	TableOrder() int32
}

// ---- 可选注入接口（扩展层的道具语义从这里进入核心句柄；核心自身不实现任何一个）----
//
// 它们由模型/适配器在注册时以"可选接口"形式提供，句柄构造时做一次类型断言捕获。
// 核心代码只认这几个中性名字，任何 iid/IType/溢出词汇出现在核心即设计漂移。

// Keyer 可选接口：key 解析。默认实现是"string 直通"；
// 注入后非 string 的 key（如道具 iid）经它换算成字段名/OID。
type Keyer interface {
	Key(s *Store, k any) (string, error)
}

// OperatorDecorator 可选接口：operator 构造后的装饰（填 IType、换算 OID、预读监听、
// 校验拦截等）。返回 false 表示**丢弃该操作**（含静默丢弃场景），装饰方自行负责打脏错误。
type OperatorDecorator interface {
	DecorateOperator(s *Store, op *operator.Operator) (keep bool)
}

// ParseDecorator 可选接口：parse 分发前的钩子（溢出检查/装备生成分支等）。
// h 是本操作所属句柄（钩子可取数据/写内存）。handled=true 表示钩子已自行处理该操作，
// 核心分发表跳过；handled=false 且 err==nil 表示仅调整（如溢出截断 op.Value），继续核心分发。
type ParseDecorator interface {
	DecorateParse(h Handle, s *Store, op *operator.Operator) (handled bool, err error)
}

// ModelReset 可选接口：返回 true 时重新调用 model.Getter（跨天/跨周重置）。
// 第二个参数是【上次请求的时间】用于跨天判定，零值表示本 Store 实例尚未处理过请求。
//
// 🔴 实现方必须加编译期断言 `var _ hamster.ModelReset = (*YourModel)(nil)`：
// 本接口靠类型断言识别，签名写错不会编译报错，只会让 Reset 永不被调用（静默失效）。
type ModelReset interface {
	Reset(s *Store, last time.Time) bool
}

// HandleFactory 句柄工厂：两层注册表的唯一合法接缝。
//
// 核心版用默认工厂（RegisterDocument/RegisterCollection 内部构造）；
// 扩展层注册时传入自己的工厂，在工厂内创建道具句柄并注入 handleResult 等钩子。
type HandleFactory func(s *Store, m *Model) Handle

// Model 已注册模型的元数据。
type Model struct {
	ram     RAMType
	name    string
	model   any
	parser  Parser //元数据/兼容锚点，核心版不参与路由
	order   int32  //倒序排列
	factory HandleFactory
}

// Name 模型名（注册名，通常为表名）
func (m *Model) Name() string { return m.name }

// RAM 内存策略
func (m *Model) RAM() RAMType { return m.ram }

// Raw 业务模型本体
func (m *Model) Raw() any { return m.model }

// Parser 解析器类型
func (m *Model) Parser() Parser { return m.parser }

var modelsRank []*Model

// Register 注册模型：name 重复时报错（同一张表在一个 Store 里有两个句柄各写各的，
// 是静默的数据竞争 —— Mount 重名检查同款口径）。
func Register(name string, ram RAMType, model any, factory HandleFactory, opts ...RegisterOption) error {
	if name == "" {
		return fmt.Errorf("hamster register name empty")
	}
	if factory == nil {
		return fmt.Errorf("hamster register factory nil,model:%v", name)
	}
	for _, m := range modelsRank {
		if m.name == name {
			return fmt.Errorf("hamster model 已经存在:%v", name)
		}
	}
	mod := &Model{ram: ram, name: name, model: model, factory: factory, order: -1}
	for _, opt := range opts {
		opt(mod)
	}
	modelsRank = append(modelsRank, mod)
	sort.SliceStable(modelsRank, func(i, j int) bool {
		return modelsRank[i].order > modelsRank[j].order
	})
	return nil
}

// RegisterOption 注册选项
type RegisterOption func(*Model)

// WithParser 记录解析器类型（元数据锚点）
func WithParser(p Parser) RegisterOption {
	return func(m *Model) { m.parser = p }
}

// WithTableOrder 记录加载顺序
func WithTableOrder(order int32) RegisterOption {
	return func(m *Model) { m.order = order }
}

// RegisterDocument 按核心版窄接口注册单文档模型（主档等），内部使用默认工厂
func RegisterDocument(name string, ram RAMType, m DocumentModel, opts ...RegisterOption) error {
	if m == nil {
		return fmt.Errorf("hamster register model nil,name:%v", name)
	}
	factory := func(s *Store, mod *Model) Handle { return newDocument(s, mod) }
	opts = append(opts, WithParser(ParserTypeDocument))
	return Register(name, ram, m, factory, opts...)
}

// RegisterCollection 按核心版窄接口注册集合模型，内部使用默认工厂
func RegisterCollection(name string, ram RAMType, m CollectionModel, opts ...RegisterOption) error {
	if m == nil {
		return fmt.Errorf("hamster register model nil,name:%v", name)
	}
	factory := func(s *Store, mod *Model) Handle { return newCollection(s, mod) }
	opts = append(opts, WithParser(ParserTypeCollection))
	return Register(name, ram, m, factory, opts...)
}

// RegisterValues 按核心版窄接口注册数值 KV 模型（纯数据结构，无道具语义），内部使用默认工厂
func RegisterValues(name string, ram RAMType, m ValuesModel, opts ...RegisterOption) error {
	if m == nil {
		return fmt.Errorf("hamster register model nil,name:%v", name)
	}
	factory := func(s *Store, mod *Model) Handle { return newValues(s, mod) }
	opts = append(opts, WithParser(ParserTypeValues))
	return Register(name, ram, m, factory, opts...)
}

// RegisterVirtual 按核心版窄接口注册委托视图（纯 string 键，无 iid→字段映射），内部使用默认工厂
func RegisterVirtual(name string, ram RAMType, m VirtualModel, opts ...RegisterOption) error {
	if m == nil {
		return fmt.Errorf("hamster register model nil,name:%v", name)
	}
	factory := func(s *Store, mod *Model) Handle { return newVirtual(s, mod) }
	opts = append(opts, WithParser(ParserTypeVirtual))
	return Register(name, ram, m, factory, opts...)
}

// findModel 按名查找已注册模型
func findModel(name string) *Model {
	for _, m := range modelsRank {
		if m.name == name {
			return m
		}
	}
	return nil
}
