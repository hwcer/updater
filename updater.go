package updater

import (
	"fmt"
	"reflect"
	"slices"
	"time"

	"github.com/hwcer/cosgo/values"
	"github.com/hwcer/logger"
	"github.com/hwcer/updater/dataset"
	"github.com/hwcer/updater/operator"
)

// Entity 数据属主：Updater 所管理数据的身份主体。
//
// 玩家、公会、临时副本……凡"拥有自己一份独立数据空间"的东西都是一个 Entity：
// 一个 Entity 对应一个 Updater 实例，其下有自己的道具、每日数据、文档集合。
// 多域场景（Manage）下，公会域里的 Entity 就是一个公会 —— 一个公会就相当于一个玩家。
//
// 业务层在自己的身份对象上实现本接口把框架挂上去；模型回调（Getter/Setter）里
// 也经 u.Entity() 拿到它拼落库条件（如 where uid = e.Id()）。
type Entity interface {
	Id() string
}

// Updater 数据属主的数据更新器，管理所有 Handle 的生命周期和持久化
// 每个 Entity 持有一个 Updater 实例，通过 Reset → Add/Sub/Set → Submit → Release 驱动请求周期
type Updater struct {
	now       time.Time            //当前请求时间
	last      time.Time            //上次请求时间，用于判断数据是否需要重置(零值表示本实例尚未处理过请求)
	dirty     []*operator.Operator //本次请求产生的操作列表，用于同步给客户端
	entity    Entity               //数据属主（业务层实现，见 Entity）
	status    Status               //状态位：Init/Submit/Changed/Operated
	manage    *Manage              //所属数据域：注册表/Config/域级事件缓存的宿主，见 Manage
	handles   map[string]Handle    //已注册的数据 Handle（Document/Collection/Values）
	bulkWrite BulkWrite            //共享 BulkWrite 实例:提交失败跨请求保留(由 Reset/Submit/Destroy 重试),写库成功才清空

	Cache         Cache           //自定义缓存数据
	Error         *values.Message //请求过程中的错误（业务码+参数，可直接作为回包错误下发），一律经 Errorf 写入
	Events        Events          //生命周期事件
	Mounts        Mounts          //临时挂载的数据集合（挂载表，入口见 Mounts.Load）
	Middleware    Middlewares     //中间件，所有事件类型都会触发
	CreditAllowed bool            //本次请求是否允许扣量为负（一次性标记）
}

// Manage 返回所属数据域
func (u *Updater) Manage() *Manage {
	return u.manage
}

func (u *Updater) On(t EventType, handle Listener) {
	u.Events.On(t, handle)
}

func (u *Updater) BulkWrite() BulkWrite {
	//🔴 契约:Updater 按"每玩家单协程"驱动,check-then-act 在契约内安全,
	//不做并发防护——并发驱动同一玩家数据本就超出模块契约
	if u.bulkWrite == nil && u.manage.Config.BulkWrite != nil {
		u.bulkWrite = u.manage.Config.BulkWrite(u)
	}
	return u.bulkWrite
}

// Id 数据属主标识：玩家 uid / 公会 gid / 副本 id
func (u *Updater) Id() string {
	return u.entity.Id()
}

func (u *Updater) Now() time.Time {
	return u.now
}

func (u *Updater) Unix() int64 {
	return u.now.Unix()
}
func (u *Updater) Milli() int64 {
	return u.now.UnixMilli()
}
func (u *Updater) Entity() Entity {
	return u.entity
}

// Errorf 设置错误状态并返回该错误，方便调用方直接抛给上层。**错误状态写入的唯一入口。**
// 直接委托 values.Errorf：*values.Message 原样收存（写时复制，Code/Args 保留）；
// 普通 error/字符串以文案形式收进 Data（错误码归 values 默认码）。
// nil 入参（无格式串无参数）为赋值语义的"清空"：values.Errorf 会把 nil 格式化成
// "<nil>" 文案产出非 nil Message，这里拦下来，保持旧 `u.Error = err`（err 为 nil）的行为。
func (u *Updater) Errorf(format any, args ...any) *values.Message {
	if format != nil {
		u.Error = values.Errorf(0, format, args...)
	}
	return u.Error
}

