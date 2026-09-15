package updater

import (
	"fmt"
	"sync"
	"time"

	"github.com/hwcer/cosgo/schema"
	"github.com/hwcer/updater/hamster"
)

type handleFunc func(updater *Updater, model *Model) Handle

// handles 句柄工厂表：Parser → 构造函数。hamster 注册时经闭包桥接（见 Register）。
var handles = make(map[Parser]handleFunc)

// NewHandle 注册新解析器的句柄工厂
func NewHandle(name Parser, f handleFunc) {
	handles[name] = f
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
//
// 🔴 实现方必须加一行编译期断言:
//
//	var _ updater.ModelReset = (*YourModel)(nil)
//
// 本接口靠类型断言 model.(ModelReset) 识别,签名写错【不会编译报错】,
// 只会让断言不再命中、Reset 从此永不被调用,功能悄无声息地失效。
// 接口一旦改签名,没有断言的实现方就是这样中招的。
type ModelReset interface {
	Reset(*Updater, time.Time) bool
}

// modelsDict/itypesDict 道具路由表：iid 的 IType → 模型/IType。
// 🔴 它们是**扩展层私产**，核心版（hamster）的注册表只认 name ——
// 两层注册表的唯一接缝是 Register 里的 HandleFactory（HAMSTER_PLAN.md 第六节①）。
var modelsDict = make(map[int32]*Model)
var itypesDict = make(map[int32]IType) //ITypeId = IType

// Model 已注册道具模型的元数据（扩展层）。
// hamster 侧注册在 Register 内部桥接完成，本结构只服务 iid 路由。
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

// Register 注册道具模型。
//
// 内部做两件事：①经 HandleFactory 桥接进 hamster 注册表（hamster 只认 name，
// 加载顺序/重名检查/生命周期驱动都归它）；②把 IType 归属记进扩展层路由表。
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

	// 两层注册表的唯一合法接缝：工厂闭包捕获扩展层构造函数与模型元数据，
	// hamster.Loading 创建句柄时经 updaterOf 找回 Updater 实例。
	factory := func(s *hamster.Store, _ *hamster.Model) hamster.Handle {
		return handles[parser](updaterOf(s), mod)
	}
	if err := hamster.Register(mod.name, ram, model, factory,
		hamster.WithParser(parser), hamster.WithTableOrder(mod.order)); err != nil {
		return err
	}

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

// updaterOf 由 hamster.Store 反查所属 Updater（扩展层句柄的 model 回调需要 *Updater）。
// 注册在 New，注销在 Destroy。
var updaters sync.Map // *hamster.Store → *Updater

func updaterOf(s *hamster.Store) *Updater {
	if s == nil {
		return nil
	}
	if v, ok := updaters.Load(s); ok {
		return v.(*Updater)
	}
	return nil
}
