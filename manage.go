package updater

import (
	"fmt"
	"maps"
	"sort"

	"github.com/hwcer/cosgo/schema"
)

// defaultNewMax 不可叠加道具单次批量创建的默认上限。
// 正常业务一次至多创建几十件装备,999 已远超合理峰值;再大基本都是程序错误或恶意请求,
// 逐个 it.New 会在无上限时原地挂起直至 OOM。
const defaultNewMax int64 = 999

// Options 数据域配置：路由（IType/ParseId）、上限（IMax/NewMax）与落库（BulkWrite）。
// 原为包级全局 Config，实例化后每个 Manage 持有一份；包级 var Config 是 Default 域
// 配置的别名（*Options），updater.Config.IType = f 照旧编译生效。
type Options struct {
	IMax      func(iid int32) int64                                     //通过道具iid查找上限
	IType     func(iid int32) int32                                     //通过道具iid查找IType ID
	ParseId   func(adapter *Updater, oid string) (iid int32, err error) //解析OID获得IID; New() 安装默认实现(defaultParseId),未配置时字符串键报错而不是 panic
	NewMax    int64                                                     //单次操作批量创建不可叠加道具(TypesNew)的数量上限;可叠加道具不受限;<=0 视为 defaultNewMax
	BulkWrite func(u *Updater) BulkWrite                                //域内 BulkWrite 工厂;实现需支持失败后重复 Submit 直到成功(cosmo.BulkWrite 即此语义:失败保留 models,成功才清空)
}

// Manage 数据域：一套注册表 + Config + 域级事件/缓存。
//
// 原包级全局 modelsRank/modelsDict/itypesDict/Config/globalCache/globalEvents/
// globalMiddlewares 全部收进本类型：一个进程可以并存多个域（玩家域、公会域…），
// 各域独立注册模型与 IType 路由空间，互不串扰 —— 一个公会就相当于一个玩家，
// 公会域可以有自己的每日数据、IType 编号甚至落库目标（BulkWrite 可指向不同的库）。
//
// 🔴 IType ID 仅域内有意义：operator 上只有裸 ID，跨域流转（或发往客户端）时
// 接收方必须先定位域、再按 IType 分发；两个域用同一 ID 指向不同模型是合法且常见的。
//
// ⚠️ 本类型只管"域"不管"实例"：实例（Updater）的创建与生命周期（含玩家锁、
// 实例表）归业务层（yyds/players）所有，经 New(e) 取绑定到本域的裸实例即可。
//
// 🔴 注册面（Register/NewHandle/RegisterGlobalXxx）只在启动期调用：无锁保护，
// 必须先注册完再创建实例，运行期追加注册是数据竞争。
//
// 兼容：包级 Register/Config/RegisterGlobalXxx/ITypes/Models 一行委托 Default 域；
// Default 本身是包级 New() 造出的域。
type Manage struct {
	Config *Options //域配置：路由/上限/落库

	parser        map[Parser]handleFunc          //句柄工厂表：构造时拷贝内置四项，NewHandle 可覆盖
	modelsRank    []*Model                       //已注册模型，TableOrder 降序，驱动 Loading/Handles 遍历顺序
	modelsDict    map[int32]*Model               //IType ID → Model，iid 路由反查表
	itypesDict    map[int32]IType                //IType ID → IType（钩子载体）
	cacheCreators map[string]CacheCreator        //域级缓存构造器：Loading 时为该域每个实例创建
	events        map[EventType][]func(*Updater) //域级事件：对该域所有实例生效，永不取消（原 globalEvents）
	middlewares   []Middleware                   //域级中间件：同上（原 globalMiddlewares）
}

// defaultParseId ParseId 未配置时的默认实现:显式报错而不是运行期 nil 调用 panic。
// 字符串 OID 的两个消费点都经它兜底 —— Updater.ParseId(handleWithKey 路由,告警后忽略操作)
// 与 Collection.operator(把 Updater.Error 置脏,请求失败)。
func defaultParseId(_ *Updater, oid string) (int32, error) {
	return 0, ErrParseIdNotInitialize
}

// defaultIType IType 未配置时的默认实现:返回 0(未知类型)。
// handleWithKey 路由不到模型,操作被忽略 —— 与 IType 查不到模型的既有路径同口径,
// 而不是运行期 nil 调用 panic。业务必须通过 Options.IType 显式配置路由。
func defaultIType(int32) int32 { return 0 }

