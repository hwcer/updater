package updater

import (
	"fmt"
	"time"
)

type Parser int8

const (
	ParserTypeValues     Parser = iota //Map[string]int64模式
	ParserTypeDocument                 //Document 单文档模式
	ParserTypeCollection               //Collection 文档集合模式
	ParserTypeVirtual                  //Virtual 虚拟模式,本身不会存储数据，依赖于其他模块数据，如 日常 依赖 历史数据
)

type handleFunc func(updater *Updater, model *Model) Handle

// builtinHandles 内置句柄工厂表：每个 Manage 构造时拷贝一份，
// 经 Manage.NewHandle 按域覆盖，互不污染（原包级 handles + init() 注册）。
var builtinHandles = map[Parser]handleFunc{
	ParserTypeValues:     NewValues,
	ParserTypeDocument:   NewDocument,
	ParserTypeCollection: NewCollection,
	ParserTypeVirtual:    NewVirtual,
}

type TableOrder interface {
	TableOrder() int32
}

// ModelIMax 可选接口,模型未实现时回落到所属域 ManageConfig.IMax
type ModelIMax interface {
	IMax(iid int32) int64 //单个道具可拥有的最大数量,默认无限
}

// ModelIType 可选接口,模型未实现时回落到所属域 ManageConfig.IType
// 约束:Updater 始终按 ManageConfig.IType 把 iid 路由到 Handle,所以模型返回的 itype 必须仍归属模型自身,
// 它只能用于同一模型内多个 itype 的细分,不能与 ManageConfig.IType 给出不同的模型归属;
// 两者归属不一致时 Values 会静默丢弃操作,Collection 返回 ErrITypeNotExist
type ModelIType interface {
	IType(iid int32) int32 //内部查询道具的类型
}

// modelIMax 单个道具持有上限,模型实现 ModelIMax 时优先,否则使用所属域 Config
func modelIMax(u *Updater, model any, iid int32) int64 {
	if v, ok := model.(ModelIMax); ok {
		return v.IMax(iid)
	}
	if u != nil && u.manage != nil && u.manage.Config.IMax != nil {
		return u.manage.Config.IMax(iid)
	}
	return 0
}

// modelIType 查询道具类型,模型实现 ModelIType 时优先,否则使用所属域 Config
// iid==0 时由模型返回默认 IType,Config 兜底通常返回 0(nil)
func modelIType(u *Updater, model any, iid int32) IType {
	var it int32
	if v, ok := model.(ModelIType); ok {
		it = v.IType(iid)
	} else if u != nil && u.manage != nil && u.manage.Config.IType != nil {
		it = u.manage.Config.IType(iid)
	}
	if it == 0 || u == nil || u.manage == nil {
		return nil
	}
	return u.manage.itypesDict[it]
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
//
// 框架无法替你自动检出:方法名与参数形态都不足以区分"想实现但写错"和"恰好同名的
// 无关方法"——protobuf 生成的每个 message 都带无参 Reset(),而业务侧也可能有
// Reset(*Updater, *dataset.Document) error 这类首参同样是 *Updater 的自有方法。
// 试过按这些特征猜,两种情况都会误报并炸掉启动,故只能由实现方显式声明。
type ModelReset interface {
	Reset(*Updater, time.Time) bool
}

// Model 已注册模型的元数据（属于某一个 Manage 域）
type Model struct {
	ram    RAMType
	name   string
	model  any
	parser Parser
	order  int32 //倒序排列
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
