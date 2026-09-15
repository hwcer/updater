package hamster

import (
	"fmt"
	"slices"
	"time"

	"github.com/hwcer/cosgo/schema"
	"github.com/hwcer/updater/dataset"
	"github.com/hwcer/updater/operator"
)

// MountModel 挂载/临时集合模型。
//
// 组合 schema.Tabler 是必需的：挂载名取自 TableName()，而 CollectionModel 本身不含它。
// ⚠️ Getter 只会收到**非空**的 keys —— 挂载是按需加载的，从不做全量拉取。
type MountModel interface {
	CollectionModel
	schema.Tabler
}

// errorState 错误状态外接钩子。
//
// 扩展层(updater)在 New 时注入，让核心版的错误读写命中扩展层实例的公开 Error 字段 ——
// 那是 API 冻结面，用户直接读写 `updater.Updater.Error`。nil 时用本地 Error 字段。
type errorState interface {
	GetError() error
	SetError(error)
}

// Store 核心版存储引擎：颊囊预载（内存缓存）→ 囤货入仓（批量落库）→ 记得囤了什么（脏标记）。
//
// 每个数据属主（公会/玩家/临时副本）持有一个实例，通过
// Loading → Reset → Set/Del/Add → Submit → Release 驱动请求周期。
// 与扩展层 updater.Updater 的关系：扩展层**持有**本类型（🔴 不内嵌，方法提升没有虚派发），
// 生命周期逐方法显式委托，见 HAMSTER_PLAN.md 第七节。
type Store struct {
	now       time.Time            //当前请求时间
	last      time.Time            //上次请求时间，用于判断数据是否需要重置
	dirty     []*operator.Operator //本次请求产生的操作列表（经接收器收集）
	entity    Entity               //数据属主
	status    Status               //状态位：Init/Submit/Changed/Operated
	handles   map[string]Handle    //已注册的数据 Handle
	mounts    map[string]*Collection //临时挂载的数据集合，见 Mount
	bulkWrite BulkWrite            //共享 BulkWrite 实例，Submit 末尾一次原子提交

	errState errorState //错误状态外接钩子（扩展层注入），nil 时用本地 Error

	emitHook func(s *Store, t EventType) //扩展层事件桥：内部 Emit 前先交给扩展层分发

	Cache         Cache       //自定义缓存数据
	Error         error       //本地错误状态（errState 为 nil 时生效）
	Events        Events      //生命周期事件
	Middleware    Middlewares //中间件，所有事件类型都会触发
	CreditAllowed bool        //本次请求是否允许扣量为负（一次性标记）
}

func New(e Entity) *Store {
	return &Store{entity: e, Cache: Cache{}, Events: Events{}, Middleware: Middlewares{}}
}

// SetErrorState 错误状态外接（扩展层注入）：注入后 store 的错误读写命中外部实例
func (s *Store) SetErrorState(es errorState) { s.errState = es }

// SetEmitHook 事件桥（扩展层注入）：内部 Emit 时先回调，再走 hamster 自己的监听器
func (s *Store) SetEmitHook(f func(s *Store, t EventType)) { s.emitHook = f }

func (s *Store) Id() string {
	return s.entity.Id()
}

func (s *Store) Entity() Entity {
	return s.entity
}

func (s *Store) Now() time.Time {
	return s.now
}

// Last 上次请求时间（零值表示本实例尚未处理过请求），ModelReset 等跨天重置判定用
func (s *Store) Last() time.Time {
	return s.last
}

func (s *Store) Unix() int64 {
	return s.now.Unix()
}

func (s *Store) Milli() int64 {
	return s.now.UnixMilli()
}

// ---------------- 错误状态（经 errState 外接或本地） ----------------

func (s *Store) getError() error {
	if s.errState != nil {
		return s.errState.GetError()
	}
	return s.Error
}

func (s *Store) setError(err error) {
	if s.errState != nil {
		s.errState.SetError(err)
		return
	}
	s.Error = err
}

func (s *Store) Errorf(format any, args ...any) error {
	var err error
	switch v := format.(type) {
	case string:
		err = fmt.Errorf(v, args...)
	case error:
		err = v
	default:
		err = fmt.Errorf("%v", v)
	}
	s.setError(err)
	return err
}

func (s *Store) WriteAble() error {
	return s.getError()
}

// ---------------- 事件与扩展 ----------------

func (s *Store) On(t EventType, handle Listener) {
	s.Events.On(t, handle)
}

