package updater

import (
	"fmt"
	"sort"
	"sync"

	"github.com/hwcer/cosgo/schema"
)

// ManageConfig 数据域配置：路由（IType/ParseId）、上限（IMax）与落库（BulkWrite）。
// 原为包级全局 Config，实例化后每个 Manage 持有一份；包级 var Config 是 Default 域
// 配置的别名（*ManageConfig），updater.Config.IType = f 照旧编译生效。
type ManageConfig struct {
	IMax      func(iid int32) int64                                     //通过道具iid查找上限
	IType     func(iid int32) int32                                     //通过道具iid查找IType ID
	ParseId   func(adapter *Updater, oid string) (iid int32, err error) //解析OID获得IID
	BulkWrite func(u *Updater) BulkWrite                                //域内 BulkWrite 工厂
}

// entityBox 实体实例箱：实例 + 实体锁。
// 同一实体的请求必须串行（Reset→业务→Submit→Release 在锁内完成），跨实体并行不受影响
// —— 与 yyds/players 外置锁的同款纪律，沉淀至此由 Manage 统一提供。
type entityBox struct {
	mu sync.Mutex
	u  *Updater
}

// Manage 数据域：一套注册表 + Config + 域级事件/缓存 + 实体实例管理器。
//
// 原包级全局 modelsRank/modelsDict/itypesDict/Config/globalCache/globalEvents/
// globalMiddlewares 全部收进本类型：一个进程可以并存多个域（玩家域、公会域…），
// 各域独立注册模型与 IType 路由空间，互不串扰 —— 一个公会就相当于一个玩家，
// 公会域可以有自己的每日数据、IType 编号甚至落库目标（BulkWrite 可指向不同的库）。
//
// 🔴 IType ID 仅域内有意义：operator 上只有裸 ID，跨域流转（或发往客户端）时
// 接收方必须先定位域、再按 IType 分发；两个域用同一 ID 指向不同模型是合法且常见的。
//
// 兼容：包级 Register/Config/RegisterGlobalXxx/ITypes/Models/New 一行委托 Default 域，
// 旧代码零改动；多域场景用 NewManage 另建，别往 Default 里塞非玩家模型。
type Manage struct {
	Config *ManageConfig //域配置：路由/上限/落库

	parser        map[Parser]handleFunc          //句柄工厂表：构造时拷贝内置四项，NewHandle 可覆盖
	modelsRank    []*Model                       //已注册模型，TableOrder 降序，驱动 Loading/Handles 遍历顺序
	modelsDict    map[int32]*Model               //IType ID → Model，iid 路由反查表
	itypesDict    map[int32]IType                //IType ID → IType（钩子载体）
	cacheCreators map[string]CacheCreator        //域级缓存构造器：Loading 时为该域每个实例创建
	events        map[EventType][]func(*Updater) //域级事件：对该域所有实例生效，永不取消（原 globalEvents）
	middlewares   []Middleware                   //域级中间件：同上（原 globalMiddlewares）

	entities sync.Map //实体实例表：uid → *entityBox（Load/Unload 管理）
}

// Default 默认数据域：包级 API 的委托目标。
// 单域（只管玩家）用法零改动；新域请 NewManage。
var Default = NewManage()

// Config 兼容锚点：原包级配置，现为 Default 域配置（*ManageConfig）的别名。
var Config = Default.Config

