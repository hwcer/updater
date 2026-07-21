package updater

import (
	"fmt"
	"sort"

	"github.com/hwcer/cosgo/schema"
)

type Parser int8

const (
	ParserTypeValues     Parser = iota //Map[string]int64模式
	ParserTypeDocument                 //Document 单文档模式
	ParserTypeCollection               //Collection 文档集合模式
	ParserTypeVirtual                  //Virtual 虚拟模式,本身不会存储数据，依赖于其他模块数据，如 日常 依赖 历史数据
)

type handleFunc func(updater *Updater, model *Model) Handle

var handles = make(map[Parser]handleFunc)

func init() {
	NewHandle(ParserTypeValues, NewValues)
	NewHandle(ParserTypeDocument, NewDocument)
	NewHandle(ParserTypeCollection, NewCollection)
	NewHandle(ParserTypeVirtual, NewVirtual)
}

type TableOrder interface {
	TableOrder() int32
}

// ModelIMax 可选接口,模型未实现时回落到全局 Config.IMax
type ModelIMax interface {
	IMax(iid int32) int64 //单个道具可拥有的最大数量,默认无限
}

// ModelIType 可选接口,模型未实现时回落到全局 Config.IType
// 约束:Updater 始终按 Config.IType 把 iid 路由到 Handle,所以模型返回的 itype 必须仍归属模型自身,
// 它只能用于同一模型内多个 itype 的细分,不能与 Config.IType 给出不同的模型归属;
// 两者归属不一致时 Values 会静默丢弃操作,Collection 返回 ErrITypeNotExist
type ModelIType interface {
	IType(iid int32) int32 //内部查询道具的类型
}

// modelIMax 单个道具持有上限,模型实现 ModelIMax 时优先,否则使用全局 Config
func modelIMax(model any, iid int32) int64 {
	if v, ok := model.(ModelIMax); ok {
		return v.IMax(iid)
	}
	if Config.IMax != nil {
		return Config.IMax(iid)
	}
	return 0
}

// modelIType 查询道具类型,模型实现 ModelIType 时优先,否则使用全局 Config
// iid==0 时由模型返回默认 IType,Config 兜底通常返回 0(nil)
func modelIType(model any, iid int32) IType {
	var it int32
	if v, ok := model.(ModelIType); ok {
		it = v.IType(iid)
	} else if Config.IType != nil {
		it = Config.IType(iid)
	}
	if it == 0 {
		return nil
	}
	return itypesDict[it]
}

// ModelReset 返回true时 重新调用 model.Getter
type ModelReset interface {
	Reset(*Updater, int64) bool
}

// NewHandle 注册新解析器
func NewHandle(name Parser, f handleFunc) {
	handles[name] = f
}

var modelsRank []*Model
var modelsDict = make(map[int32]*Model)
var itypesDict = make(map[int32]IType) //ITypeId = IType

type Model struct {
	ram    RAMType
	name   string
	model  any
	parser Parser
	order  int32 //倒序排列
}

func ITypes(f func(int32, IType) bool) {
	for k, it := range itypesDict {
		if !f(k, it) {
			break
		}
	}
}
func Models(f func(int32, any) bool) {
	for k, m := range modelsDict {
		if !f(k, m) {
			break
		}
	}
}
func Register(parser Parser, ram RAMType, model any, its ...IType) error {
	if _, ok := handles[parser]; !ok {
		return fmt.Errorf("parser unknown:%v", parser)
	}

	if err := verifyModel(parser, model); err != nil {
		return err
	}

	mod := &Model{ram: ram, model: model, parser: parser}
	if t, ok := model.(schema.Tabler); ok {
		mod.name = t.TableName()
	} else {
		mod.name = schema.Kind(model).Name()
	}
	if o, ok := model.(TableOrder); ok {
		mod.order = o.TableOrder()
	} else {
		mod.order = -1
	}
	modelsRank = append(modelsRank, mod)
	sort.SliceStable(modelsRank, func(i, j int) bool {
		return modelsRank[i].order > modelsRank[j].order
	})

	for _, it := range its {
		if err := verifyIType(parser, mod.name, it); err != nil {
			return err
		}
		id := it.ID()
		if _, ok := modelsDict[id]; ok {
			return fmt.Errorf("model IType(%v)已经存在:%v", it, mod.name)
		}
		modelsDict[id] = mod
		itypesDict[id] = it
	}
	return nil
}

func verifyModel(parser Parser, model any) error {
	switch parser {
	case ParserTypeValues:
		if _, ok := model.(ValuesModel); !ok {
			return fmt.Errorf("model %T does not implement ValuesModel", model)
		}
	case ParserTypeDocument:
		if _, ok := model.(DocumentModel); !ok {
			return fmt.Errorf("model %T does not implement DocumentModel", model)
		}
	case ParserTypeCollection:
		if _, ok := model.(CollectionModel); !ok {
			return fmt.Errorf("model %T does not implement CollectionModel", model)
		}
	case ParserTypeVirtual:
		if _, ok := model.(VirtualModel); !ok {
			return fmt.Errorf("model %T does not implement VirtualModel", model)
		}
	}
	return nil
}

func verifyIType(parser Parser, name string, it IType) error {
	switch parser {
	case ParserTypeCollection:
		if _, ok := it.(ITypeCollection); !ok {
			return fmt.Errorf("IType(%d) does not implement ITypeCollection for model %s", it.ID(), name)
		}
	default:
		return nil
	}
	return nil
}
