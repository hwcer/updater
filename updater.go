package updater

import (
	"fmt"
	"reflect"
	"time"

	"github.com/hwcer/logger"
	"github.com/hwcer/updater/hamster"
	"github.com/hwcer/updater/dataset"
	"github.com/hwcer/updater/operator"
)

// Updater 玩家数据更新器（核心版 hamster 之上的道具扩展层）。
//
// IType 路由、Add/Sub 便捷 API、溢出分解都长在这层；生命周期与批量落库委托给
// 持有的 hamster.Store。
//
// 🔴 **持有，不内嵌**：Go 的方法提升没有虚派发（Mount 三稿教训，CLAUDE.md 明文），
// 内嵌会让 hamster.Store 的方法透传成隐式 API，任何"想让 Submit 多做一点"的改动
// 都会变成同名覆盖雷区。生命周期委托方法一次写清，不得在委托里偷改时序。
//
// 错误状态：store.Error 是唯一权威字段（生命周期闸门都读它），Error 字段是
// API 冻结的公开镜像（用户直接读写），经同步纪律保持一致（见 syncErrIn/syncErrOut）。
// 变更流水：dirty 字段是唯一权威，句柄 statement 的默认接收器直接推进本字段。
type Updater struct {
	store *hamster.Store      //核心版存储引擎（生命周期/落库/mounts）
	dirty []*operator.Operator //本次请求产生的操作列表，用于同步给客户端

	Cache         Cache       //自定义缓存数据（与 store.Cache 同一块 map）
	Error         error       //请求过程中的错误（唯一权威，经钩子外接给 store）
	Events        Events      //生命周期事件
	Middleware    Middlewares //中间件，所有事件类型都会触发
	CreditAllowed bool        //本次请求是否允许扣量为负（一次性标记）
}

// Entity 数据属主（取代 Player/Uid 用词 —— 核心版不绑玩家域）。
// 🔴 这是 API 冻结的**唯一豁免**：用户迁移动作 = 把 Player 实现的 Uid() string 改名 Id() string。
type Entity = hamster.Entity

// New 创建更新器。
func New(e Entity) *Updater {
	st := hamster.New(e)
	u := &Updater{store: st}
	u.Cache = Cache(st.Cache) //共享同一块底层 map（同底层类型的 map 转换是重挂类型、不拷贝）
	st.SetEmitHook(func(_ *hamster.Store, t hamster.EventType) {
		u.syncErrOut() //事件分发前先镜像：监听器读 u.Error 才是新鲜的
		u.emitRoot(EventType(t))
	})
	updaters.Store(st, u)
	return u
}

// 🔴 错误状态同步纪律（store.Error 是唯一权威字段，u.Error 是冻结的公开镜像字段）：
//   1. 委托进 store 生命周期的公开方法：入口 syncErrIn、出口 syncErrOut；
//   2. 扩展句柄写错误一律走 setError（双写两个字段）；
//   3. Mount 包装方法委托前后同样 syncErrIn / syncErrOut；
//   4. 事件桥分发前先 syncErrOut。
// 漏一处 = 错误闸门读到陈旧值 = 静默失效；新增公开方法必须照此办理。
func (u *Updater) syncErrIn() {
	if u.store != nil {
		u.store.Error = u.Error
	}
}
func (u *Updater) syncErrOut() {
	if u.store != nil {
		u.Error = u.store.Error
	}
}

// setError 扩展句柄专用：双写错误状态（u.Error 供用户/句柄读，store.Error 供闸门读）
func (u *Updater) setError(err error) {
	u.Error = err
	if u.store != nil {
		u.store.Error = err
	}
}

// emitRoot 根包事件分发（拆分前 Emit 的原语义）
func (u *Updater) emitRoot(t EventType) {
	u.Events.emit(u, t)
	u.Middleware.emit(u, t)
}

func (u *Updater) On(t EventType, handle Listener) {
	u.Events.On(t, handle)
}

// BulkWrite 共享 BulkWrite 实例（经 hamster 桥回 updater.Config 工厂，见 define.go init）
func (u *Updater) BulkWrite() BulkWrite {
	return u.store.BulkWrite()
}