func NewManage() *Manage {
	m := &Manage{
		Config:     &ManageConfig{},
		parser:     make(map[Parser]handleFunc, len(builtinHandles)),
		modelsDict: make(map[int32]*Model),
		itypesDict: make(map[int32]IType),
		events:     map[EventType][]func(*Updater){},
	}
	for k, v := range builtinHandles {
		m.parser[k] = v
	}
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
	if m.cacheCreators == nil {
		m.cacheCreators = make(map[string]CacheCreator)
	}
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

// Models 遍历本域的模型表
func (m *Manage) Models(f func(int32, any) bool) {
	for k, v := range m.modelsDict {
		if !f(k, v) {
			break
		}
	}
}

// New 创建本域的裸实例（不进实体表）。生命周期（Loading/Reset/Submit/Release/Destroy）
// 由调用方自管 —— 与包级 updater.New 旧用法等价；要实体表 + 实体锁 + 自动加载用 Load。
func (m *Manage) New(p Player) *Updater {
	return &Updater{manage: m, player: p, Cache: Cache{}, Events: Events{}, Middleware: Middlewares{}}
}

// ===================== 实体实例管理（自 yyds/players 沉淀） =====================

// Get 按实体ID取已加载的实例：只取不建、不加锁，未加载返回 nil。
// 需要请求级串行保证时用 Load。
func (m *Manage) Get(uid string) *Updater {
	if v, ok := m.entities.Load(uid); ok {
		return v.(*entityBox).u
	}
	return nil
}

// Load 取或建 + 实体锁：实例不存在时创建并 Loading（幂等；失败不留半实例），
// 然后持有实体锁返回，unlock 释放。同 uid 的并发 Load 在此串行，跨 uid 并行。
//
// 标准请求周期全部在锁内完成：
//
//	u, unlock, err := manage.Load(player)
//	if err != nil { return err }
//	defer unlock()
//	u.Reset()
//	... 业务 ...
//	_, err = u.Submit()
//	u.Release()
//
// ⚠️ unlock 必须且只能调用一次（sync.Mutex 不可重入）；长命持有（跨多次请求的战斗副本
// 之类）就多拿一会儿，每次请求各自 Reset/Release，锁跟着业务周期走。
// ⚠️ Loading 失败时实例不入表，但空箱保留 —— 下一次 Load 在同一把锁上重试创建。
func (m *Manage) Load(p Player) (u *Updater, unlock func(), err error) {
	uid := p.Uid()
	v, _ := m.entities.LoadOrStore(uid, &entityBox{})
	box := v.(*entityBox)
	box.mu.Lock()
	if box.u != nil {
		return box.u, box.mu.Unlock, nil
	}
	u = m.New(p)
	if err = u.Loading(); err != nil {
		box.mu.Unlock()
		return nil, nil, err
	}
	box.u = u
	return u, box.mu.Unlock, nil
}

// Unload 实体下线：加锁 → Destroy 刷盘 → 摘除。
// Destroy 失败时保留实例（数据还在内存里），排除问题后重试 Unload；
// 未加载的 uid 返回 nil。
func (m *Manage) Unload(uid string) error {
	v, ok := m.entities.Load(uid)
	if !ok {
		return nil
	}
	box := v.(*entityBox)
	box.mu.Lock()
	defer box.mu.Unlock()
	if u := box.u; u != nil {
		if err := u.Destroy(); err != nil {
			return err
		}
		box.u = nil
	}
	m.entities.Delete(uid)
	return nil
}

// Range 遍历域内全部实例（停服全量刷盘、运营补丁等场景），f 返回 false 停止。
// ⚠️ 不加实体锁：读 box.u 不经互斥，调用方自行保证此时无并发请求（停服场景天然满足）。
func (m *Manage) Range(f func(uid string, u *Updater) bool) {
	m.entities.Range(func(k, v any) bool {
		box := v.(*entityBox)
		if box.u == nil {
			return true
		}
		return f(k.(string), box.u)
	})
}

// ===================== 兼容层（一行委托 Default 域，旧代码零改动） =====================

// New 创建默认域的 Updater 实例
func New(p Player) *Updater { return Default.New(p) }

// Register 注册模型到默认域
func Register(parser Parser, ram RAMType, model any, its ...IType) error {
	return Default.Register(parser, ram, model, its...)
}

// NewHandle 覆盖默认域的句柄工厂
func NewHandle(name Parser, f handleFunc) { Default.NewHandle(name, f) }

// RegisterGlobalCache 注册默认域的缓存构造器
func RegisterGlobalCache(name string, creator CacheCreator) {
	Default.RegisterGlobalCache(name, creator)
}

// RegisterGlobalEvent 注册默认域的事件
func RegisterGlobalEvent(t EventType, handle func(u *Updater)) {
	Default.RegisterGlobalEvent(t, handle)
}

// RegisterGlobalMiddleware 注册默认域的中间件
func RegisterGlobalMiddleware(handle Middleware) {
	Default.RegisterGlobalMiddleware(handle)
}

// ITypes 遍历默认域的 IType 表
func ITypes(f func(int32, IType) bool) { Default.ITypes(f) }

// Models 遍历默认域的模型表
func Models(f func(int32, any) bool) { Default.Models(f) }
