package updater

import (
	"fmt"
	"reflect"
	"time"

	"github.com/hwcer/logger"
	"github.com/hwcer/updater/dataset"
	"github.com/hwcer/updater/operator"
)

type Player interface {
	Uid() string
}

type Updater struct {
	now           time.Time
	init          bool                 //初始化,false-不初始化，实时读写数据库  true-按照模块预设进行初始化，
	last          int64                //上次请求的时间时间戳，用于判断数据是否需要重置
	dirty         []*operator.Operator //临时操作,不涉及数据,直接返回给客户端
	player        Player               //业务层角色对象
	submit        bool                 //是否需要触发提交，默认每次都强制触发一次
	changed       bool                 //数据变动,需要使用Data更新数据
	operated      bool                 //新操作需要重执行Verify检查数据
	handles       map[string]Handle    //Handle
	Error         error
	Events        Events
	Process       Process
	CreditAllowed bool //是否扣钱时是否允许负债，每次设置仅仅一次性有效
}

func New(p Player) *Updater {
	return &Updater{player: p, Process: Process{}}
}

func (u *Updater) On(t EventType, handle Listener) {
	u.Events.On(t, handle)
}

func (u *Updater) Uid() string {
	return u.player.Uid()
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
func (u *Updater) Player() Player {
	return u.player
}

func (u *Updater) Errorf(format any, args ...any) error {
	switch v := format.(type) {
	case string:
		u.Error = fmt.Errorf(v, args...)
	case error:
		u.Error = v
	default:
		u.Error = fmt.Errorf("%v", v)
	}
	return u.Error
}

// Save 保存所有缓存数并自动关闭异步模式
func (u *Updater) Save() (err error) {
	for _, w := range u.Handles() {
		if err = w.save(); err != nil {
			return
		}
	}
	return
}

func (u *Updater) Loader() bool {
	return u.init
}

// Loading 重新加载数据,自动关闭异步数据
// init 立即加载玩家所有数据
func (u *Updater) Loading(init bool, cb ...func()) (err error) {
	if u.init {
		return
	}
	if init {
		u.init = true
	}
	if u.handles == nil {
		u.handles = make(map[string]Handle)
	}
	for _, model := range modelsRank {
		name := model.name
		handle := u.handles[name]
		if handle == nil {
			handle = handles[model.parser](u, model)
			u.handles[name] = handle
		}
		if err = handle.loading(); err != nil {
			return
		}
	}
	for _, f := range cb {
		f()
	}
	for k, f := range processDefault {
		u.Process.GetOrCreate(u, k, f)
	}
	if u.init {
		u.emit(EventTypeInit)
	}
	u.last = u.now.Unix()
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
	u.submit = true
	for _, w := range u.Handles() {
		w.reset()
	}

	if disaster.Load() > 0 {
		u.Error = ErrServerDeniedService //存在灾难性错误，拒绝服务
	} else {
		u.emit(EventTypeReset)
	}
}

// Release 释放并返回所有已执行的操作,每次请求结束时调用
// 无论有无错误,都应该执行Release
// Release 返回的错误仅代表本次请求过程中某一步产生的错误,不代表Release本身有错误
func (u *Updater) Release() {
	u.emit(EventTypeRelease)
	u.last = u.now.Unix()
	for _, op := range u.dirty {
		op.Release()
	}
	u.dirty = nil
	u.changed = false
	u.operated = false
	u.Error = nil
	u.CreditAllowed = false
	hs := u.Handles()
	for i := len(hs) - 1; i >= 0; i-- {
		hs[i].release()
	}
}

func (u *Updater) emit(t EventType) {
	u.Events.emit(u, t)
}

// Add 添加道具,num 支持 int32|int64
func (u *Updater) Add(iid int32, num any) {
	if w := u.handle(iid); w != nil {
		w.increase(iid, dataset.ParseInt64(num))
	}
}

// Sub 扣除道具,num 支持 int32|int64
func (u *Updater) Sub(iid int32, num any) {
	if w := u.handle(iid); w != nil {
		w.decrease(iid, dataset.ParseInt64(num))
	}
}

// Get 通过 iid 获取原始数据，返回类型取决于 Handle 类型
func (u *Updater) Get(iid int32) (r any) {
	if w := u.handle(iid); w != nil {
		r = w.Get(iid)
	}
	return
}

// Val 通过 iid 获取数值
func (u *Updater) Val(iid int32) (r int64) {
	if w := u.handle(iid); w != nil {
		r = w.Val(iid)
	}
	return
}

// Select 预拉取指定 key 的数据，非内存模式时在 Data 阶段从数据库加载
func (u *Updater) Select(keys ...any) {
	for _, k := range keys {
		if w := u.handle(k); w != nil {
			w.Select(k)
		}
	}
}

func (u *Updater) Data() (err error) {
	hs := u.Handles()
	return u.data(hs)
}

// Verify 手动执行Verify对操作进行检查
func (u *Updater) Verify() (err error) {
	if err = u.WriteAble(); err != nil {
		return err
	}
	hs := u.Handles()
	return u.verify(hs)
}

func (u *Updater) data(hs []Handle) (err error) {
	if err = u.Error; err != nil {
		return
	}
	if !u.changed {
		return
	}
	u.changed = false
	u.emit(EventTypeData)
	for _, w := range hs {
		if err = w.Data(); err != nil {
			return
		}
	}
	return
}

func (u *Updater) verify(hs []Handle) (err error) {
	if err = u.Error; err != nil {
		return
	}
	if !u.operated {
		return
	}
	u.operated = false
	u.emit(EventTypeVerify)
	for i := len(hs) - 1; i >= 0; i-- {
		if err = hs[i].verify(); err != nil {
			return
		}
	}
	return
}

// Submit 收敛循环执行 data→verify→submit 直到无新操作产生，最多100轮防止死循环
// 返回本次请求所有操作的 Operator 列表，用于同步给前端
func (u *Updater) Submit() (r []*operator.Operator, err error) {
	if err = u.WriteAble(); err != nil {
		return nil, err
	}
	hs := u.Handles()

	loop := int8(1)
	for u.submit || u.changed || u.operated {
		if err = u.data(hs); err != nil {
			return
		}
		if err = u.verify(hs); err != nil {
			return
		}
		u.submit = false
		u.emit(EventTypeSubmit)
		if loop = loop + 1; loop >= 100 {
			u.Error = ErrSubmitEndlessLoop
			return nil, u.Error
		}
	}
	for i := len(hs) - 1; i >= 0; i-- {
		if err = hs[i].submit(); err != nil {
			return
		}
	}
	u.emit(EventTypeSuccess)
	r = u.dirty
	u.dirty = nil
	return
}

// IType 通过iid获取IType
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

// Handle 根据 name(string) || itype(int32) 查找，支持命名整型（如 protobuf 枚举）
func (u *Updater) Handle(name any) Handle {
	switch k := name.(type) {
	case string:
		return u.handles[k]
	case int:
		return u.handleByIType(int32(k))
	case int32:
		return u.handleByIType(k)
	case int64:
		return u.handleByIType(int32(k))
	default:
		if rv := reflect.ValueOf(name); rv.CanInt() {
			return u.handleByIType(int32(rv.Int()))
		}
		return nil
	}
}

// handle 通过 iid 或 oid 路由到对应的 Handle 实例
func (u *Updater) handle(k any) Handle {
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
	return u.Handle(model.name)
}

func (u *Updater) handleByIType(id int32) Handle {
	mod := modelsDict[id]
	if mod == nil {
		return nil
	}
	return u.handles[mod.name]
}

func (u *Updater) Handles() (r []Handle) {
	r = make([]Handle, 0, len(modelsRank))
	for _, model := range modelsRank {
		r = append(r, u.handles[model.name])
	}
	return
}

// Destroy 销毁用户实例,强制将缓存数据改变写入数据库,返回错误时无法写入数据库,应该排除问题后后再次尝试销毁
// 仅缓存模式下需要且必要执行
func (u *Updater) Destroy() (err error) {
	hs := u.Handles()
	for i := len(hs) - 1; i >= 0; i-- {
		if err = hs[i].destroy(); err != nil {
			return
		}
	}
	u.player = nil
	for _, op := range u.dirty {
		op.Release()
	}
	u.handles = nil
	u.dirty = nil
	return
}

// Dirty 设置脏数据,手动更新到客户端,不进行任何操作
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
	i := u.Handle(name)
	if i == nil {
		return nil
	}
	r, _ := i.(*Values)
	return r
}
func (u *Updater) Virtual(name any) *Virtual {
	i := u.Handle(name)
	if i == nil {
		return nil
	}
	r, _ := i.(*Virtual)
	return r
}
func (u *Updater) Document(name any) *Document {
	i := u.Handle(name)
	if i == nil {
		return nil
	}
	r, _ := i.(*Document)
	return r
}

func (u *Updater) Collection(name any) *Collection {
	i := u.Handle(name)
	if i == nil {
		return nil
	}
	r, _ := i.(*Collection)
	return r
}