// Id 数据属主标识（取代 Uid，见 Entity）
func (u *Updater) Id() string {
	return u.store.Id()
}

func (u *Updater) Now() time.Time {
	return u.store.Now()
}

func (u *Updater) Unix() int64 {
	return u.store.Unix()
}
func (u *Updater) Milli() int64 {
	return u.store.Milli()
}

// Entity 返回数据属主
func (u *Updater) Entity() Entity {
	return u.store.Entity()
}

// Last 上次请求时间（零值表示尚未处理过请求），跨天重置判定用
func (u *Updater) Last() time.Time {
	return u.store.Last()
}

func (u *Updater) Errorf(format any, args ...any) error {
	switch v := format.(type) {
	case string:
		u.setError(fmt.Errorf(v, args...))
	case error:
		u.setError(v)
	default:
		u.setError(fmt.Errorf("%v", v))
	}
	return u.Error
}

// Save 保存所有缓存数并自动关闭异步模式
func (u *Updater) Save() (err error) {
	u.syncErrIn()
	err = u.store.Save()
	u.syncErrOut()
	return
}

func (u *Updater) Loader() bool {
	return u.store.Loader()
}

// Develop 设置或获取开发者模式标记，仅供业务层自取
func (u *Updater) Develop(v ...bool) bool {
	return u.store.Develop(v...)
}

// Testing 测试模式开关，开启后所有操作仅在内存生效不写库，关闭时强制从数据库重新加载
func (u *Updater) Testing(on bool) error {
	u.syncErrIn()
	err := u.store.Testing(on)
	u.syncErrOut()
	return err
}

// Reload 丢弃所有已加载的内存数据，下次访问时重新从数据库读取。
//
// 用于「带外改动了数据库、要求内存跟上」的场景：运营后台改档、GM 工具导入存档、
// 线上数据修复等——这些改动绕过了 updater，内存里仍是旧值，玩家下一次操作会在旧值上
// 算增量并把脏数据写回去。
//
// 调用方必须持有玩家锁（与其它 Updater 方法一致）。只重置已加载的数据集，不动 status。
//
// ⚠ 对**在线**玩家只重载服务端内存，客户端手上那份仍是旧的，通常还需要让客户端重新拉取。
func (u *Updater) Reload() error {
	u.syncErrIn()
	err := u.store.Reload()
	u.syncErrOut()
	return err
}

// loadGlobalCache 把 RegisterGlobalCache 注册的全局缓存载入实例
func (u *Updater) loadGlobalCache() {
	for k, v := range globalCache {
		_ = u.Cache.LoadOrCreate(u, k, v)
	}
}

// Loading 重新加载数据,自动关闭异步数据
// init 立即加载玩家所有数据
func (u *Updater) Loading(cb ...func()) (err error) {
	//🔴 开服自检：Config.BulkWrite 没配的话，**所有句柄的落库都会静默失效** ——
	//save 报出的 ErrBulkWriteNotInit 会被 submit 吞成一行 Alert，玩家一路正常玩、
	//一行数据都没落库，重启才发现。
	//先于 store.Loading 检查：工厂变量是本包配置面，nil 判定必须以它为准。
	if Config.BulkWrite == nil {
		return ErrBulkWriteNotInit
	}
	//全局缓存载入插在 cb 尾部：hamster.Loading 内部的顺序是
	//handles → 时钟 → cb → hamster全局缓存 → Emit(Init)，
	//追加的闭包在 Emit 之前执行，与拆分前「globalCache 先于 Init 事件」的时序一致。
	cb = append(cb, u.loadGlobalCache)
	u.syncErrIn()
	err = u.store.Loading(cb...)
	u.syncErrOut()
	return
}

// Reset 重置,每次请求开始时调用
func (u *Updater) Reset(t ...time.Time) {
	u.syncErrIn()
	u.store.Reset(t...)
	u.syncErrOut()
}

