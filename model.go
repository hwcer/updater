package updater

import (
	"fmt"
	"sync"

	"github.com/hwcer/cosgo/schema"
	"github.com/hwcer/updater/dataset"
	"github.com/hwcer/updater/hamster"
	"github.com/hwcer/updater/operator"
)

// ---------------- 扩展层模型接口（业务实现，签名含 *Updater） ----------------
//
// 它们经 adapters.go 桥接成 hamster 模型；Getter/Setter 在调用时经 updaterOf
// 反查 *Updater，业务模型零改动。

// ValuesModel 数字型键值对模型接口
// 可选实现 ModelIMax / ModelIType 覆盖全局 Config 的上限与类型查询
type ValuesModel interface {
	Getter(u *Updater, data *dataset.Values, keys []int32) error
	Setter(u *Updater, bulkWrite BulkWrite, dirty dataset.Data, unset []int32) error
}

// DocumentModel 文档模型接口
// 建议在业务model中实现 dataset.ModelGet 和 dataset.ModelSet 接口提高性能
// 可选实现 ModelIMax 覆盖全局 Config 的上限查询
// IType 为必需方法(等价 ModelIType):Document 大部分操作按 field 定位,iid 为 0,
// 只能由模型给出默认 IType,全局 Config.IType(0) 无法兜底;Values/Virtual 无此需求,IType 是可选的
type DocumentModel interface {
	New(update *Updater) any
	IType(int32) int32
	Field(update *Updater, iid int32) (string, error)
	Getter(update *Updater, data *dataset.Document, keys []string) error
	Setter(update *Updater, bulkWrite BulkWrite, dirty dataset.Update, unset []string) error
}

type CollectionModel interface {
	Upsert(update *Updater, op *operator.Operator) bool
	Schema() *schema.Schema
	Getter(update *Updater, data *dataset.Collection, keys []string) error
	Setter(update *Updater, bulkWrite BulkWrite, _id string, dirty dataset.Update, unset []string) error
}

type CollectionModelValueJSName interface {
	GetValueJSName() string //获取value值的jsname
}

// VirtualModel 虚拟模型接口
// 可选实现 ModelIMax / ModelIType 覆盖全局 Config 的上限与类型查询
type VirtualModel interface {
	Has(u *Updater, k any) bool
	Get(u *Updater, k any) (r any)
	Field(int32) (string, bool) //格式化字段
	Update(u *Updater, op *operator.Operator)
	Select(u *Updater, keys ...any)
	Reload(u *Updater) error
}

// ---------------- 道具路由表（扩展层私产） ----------------

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

// NewHandle 注册新解析器的句柄工厂。
//
// ⚠️ 句柄并入核心后，工厂不再参与句柄创建（hamster 按 Parser 用固定适配器构造）；
// 保留本函数只为兼容既有注册代码，自定义句柄类型已不支持。
func NewHandle(name Parser, f handleFunc) {
	handles[name] = f
}

type handleFunc func(updater *Updater, model *Model) Handle

var handles = make(map[Parser]handleFunc)

func init() {
	//仅作 Parser 合法性标记（句柄创建已由 hamster 按适配器固定构造，见 Register）
	NewHandle(ParserTypeValues, nil)
	NewHandle(ParserTypeDocument, nil)
	NewHandle(ParserTypeCollection, nil)
	NewHandle(ParserTypeVirtual, nil)
}

// modelsDict/itypesDict 道具路由表：iid 的 IType → 模型/IType。
// 🔴 它们是**扩展层私产**，核心版（hamster）的注册表只认 name ——
// 两层注册表的唯一接缝是 Register 里的适配器桥接（HAMSTER_PLAN.md 第六节①）。
var modelsDict = make(map[int32]*Model)
var itypesDict = make(map[int32]IType) //ITypeId = IType

// Model 已注册道具模型的元数据（扩展层）。
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
// 内部做两件事：①按 Parser 选择适配器桥接进 hamster 注册表（核心版只认 name，
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

	// 两层注册表的唯一合法接缝：适配器实现 hamster 模型接口与全部可选注入接口，
	// hamster.Loading 创建句柄时经 updaterOf 找回 Updater 实例。
	var opts []hamster.RegisterOption
	opts = append(opts, hamster.WithParser(parser), hamster.WithTableOrder(mod.order))
	var err error
	switch parser {
	case ParserTypeValues:
		err = hamster.RegisterValues(mod.name, ram, &valuesAdapter{m: model.(ValuesModel)}, opts...)
	case ParserTypeDocument:
		err = hamster.RegisterDocument(mod.name, ram, &docAdapter{m: model.(DocumentModel)}, opts...)
	case ParserTypeCollection:
		err = hamster.RegisterCollection(mod.name, ram, &collAdapter{m: model.(CollectionModel)}, opts...)
	case ParserTypeVirtual:
		err = hamster.RegisterVirtual(mod.name, ram, &virtualAdapter{m: model.(VirtualModel)}, opts...)
	default:
		err = fmt.Errorf("parser unknown:%v", parser)
	}
	if err != nil {
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

// updaterOf 由 hamster.Store 反查所属 Updater（适配器的模型回调需要 *Updater）。
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
