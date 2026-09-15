package hamster

import (
	"fmt"
	"sort"
)

// TableOrder 可选接口：控制模型在 Loading 阶段的加载顺序，值大的先加载。
//
// 主档模式的支柱：主档模型返回最大值（或 0），其余模型返回负值，
// 保证主档先于依赖它的集合加载完成（HAMSTER_PLAN.md 第四节 D6）。
type TableOrder interface {
	TableOrder() int32
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

// findModel 按名查找已注册模型
func findModel(name string) *Model {
	for _, m := range modelsRank {
		if m.name == name {
			return m
		}
	}
	return nil
}