// Release 释放并返回所有已执行的操作,每次请求结束时调用
// 无论有无错误,都应该执行Release
// Release 返回的错误仅代表本次请求过程中某一步产生的错误,不代表Release本身有错误
func (u *Updater) Release() {
	u.syncErrIn()
	u.store.Release() //内部含 Emit(EventTypeRelease)：错误闸门对 Release 放行
	u.syncErrOut()    //u.Error/bulkWrite/status 的清理在 store.Release 内完成
	for _, op := range u.dirty {
		op.Release()
	}
	u.dirty = nil
	u.CreditAllowed = false
}

func (u *Updater) Emit(t EventType) {
	u.syncErrIn()
	u.store.Emit(hamster.EventType(t))
}

// Add 添加道具,num 支持 int32|int64
func (u *Updater) Add(iid int32, num any) {
	if w := u.handleWithKey(iid); w != nil {
		if h, ok := w.(itemHandle); ok {
			h.increase(iid, dataset.ParseInt64(num))
		}
	}
}

// Sub 扣除道具,num 支持 int32|int64
func (u *Updater) Sub(iid int32, num any) {
	if w := u.handleWithKey(iid); w != nil {
		if h, ok := w.(itemHandle); ok {
			h.decrease(iid, dataset.ParseInt64(num))
		}
	}
}

// Get 通过 iid 获取原始数据，返回类型取决于 Handle 类型
func (u *Updater) Get(iid int32) (r any) {
	if w := u.handleWithKey(iid); w != nil {
		r = w.Get(iid)
	}
	return
}

// Val 通过 iid 获取数值
func (u *Updater) Val(iid int32) (r int64) {
	if w := u.handleWithKey(iid); w != nil {
		r = w.Val(iid)
	}
	return
}

// Select 预拉取指定 key 的数据，非内存模式时在 Data 阶段从数据库加载
func (u *Updater) Select(keys ...any) {
	for _, k := range keys {
		if w := u.handleWithKey(k); w != nil {
			w.Select(k)
		}
	}
}

func (u *Updater) Data() (err error) {
	u.syncErrIn()
	err = u.store.Data()
	u.syncErrOut()
	return
}

// Verify 手动执行校验：把当前已入队的操作全部跑完 data→verify，但不落库。
//
// 用于"接口内还要单独写库"的场景(如领邮件后改邮件状态)：先 Verify 确认道具发得出去，
// 再写自己那张表，避免出现"表已改、道具没发"——handle 返回后框架才 Submit，
// 那时报错只会把回包改成错误码，不会回滚 handle 内已落库的写操作。
//
// 校验失败必须把 error 返回给上层：Parse 的错误不置 u.Error，靠 handle 返回非零 code
// 让框架跳过后续 Submit(yyds/context/service.go)；吞掉它会落库半成品。
//
// Verify 之后再 Submit 是安全的：status 已被消耗，Submit 的收敛循环直接跳过，
// 走 commit 落库已 Parse 进 cache 的操作。
func (u *Updater) Verify() (err error) {
	u.syncErrIn()
	err = u.store.Verify()
	u.syncErrOut()
	return
}

// Submit 收敛循环执行 data→verify→commit 直到无新操作产生，最多100轮防止死循环
// 返回本次请求所有操作的 Operator 列表，用于同步给前端
func (u *Updater) Submit() (r []*operator.Operator, err error) {
	u.syncErrIn()
	_, err = u.store.Submit()
	u.syncErrOut()
	if err != nil {
		return nil, err
	}
	//变更流水在根包这份 dirty（句柄默认接收器 pushDirty 收集），
	//store.Submit 返回的是核心版那份（核心版默认 Discard，恒空）。
	r = u.dirty
	u.dirty = nil
	return
}

// IType 通过iid获取IType
// 始终按全局 Config.IType 查询,不受模型 ModelIType 覆盖影响,需要模型口径时用 Handle.IType
func (u *Updater) IType(iid int32) (it IType) {
	if id := Config.IType(iid); id != 0 {
		it = itypesDict[id]
	}
	return
}