// Save 将所有句柄当前的脏数据刷入共享 BulkWrite 队列(只入队,不提交;
// 提交时机见 Submit / Destroy / 下次请求 Reset 对遗留队列的重试语义)
func (u *Updater) Save() (err error) {
	for _, w := range u.Handles() {
		if err = w.save(); err != nil {
			return
		}
	}
	return
}

func (u *Updater) Loader() bool {
	return u.status.Has(StatusInit)
}

// Develop 设置或获取开发者模式标记，仅供业务层自取
func (u *Updater) Develop(v ...bool) bool {
	if len(v) > 0 {
		if v[0] {
			u.status.Set(StatusDevelop)
		} else {
			u.status.Unset(StatusDevelop)
		}
	}
	return u.status.Has(StatusDevelop)
}

// Testing 测试模式开关，开启后所有操作仅在内存生效不写库，关闭时强制从数据库重新加载
func (u *Updater) Testing(on bool) error {
	if on {
		u.status.Set(StatusTesting)
		return nil
	}
	if !u.status.Has(StatusTesting) {
		return nil
	}
	u.status.Unset(StatusTesting)
	return u.Reload()
}

// Reload 丢弃所有已加载的内存数据，下次访问时重新从数据库读取。
//
// 用于「带外改动了数据库、要求内存跟上」的场景：运营后台改档、GM 工具导入存档、
// 线上数据修复等——这些改动绕过了 updater，内存里仍是旧值，玩家下一次操作会在旧值上
// 算增量并把脏数据写回去。
//
// 调用方必须持有玩家锁（与其它 Updater 方法一致）。只重置已加载的数据集，不动 status。
//
// 🔴 重载前先清欠账：遗留的 bulkWrite 队列套在旧内存状态上,能成功提交说明数据库可写,
// 不先落库的话重载读到的数据会被队列里陈旧的 $set 再次覆盖。提交失败则直接返回 ——
// 数据库不可写时重载(Getter)必然同样失败,保留内存与队列原状。
//
// ⚠ 对**在线**玩家只重载服务端内存，客户端手上那份仍是旧的，通常还需要让客户端重新拉取。
func (u *Updater) Reload() error {
	if u.bulkWrite != nil {
		if err := u.bulkWrite.Submit(); err != nil {
			onSubmitResult(err)
			return err
		}
		onSubmitResult(nil)
		u.bulkWrite = nil
	}
	for _, w := range u.Handles() {
		if err := w.reload(); err != nil {
			return err
		}
	}
	return nil
}

// Loading 重新加载数据,自动关闭异步数据
// init 立即加载玩家所有数据
func (u *Updater) Loading(cb ...func()) (err error) {
	if u.status.Has(StatusInit) {
		return
	}
	//🔴 开服自检：Config.BulkWrite 没配的话，**所有句柄的落库都会静默失效** ——
	//save 报出的 ErrBulkWriteNotInitialize 会被 submit 吞成一行 Alert，玩家一路正常玩、
	//一行数据都没落库，重启才发现。
	//
	//它相当于"数据库连接"级别的配置（本项目在 model.start() 连完 Mongo 之后设），
	//与其让每个句柄在运行期各自发明一套更严的行为，不如在玩家数据第一次加载时就拦下来：
	//这时还没产生任何数据改动，报错干净。
	if u.manage.Config.BulkWrite == nil {
		return ErrBulkWriteNotInitialize
	}
	u.status.Set(StatusInit)

	if u.handles == nil {
		u.handles = make(map[string]Handle)
	}
	for _, model := range u.manage.modelsRank {
		name := model.name
		handle := u.handles[name]
		if handle == nil {
			handle = u.manage.parser[model.parser](u, model)
			u.handles[name] = handle
		}
		if err = handle.loading(); err != nil {
			//回退标志:否则幂等闸门(status.Has(StatusInit))会让重试静默返回nil,
			//玩家带着缺数据的句柄进游戏
			u.status.Unset(StatusInit)
			return
		}
	}

	if u.now.IsZero() {
		u.now = time.Now()
	}
	u.last = u.now

	for _, f := range cb {
		f()
	}

	for k, v := range u.manage.cacheCreators {
		_ = u.Cache.LoadOrCreate(u, k, v)
	}
	u.Emit(EventTypeInit)

	return
}

