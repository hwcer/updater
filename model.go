package updater

import (
	"fmt"
	"reflect"
	"sort"
	"time"

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
//
// 第二个参数是【上次请求的时间】,用于判断跨天/跨周等需要重置的场景。
// 类型为 time.Time 而非 unix 秒:时间比较应带完整精度与时区信息,
// 秒级时间戳在跨天判定这类场景要额外拼 time.Unix 才能用。
// 零值(IsZero)表示本 Updater 实例尚未处理过任何请求。
type ModelReset interface {
	Reset(*Updater, time.Time) bool
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
	if err := verifyOptional(model); err != nil {
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

// optionalInterfaces 可选接口清单:方法名 -> 接口类型
//
// 这类接口靠类型断言 model.(XXX) 识别,不在编译期强制。代价是【签名写错会静默失败】:
// 编译照样通过,但断言不再命中,该接口从此永不被调用,功能悄无声息地失效——
// 接口一旦改签名(如 ModelReset 的 int64 改 time.Time),所有未跟进的实现方都会中招。
// 故在注册期主动检出:有同名方法却不满足接口,一律判为签名写错。
var optionalInterfaces = []struct {
	method string
	typ    reflect.Type
}{
	{"Reset", reflect.TypeOf((*ModelReset)(nil)).Elem()},
	{"IMax", reflect.TypeOf((*ModelIMax)(nil)).Elem()},
	{"TableOrder", reflect.TypeOf((*TableOrder)(nil)).Elem()},
}

// verifyOptional 检出"有同名方法但签名与可选接口不符"
//
// 完全没有该方法 = 明确不实现,属正常情况,放行。
func verifyOptional(model any) error {
	rt := reflect.TypeOf(model)
	if rt == nil {
		return nil
	}
	for _, oi := range optionalInterfaces {
		if _, has := rt.MethodByName(oi.method); !has {
			continue
		}
		if !rt.Implements(oi.typ) {
			m, _ := rt.MethodByName(oi.method)
			return fmt.Errorf("model %v 的 %v 方法签名与 %v 不符,该接口将【静默失效】;当前签名 %v,应为 %v",
				rt, oi.method, oi.typ, m.Type, oi.typ.Method(0).Type)
		}
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