// New 创建一个数据域。包级 var Default 就是它造出来的默认域；
// 多域场景（公会等）另建，别往 Default 里塞非玩家模型。
func New() *Manage {
	m := &Manage{
		Config:        &Options{ParseId: defaultParseId, IType: defaultIType, NewMax: defaultNewMax},
		modelsDict:    make(map[int32]*Model),
		itypesDict:    make(map[int32]IType),
		cacheCreators: make(map[string]CacheCreator),
		events:        map[EventType][]func(*Updater){},
	}
	m.parser = make(map[Parser]handleFunc, len(builtinHandles))
	maps.Copy(m.parser, builtinHandles)
	return m
}

// ===================== 注册面（原包级函数平移为方法） =====================

// Register 注册模型到本域。同一 IType ID 在域内唯一，跨域可重复（各自指向自己的模型）。
func (m *Manage) Register(parser Parser, ram RAMType, model any, its ...IType) error {
	if _, ok := m.parser[parser]; !ok {
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
	m.modelsRank = append(m.modelsRank, mod)
	sort.SliceStable(m.modelsRank, func(i, j int) bool {
		return m.modelsRank[i].order > m.modelsRank[j].order
	})
	for _, it := range its {
		if err := verifyIType(parser, mod.name, it); err != nil {
			return err
		}
		id := it.ID()
		if _, ok := m.modelsDict[id]; ok {
			return fmt.Errorf("model IType(%v)已经存在:%v", it, mod.name)
		}
		m.modelsDict[id] = mod
		m.itypesDict[id] = it
	}
	return nil
}

// NewHandle 覆盖/新增本域的句柄工厂（构造时已拷贝内置四项，此处改的是域自己那份）
func (m *Manage) NewHandle(name Parser, f handleFunc) {
	m.parser[name] = f
}

// RegisterGlobalCache 注册域级缓存：Loading 时为该域每个实例创建（原"全局缓存"语义收窄到域）
func (m *Manage) RegisterGlobalCache(name string, creator CacheCreator) {
	m.cacheCreators[name] = creator
}

// RegisterGlobalEvent 注册域级事件：对该域所有实例生效，永不取消（原"全局事件"语义收窄到域）
func (m *Manage) RegisterGlobalEvent(t EventType, handle func(u *Updater)) {
	m.events[t] = append(m.events[t], handle)
}

// RegisterGlobalMiddleware 注册域级中间件：对该域所有实例生效，永不移除（原"全局中间件"语义收窄到域）
func (m *Manage) RegisterGlobalMiddleware(handle Middleware) {
	m.middlewares = append(m.middlewares, handle)
}

// ITypes 遍历本域的 IType 表
func (m *Manage) ITypes(f func(int32, IType) bool) {
	for k, it := range m.itypesDict {
		if !f(k, it) {
			break
		}
	}
}

// IMax 单个道具持有上限:模型实现 ModelIMax 接口时优先,否则回落本域 Options.IMax(未配置返回 0)。
// model 传 nil 即纯域配置口径。只依赖本域注册表,与 Updater 实例状态无关,故挂在 Manage 上。
func (m *Manage) IMax(model any, iid int32) int64 {
	if v, ok := model.(ModelIMax); ok {
		return v.IMax(iid)
	}
	if m != nil && m.Config != nil && m.Config.IMax != nil {
		return m.Config.IMax(iid)
	}
	return 0
}

// IType 查询道具类型:模型实现 ModelIType 接口时优先,否则回落本域 Options.IType;
// iid==0 时由模型返回默认 IType,域配置兜底通常返回 nil。
// ⚠️ 路由口径与模型口径是两回事:Updater 始终按 Options.IType 把 iid 路由到 Handle,
// 模型返回的 itype 必须仍归属模型自身,只能用于同一模型内多个 itype 的细分。
// model 传 nil 即纯域配置路由口径。
func (m *Manage) IType(model any, iid int32) (it IType) {
	var id int32
	if v, ok := model.(ModelIType); ok {
		id = v.IType(iid)
	} else if m != nil && m.Config != nil && m.Config.IType != nil {
		id = m.Config.IType(iid)
	}
	if id == 0 || m == nil {
		return nil
	}
	return m.itypesDict[id]
}

// Models 遍历本域的模型表
func (m *Manage) Models(f func(int32, any) bool) {
	for k, v := range m.modelsDict {
		if !f(k, v) {
			break
		}
	}
}

// New 创建绑定到本域的裸实例。实例的生命周期（Loading/Reset/Submit/Release/Destroy，
// 含玩家锁与实例表）由业务层自管 —— 单域场景用 Default.New(e)。
func (m *Manage) New(e Entity) *Updater {
	u := &Updater{manage: m, entity: e, Cache: Cache{}, Events: Events{}, Middleware: Middlewares{}}
	u.Mounts.updater = u //挂载表回引用，见 Mounts.Mount
	return u
}