// Emit 内部事件分发：错误闸门 → 扩展层桥 → hamster 监听器 → 中间件。
func (s *Store) Emit(t EventType) {
	if s.getError() != nil && t != EventTypeRelease {
		return
	}
	if s.emitHook != nil {
		s.emitHook(s, t)
	}
	s.Events.emit(s, t)
	s.Middleware.emit(s, t)
}

// ---------------- 生命周期 ----------------

// Loader 是否已完成初始化加载
func (s *Store) Loader() bool {
	return s.status.Has(StatusInit)
}

// Develop 设置或获取开发者模式标记，仅供业务层自取
func (s *Store) Develop(v ...bool) bool {
	if len(v) > 0 {
		if v[0] {
			s.status.Set(StatusDevelop)
		} else {
			s.status.Unset(StatusDevelop)
		}
	}
	return s.status.Has(StatusDevelop)
}

// Testing 测试模式开关，开启后所有操作仅在内存生效不写库，关闭时强制从数据库重新加载
func (s *Store) Testing(on bool) error {
	if on {
		s.status.Set(StatusTesting)
		return nil
	}
	if !s.status.Has(StatusTesting) {
		return nil
	}
	s.status.Unset(StatusTesting)
	return s.Reload()
}

// Reload 丢弃所有已加载的内存数据，下次访问时重新从数据库读取。
// 用于「带外改动了数据库、要求内存跟上」的场景。
// 调用方必须持有属主锁（与其它 Store 方法一致）。
func (s *Store) Reload() error {
	for _, w := range s.Handles() {
		if err := w.Reload(); err != nil {
			return err
		}
	}
	return nil
}

// Loading 重新加载数据。init 立即加载属主所有数据。
//
// 🔴 开服自检：BulkWrite 工厂没配的话，**所有句柄的落库都会静默失效** ——
// 在数据第一次加载时就拦下来，这时还没产生任何数据改动，报错干净。
func (s *Store) Loading(cb ...func()) (err error) {
	if s.status.Has(StatusInit) {
		return
	}
	if Config.BulkWrite == nil {
		return ErrBulkWriteNotInit
	}
	s.status.Set(StatusInit)

	if s.handles == nil {
		s.handles = make(map[string]Handle)
	}
	for _, model := range modelsRank {
		name := model.name
		handle := s.handles[name]
		if handle == nil {
			handle = model.factory(s, model)
			s.handles[name] = handle
		}
		if err = handle.Loading(); err != nil {
			//回退标志:否则幂等闸门(status.Has(StatusInit))会让重试静默返回nil,
			//属主带着缺数据的句柄进业务
			s.status.Unset(StatusInit)
			return
		}
	}

	if s.now.IsZero() {
		s.now = time.Now()
	}
	s.last = s.now

	for _, f := range cb {
		f()
	}

	for k, v := range globalCache {
		_ = s.Cache.LoadOrCreate(s, k, v)
	}
	s.Emit(EventTypeInit)

	return
}

// Reset 重置,每次请求开始时调用
func (s *Store) Reset(t ...time.Time) {
	if len(t) > 0 {
		s.now = t[0]
	} else {
		s.now = time.Now()
	}
	if s.now.IsZero() {
		_ = s.Errorf("获取系统时间失败")
	}
	s.status.Set(StatusSubmit) // 确保 Submit 收敛循环至少执行一次
	for _, w := range s.Handles() {
		w.Reset()
	}

	if disaster.Load() > 0 {
		s.setError(ErrServerDeniedService) //存在灾难性错误，拒绝服务
	} else {
		s.Emit(EventTypeReset)
	}
}

// Release 释放,每次请求结束时调用。无论有无错误,都应该执行Release。
func (s *Store) Release() {
	s.Emit(EventTypeRelease)
	s.last = s.now
	for _, op := range s.dirty {
		op.Release()
	}
	s.dirty = nil
	s.status = s.status & (StatusInit | StatusTesting | StatusDevelop)
	s.bulkWrite = nil
	s.setError(nil)
	s.CreditAllowed = false
	hs := s.Handles()
	for _, h := range slices.Backward(hs) {
		h.Release()
	}
	//临时句柄的卸载收在这里:Unmount 只打标记,句柄留到请求走完整条生命周期
	//(Data/Verify/Commit 一样不落)才摘除,短流程与长流程走同一条路。
	for k, h := range s.mounts {
		if h.unmount {
			delete(s.mounts, k)
		}
	}
}