// ParseId 通过OID 或者IID 获取iid
func (u *Updater) ParseId(key any) (iid int32, err error) {
	if v, ok := key.(string); ok {
		iid, err = Config.ParseId(u, v)
	} else {
		iid = dataset.ParseInt32(key)
	}
	return
}

// handle 通过 iid 或 oid 路由到对应的 Handle 实例
func (u *Updater) handleWithKey(k any) Handle {
	iid, err := u.ParseId(k)
	if err != nil {
		logger.Alert("%v", err)
		return nil
	}
	itk := Config.IType(iid)
	model, ok := modelsDict[itk]
	if !ok {
		logger.Debug("Updater.handle not exists,iid:%v IType:%v", k, itk)
		return nil
	}
	return u.handleWithAny(model.name)
}

func (u *Updater) handleWithAny(name any) Handle {
	switch k := name.(type) {
	case string:
		return u.store.Handle(k)
	case int:
		return u.handleWithIType(int32(k))
	case int32:
		return u.handleWithIType(k)
	case int64:
		return u.handleWithIType(int32(k))
	default:
		if rv := reflect.ValueOf(name); rv.CanInt() {
			return u.handleWithIType(int32(rv.Int()))
		}
		return nil
	}
}

func (u *Updater) handleWithIType(id int32) Handle {
	mod := modelsDict[id]
	if mod == nil {
		return nil
	}
	return u.store.Handle(mod.name)
}

// Handles 返回本次请求要驱动的全部句柄：**全局注册句柄 + 临时挂载句柄**。
//
// Data / converge / Submit / Reset / Release / Save / Reload 全部基于它，
// 临时句柄接进这里即被全流程覆盖。
//
// ⚠️ 两类句柄之间**没有顺序契约**：临时句柄不共享 IType 路由、不与全局句柄互相产生操作，
// 同批次原子性由共享 bulkWrite 保证，与遍历顺序无关。
func (u *Updater) Handles() []Handle {
	return u.store.Handles()
}

// Destroy 销毁用户实例,强制将缓存数据改变写入数据库,返回错误时无法写入数据库,应该排除问题后后再次尝试销毁
// 仅缓存模式下需要且必要执行
func (u *Updater) Destroy() (err error) {
	u.syncErrIn()
	if err = u.store.Destroy(); err != nil {
		u.syncErrOut()
		return
	}
	u.syncErrOut()
	for _, op := range u.dirty {
		op.Release()
	}
	u.dirty = nil
	updaters.Delete(u.store)
	return
}

// Dirty 设置脏数据,手动更新到客户端,不进行任何操作
// Operators 本次请求已产生的操作列表(只读)
//
// 用于「本次请求动了哪些数据」这类判断,如属性变更感知、埋点、审计。
//
// ⚠ 只在 EventTypeSuccess / EventTypeRelease 事件中有意义:
// Submit 返回时会把 dirty 交给调用方并置 nil,之后再取就是空的。
// 返回的切片不要修改,需要追加用 Dirty()。
func (u *Updater) Operators() []*operator.Operator {
	return u.dirty
}

func (u *Updater) Dirty(opt ...*operator.Operator) {
	u.dirty = append(u.dirty, opt...)
}

func (u *Updater) WriteAble() error {
	//直接读 store 那份：生命周期中途（尚未 syncErrOut）它也比镜像新鲜
	if u.store == nil {
		return u.Error
	}
	return u.store.WriteAble()
}

func (u *Updater) Values(name any) *Values {
	i := u.handleWithAny(name)
	if i == nil {
		return nil
	}
	r, _ := i.(*Values)
	return r
}
func (u *Updater) Virtual(name any) *Virtual {
	i := u.handleWithAny(name)
	if i == nil {
		return nil
	}
	r, _ := i.(*Virtual)
	return r
}
func (u *Updater) Document(name any) *Document {
	i := u.handleWithAny(name)
	if i == nil {
		return nil
	}
	r, _ := i.(*Document)
	return r
}

func (u *Updater) Collection(name any) *Collection {
	i := u.handleWithAny(name)
	if i == nil {
		return nil
	}
	r, _ := i.(*Collection)
	return r
}