// Reset 重置,每次请求开始时调用
func (u *Updater) Reset(t ...time.Time) {
	if len(t) > 0 {
		u.now = t[0]
	} else {
		u.now = time.Now()
	}
	if u.now.IsZero() {
		_ = u.Errorf("获取系统时间失败")
	}
	u.status.Set(StatusSubmit) // 确保 Submit 收敛循环至少执行一次

	//🔴 上次请求遗留的未落库队列先重试:数据库恢复后在**跨天重载(ModelReset)之前**清欠账,
	//否则重载读到的旧值会被队列里陈旧的 $set 覆盖(载荷是绝对值语义,重载后内存已换基)。
	//仍失败只累计计数,达到 BulkWriteMaxFails 触发灾难保护,不阻断本次请求
	if u.bulkWrite != nil {
		if err := u.bulkWrite.Submit(); err != nil {
			onSubmitResult(err)
		} else {
			onSubmitResult(nil)
			u.bulkWrite = nil
		}
	}

	for _, w := range u.Handles() {
		w.reset()
	}

	if disaster.Load() > 0 {
		u.Error = ErrServerDeniedService //存在灾难性错误，拒绝服务
	} else {
		u.Emit(EventTypeReset)
	}
}

// Release 释放并返回所有已执行的操作,每次请求结束时调用
// 无论有无错误,都应该执行Release
// Release 返回的错误仅代表本次请求过程中某一步产生的错误,不代表Release本身有错误
func (u *Updater) Release() {
	u.Emit(EventTypeRelease)
	u.last = u.now
	for _, op := range u.dirty {
		op.Release()
	}
	u.dirty = nil
	u.status &= StatusInit | StatusTesting | StatusDevelop
	//🔴 u.bulkWrite 不在这里清理:提交失败的队列要跨请求保留,
	//由下次请求的 Reset / Submit / Destroy 重试,写库成功才清空
	u.Error = nil
	u.CreditAllowed = false
	hs := u.Handles()
	for _, h := range slices.Backward(hs) {
		h.release()
	}
	//临时句柄的卸载收在这里:Mounts.Remove 只打标记,句柄留到请求走完整条生命周期
	//(Data/verify/submit 一样不落)才摘除,短流程与长流程走同一条路。
	for k, h := range u.Mounts.tables {
		if h.unmount {
			delete(u.Mounts.tables, k)
		}
	}
}

func (u *Updater) Emit(t EventType) {
	u.Events.emit(u, t)
	u.Middleware.emit(u, t)
}

// Add 添加道具,num 支持 int32|int64。
// 🔴 路由失败(解析失败/IType 未初始化/模型未注册)会置 u.Error:旧实现静默忽略,
// 调用方对"什么都没发生"零感知——扣费不生效继续发货=刷道具,发放蒸发=付费未到账。
// 需要自行处理失败的调用方用 AddErr
func (u *Updater) Add(iid int32, num any) {
	if err := u.AddErr(iid, num); err != nil {
		u.Errorf(err)
	}
}

// AddErr 带错误返回的 Add:路由失败返回 error 而非静默忽略
func (u *Updater) AddErr(iid int32, num any) error {
	w, err := u.handleWithKeyErr(iid)
	if err != nil || w == nil {
		return err
	}
	w.increase(iid, dataset.ParseInt64(num))
	return nil
}

// Sub 扣除道具,num 支持 int32|int64。路由失败置 u.Error(语义同 Add)
func (u *Updater) Sub(iid int32, num any) {
	if err := u.SubErr(iid, num); err != nil {
		u.Errorf(err)
	}
}