// Data 拉取所有句柄 Select 标记的数据
func (s *Store) Data() (err error) {
	hs := s.Handles()
	return s.data(hs)
}

// Verify 手动执行校验：把当前已入队的操作全部跑完 data→verify，但不落库。
// Verify 之后再 Submit 是安全的：status 已被消耗，Submit 的收敛循环直接跳过。
func (s *Store) Verify() (err error) {
	if err = s.WriteAble(); err != nil {
		return err
	}
	return s.converge()
}

// converge data→verify 收敛循环，直到不再产生新操作，最多 100 轮防止死循环。
//
// 核心版没有溢出分解，通常一轮收敛；扩展层的 creator/overflow 会产生新操作，
// 多轮循环的机制保留（机制通用，动机归道具层）。
func (s *Store) converge() (err error) {
	hs := s.Handles()
	loop := int8(1)
	for s.status.Has(StatusSubmit, StatusChanged, StatusOperated) {
		if err = s.data(hs); err != nil {
			return
		}
		if err = s.verify(hs); err != nil {
			return
		}
		s.status.Unset(StatusSubmit)
		s.Emit(EventTypeSubmit)
		if loop = loop + 1; loop >= 100 {
			s.setError(ErrSubmitEndlessLoop)
			return ErrSubmitEndlessLoop
		}
	}
	return
}

func (s *Store) data(hs []Handle) (err error) {
	if err = s.getError(); err != nil {
		return
	}
	if !s.status.Has(StatusChanged) {
		return
	}
	s.status.Unset(StatusChanged)
	s.Emit(EventTypeData)
	for _, w := range hs {
		if err = w.Data(); err != nil {
			return
		}
	}
	return
}

func (s *Store) verify(hs []Handle) (err error) {
	if err = s.getError(); err != nil {
		return
	}
	if !s.status.Has(StatusOperated) {
		return
	}
	s.status.Unset(StatusOperated)
	s.Emit(EventTypeVerify)
	for _, h := range slices.Backward(hs) {
		if err = h.Verify(); err != nil {
			return
		}
	}
	return
}

// Submit 收敛循环执行 data→verify→commit 直到无新操作产生，最多100轮防止死循环。
// 返回本次请求所有操作的 Operator 列表（默认接收器为 Discard 时为空，需变更记录装自己的接收器）。
func (s *Store) Submit() (r []*operator.Operator, err error) {
	if err = s.WriteAble(); err != nil {
		return nil, err
	}
	if err = s.converge(); err != nil {
		return
	}
	hs := s.Handles()
	for _, h := range slices.Backward(hs) {
		if err = h.Commit(); err != nil {
			return
		}
	}
	if s.bulkWrite != nil {
		if s.status.Has(StatusTesting) {
			s.bulkWrite = nil
		} else if err = s.bulkWrite.Submit(); err != nil {
			return
		}
	}
	s.Emit(EventTypeSuccess)
	r = s.dirty
	s.dirty = nil
	return
}

// Save 保存所有缓存数据
func (s *Store) Save() (err error) {
	for _, w := range s.Handles() {
		if err = w.Save(); err != nil {
			return
		}
	}
	return
}

// Destroy 销毁实例,强制将缓存数据改变写入数据库。
// 仅缓存模式下需要且必要执行；返回错误时排除问题后再次尝试销毁。
func (s *Store) Destroy() (err error) {
	hs := s.Handles()
	for _, h := range slices.Backward(hs) {
		if err = h.Destroy(); err != nil {
			return
		}
	}
	if s.bulkWrite != nil {
		if !s.status.Has(StatusTesting) {
			err = s.bulkWrite.Submit()
		}
		s.bulkWrite = nil
	}
	s.entity = nil
	for _, op := range s.dirty {
		op.Release()
	}
	s.handles = nil
	s.mounts = nil
	s.dirty = nil
	return
}

// ---------------- 定位与挂载 ----------------

// Handles 返回本次请求要驱动的全部句柄：**全局注册句柄 + 临时挂载句柄**。
// ⚠️ 两类句柄之间没有顺序契约；具体到实现：临时句柄追加在尾部，而 Commit/Verify/Release
// 是倒序遍历 —— 也就是说它们实际最先跑。别在这个次序上建立任何依赖。
func (s *Store) Handles() (r []Handle) {
	r = make([]Handle, 0, len(modelsRank)+len(s.mounts))
	for _, model := range modelsRank {
		if h := s.handles[model.name]; h != nil {
			r = append(r, h)
		}
	}
	for _, h := range s.mounts {
		r = append(r, h)
	}
	return
}

// Handle 按注册名取句柄
func (s *Store) Handle(name string) Handle {
	return s.handles[name]
}

// Document 取单文档句柄（主档等），未注册返回 nil
func (s *Store) Document(name string) *Document {
	i := s.handles[name]
	if i == nil {
		return nil
	}
	r, _ := i.(*Document)
	return r
}

// Collection 取集合句柄，未注册返回 nil
func (s *Store) Collection(name string) *Collection {
	i := s.handles[name]
	if i == nil {
		return nil
	}
	r, _ := i.(*Collection)
	return r
}

// Values 取数值 KV 句柄，未注册返回 nil
func (s *Store) Values(name string) *Values {
	i := s.handles[name]
	if i == nil {
		return nil
	}
	r, _ := i.(*Values)
	return r
}

// Virtual 取委托视图句柄，未注册返回 nil
func (s *Store) Virtual(name string) *Virtual {
	i := s.handles[name]
	if i == nil {
		return nil
	}
	r, _ := i.(*Virtual)
	return r
}

// Mount 挂载/取回一个临时数据集合，keys 非空时顺带把这几条**当场查出来**。
//
// 幂等：同模型重复 Mount 直接返回已挂句柄。带 keys 时等价于 Select(keys...) + Data()。
// 挂载名取 model.TableName()，与已注册的全局模型重名时报错。
//
// ⚠️ 不带 keys 时**不做任何预加载**。⚠️ 查库失败时返回 (句柄, err) —— 句柄已挂上可用，
// 重试一次 Select + Data 即可；唯一会返回 nil 的是重名。
func (s *Store) Mount(m MountModel, keys ...string) (*Collection, error) {
	name := m.TableName()
	r, exist := s.mounts[name]
	if !exist {
		if findModel(name) != nil {
			return nil, Errorf(0, "mount name conflicts with registered model:%v", name)
		}
		if s.mounts == nil {
			s.mounts = make(map[string]*Collection)
		}
		//ram 强制 RAMTypeMaybe：只影响 statement.Has 里 `Always && loader` 那条短路，
		//绝不能命中——命中之后 Select 会跳过每一个 key，Data 永不执行、Get 全 nil 且不报错。
		r = &Collection{name: name, model: m, dataset: dataset.NewColl()}
		r.Statement = *NewStatement(s, RAMTypeMaybe, r.exist)
		r.Statement.Receiver(DiscardReceiver)
		r.Reset()
		s.mounts[name] = r
	}
	r.unmount = false //改主意了:上一次标记的卸载作废
	if len(keys) == 0 {
		return r, nil
	}
	for _, k := range keys {
		r.Select(k)
	}
	return r, r.Data()
}

// Mounted 取回已挂载的临时集合，未挂载返回 nil。只取不挂，也不取数。
func (s *Store) Mounted(m MountModel) *Collection {
	return s.mounts[m.TableName()]
}

// Mounts 挂载表（只读视图，键为挂载名）。Destroy 后为空。
func (s *Store) Mounts() map[string]*Collection {
	return s.mounts
}

// Unmount 标记卸载。**只打标记，真正摘除在 Release 阶段**。
// ⚠️ 卸载粒度是**整张表**；判断不了是不是最后一个使用者就别卸，留给下线兜底（Destroy 刷盘）。
func (s *Store) Unmount(m MountModel) {
	if r, ok := s.mounts[m.TableName()]; ok {
		r.unmount = true
	}
}

// ---------------- 脏数据（变更流水） ----------------

// Operators 本次请求已产生的操作列表(只读)。
// ⚠️ 只在 EventTypeSuccess / EventTypeRelease 事件中有意义。
func (s *Store) Operators() []*operator.Operator {
	return s.dirty
}

// Dirty 手动追加要下发的 operator，不做任何校验
func (s *Store) Dirty(opt ...*operator.Operator) {
	s.dirty = append(s.dirty, opt...)
}

// ---------------- BulkWrite ----------------

func (s *Store) BulkWrite() BulkWrite {
	if s.bulkWrite == nil && Config.BulkWrite != nil {
		s.bulkWrite = Config.BulkWrite(s)
	}
	return s.bulkWrite
}