// SubErr 带错误返回的 Sub:路由失败返回 error 而非静默忽略
func (u *Updater) SubErr(iid int32, num any) error {
	w, err := u.handleWithKeyErr(iid)
	if err != nil || w == nil {
		return err
	}
	w.decrease(iid, dataset.ParseInt64(num))
	return nil
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
	hs := u.Handles()
	return u.data(hs)
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
// 走 submit 落库已 Parse 进 cache 的操作。
func (u *Updater) Verify() (err error) {
	if err = u.WriteAble(); err != nil {
		return err
	}
	return u.converge()
}

// converge data→verify 收敛循环，直到不再产生新操作，最多 100 轮防止死循环。
//
// 单轮是不够的：verify 期间 IType 的 creator/overflow 会产生新操作(自动分解、溢出转化)，
// 跑完 status 会重新变成 StatusOperated，这些新操作必须再过一轮才算校验完整。
// Verify 与 Submit 共用本函数——前者只要校验结论，后者在此之后才落库。
func (u *Updater) converge() (err error) {
	hs := u.Handles()
	loop := int8(1)
	for u.status.Has(StatusSubmit, StatusChanged, StatusOperated) {
		if err = u.data(hs); err != nil {
			return
		}
		if err = u.verify(hs); err != nil {
			return
		}
		u.status.Unset(StatusSubmit)
		u.Emit(EventTypeSubmit)
		if loop = loop + 1; loop >= 100 {
			u.Error = ErrSubmitEndlessLoop
			return u.Error
		}
	}
	return
}

func (u *Updater) data(hs []Handle) (err error) {
	if u.Error != nil {
		return u.Error
	}
	if !u.status.Has(StatusChanged) {
		return
	}
	u.status.Unset(StatusChanged)
	u.Emit(EventTypeData)
	for _, w := range hs {
		if err = w.Data(); err != nil {
			return
		}
	}
	return
}

func (u *Updater) verify(hs []Handle) (err error) {
	if u.Error != nil {
		return u.Error
	}
	if !u.status.Has(StatusOperated) {
		return
	}
	u.status.Unset(StatusOperated)
	u.Emit(EventTypeVerify)
	for _, h := range slices.Backward(hs) {
		if err = h.verify(); err != nil {
			return
		}
	}
	return
}

// Submit 收敛循环执行 data→verify→submit 直到无新操作产生，最多100轮防止死循环
// 返回本次请求所有操作的 Operator 列表，用于同步给前端
//
// 🔴 bulkWrite 提交失败时实例**保留**(连同已入队操作,载荷是绝对值语义可安全重发),
// 由下次请求的 Reset / Submit / Destroy 重试,写库成功才清理;Release 不再丢弃
//
// ⚠ 中途失败的分叉口径:任一句柄 h.submit() 返回错误(仅 RAMTypeNone 的 save 失败会上抛,
// 其余句柄自吞为告警继续)即中止本轮 Submit —— 后续句柄不再落库,内存不回滚,属最终一致语义。
// 先前句柄已入队共享 bulkWrite 的操作不受影响:队列保留待重试,不会丢。
func (u *Updater) Submit() (r []*operator.Operator, err error) {
	if err = u.WriteAble(); err != nil {
		return nil, err
	}
	if err = u.converge(); err != nil {
		return
	}
	hs := u.Handles()
	for _, h := range slices.Backward(hs) {
		if err = h.submit(); err != nil {
			return
		}
	}
	if u.bulkWrite != nil {
		if u.status.Has(StatusTesting) {
			u.bulkWrite = nil //测试模式只改内存不写库,直接丢弃
		} else if err = u.bulkWrite.Submit(); err != nil {
			onSubmitResult(err)
			//🔴 接入错误分级:程序级错误(结构不一致/主键冲突等,重试无意义)丢弃队列,
			//避免同一坏载荷每请求重发,累计到 BulkWriteMaxFails 拖垮全服(灾难保护);
			//旧实现 onSaveErrorHandle 从未被调用,分级链路是死代码
			if retain, newErr := onSaveErrorHandle(u, err); !retain {
				logger.Alert("bulkWrite 程序级错误,丢弃队列不再重试: %v", newErr)
				u.bulkWrite = nil
				return
			}
			return //失败保留队列,等待重试
		} else {
			onSubmitResult(nil)
			u.bulkWrite = nil //写库成功才清理
		}
	}
	u.Emit(EventTypeSuccess)
	r = u.dirty
	u.dirty = nil
	return
}

// ParseId 通过OID 或者IID 获取iid
func (u *Updater) ParseId(key any) (iid int32, err error) {
	if v, ok := key.(string); ok {
		iid, err = u.manage.Config.ParseId(u, v)
	} else {
		iid = dataset.ParseInt32(key)
	}
	return
}

// Handle 根据 name(string) || itype(int32) 查找，支持命名整型（如 protobuf 枚举）

// handle 通过 iid 或 oid 路由到对应的 Handle 实例。
// 路由失败返回 nil(读取类场景"模型不存在"是正常态);写入类场景用 handleWithKeyErr
func (u *Updater) handleWithKey(k any) Handle {
	h, _ := u.handleWithKeyErr(k)
	return h
}

// handleWithKeyErr 带错误返回的路由:写入类操作(Add/Sub)据此把失败上抛,
// 而不是静默丢操作
func (u *Updater) handleWithKeyErr(k any) (Handle, error) {
	iid, err := u.ParseId(k)
	if err != nil {
		return nil, fmt.Errorf("updater ParseId failed, key:%v: %w", k, err)
	}
	//🔴 New() 已装 defaultIType,这里是手工构造 &Options{} 绕过默认值的兜底:
	//缺配置属启动期错误
	if u.manage.Config.IType == nil {
		//IType 未初始化属手工构造 &Options{} 的启动期错误,保持降级兼容
		//(见 TestNilITypeDegradesInsteadOfPanic);运行期路由失败仍上抛
		logger.Alert("updater.Options.IType not initialized: 无法路由 key:%v, 操作被忽略", k)
		return nil, nil
	}
	itk := u.manage.Config.IType(iid)
	model, ok := u.manage.modelsDict[itk]
	if !ok {
		return nil, fmt.Errorf("updater model not registered, iid:%v IType:%v", iid, itk)
	}
	return u.handleWithAny(model.name), nil
}

func (u *Updater) handleWithAny(name any) Handle {
	switch k := name.(type) {
	case string:
		return u.handles[k]
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
	mod := u.manage.modelsDict[id]
	if mod == nil {
		return nil
	}
	return u.handles[mod.name]
}

// Handles 返回本次请求要驱动的全部句柄：**全局注册句柄 + 临时挂载句柄**。
//
// Data / converge / Submit / Reset / Release / Save / Reload 全部基于它，
// 临时句柄接进这里即被全流程覆盖。
//
// ⚠️ 两类句柄之间**没有顺序契约**：临时句柄不共享 IType 路由、不与全局句柄互相产生操作，
// 同批次原子性由共享 bulkWrite 保证，与遍历顺序无关。
// 具体到实现：临时句柄追加在尾部，而 Submit/verify/Release 是**倒序**遍历
// —— 也就是说它们实际最先跑。别在这个次序上建立任何依赖。
func (u *Updater) Handles() (r []Handle) {
	r = make([]Handle, 0, len(u.manage.modelsRank)+len(u.Mounts.tables))
	for _, model := range u.manage.modelsRank {
		if h := u.handles[model.name]; h != nil {
			r = append(r, h)
		}
	}
	for _, h := range u.Mounts.tables {
		r = append(r, h)
	}
	return
}

// Destroy 销毁用户实例,强制将缓存数据改变写入数据库
// 提交失败时队列保留、实体不拆,排除问题后再次调用 Destroy 即可重试同一批(写库成功才清理)
// 仅缓存模式下需要且必要执行
func (u *Updater) Destroy() (err error) {
	hs := u.Handles()
	for _, h := range slices.Backward(hs) {
		if err = h.destroy(); err != nil {
			return
		}
	}
	if u.bulkWrite != nil {
		if u.status.Has(StatusTesting) {
			u.bulkWrite = nil
		} else if err = u.bulkWrite.Submit(); err != nil {
			onSubmitResult(err)
			//失败保留队列并提前返回:实体不提前拆,重试 Destroy 才有完整上下文
			return
		} else {
			onSubmitResult(nil)
			u.bulkWrite = nil
		}
	}
	u.entity = nil
	for _, op := range u.dirty {
		op.Release()
	}
	u.handles = nil
	u.Mounts.tables = nil
	u.dirty = nil
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
	if u.Error != nil {
		return u.Error
	}
	return nil
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
