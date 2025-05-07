package updater

import (
	"errors"
	"fmt"
	"github.com/hwcer/logger"
	"github.com/hwcer/updater/dataset"
	"github.com/hwcer/updater/operator"
	"runtime/debug"
	"time"
)

type Player interface {
	Uid() string
}

type Updater struct {
	now      time.Time
	init     bool                 //初始化,false-不初始化，实时读写数据库  true-按照模块预设进行初始化，
	dirty    []*operator.Operator //临时操作,不涉及数据,直接返回给客户端
	player   Player               //业务层角色对象
	changed  bool                 //数据变动,需要使用Data更新数据
	develop  bool                 //开发者模式，关闭数据库写入,进入内存模式,不影响数据库读操作，退出时可以丢弃内存数据，重新加载数据库数据
	operated bool                 //新操作需要重执行Verify检查数据
	handles  map[string]Handle    //Handle
	Error    error
	Events   Events
	Process  Process
}

func New(p Player) (u *Updater) {
	u = &Updater{player: p, Process: Process{}}
	return u
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

// Develop 设置，并返回当前Debug状态
func (u *Updater) Develop(v ...bool) bool {
	if len(v) > 0 {
		if u.develop && u.develop != v[0] {
			for _, w := range u.Handles() {
				_ = w.reload()
			}
		}
		u.develop = v[0]
	}
	return u.develop
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

// Loading 重新加载数据,自动关闭异步数据
// init 立即加载玩家所有数据
func (u *Updater) Loading(init bool, cb ...func()) (err error) {
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
		u.Process.Try(u, k, f)
	}
	u.emit(EventTypeInit)
	return
}

// Reset 重置,每次请求开始时调用
func (u *Updater) Reset(t ...time.Time) {
	//u.ReadOnly = readOnly
	if len(t) > 0 {
		u.now = t[0]
	} else {
		u.now = time.Now()
	}
	if u.now.IsZero() {
		_ = u.Errorf("获取系统时间失败")
		fmt.Printf("%s\n", string(debug.Stack()))
	}
	for _, w := range u.Handles() {
		w.reset()
	}
}

// Release 释放并返回所有已执行的操作,每次请求结束时调用
// 无论有无错误,都应该执行Release
// Release 返回的错误仅代表本次请求过程中某一步产生的错误,不代表Release本身有错误
func (u *Updater) Release() {
	u.emit(EventTypeRelease)
	u.dirty = nil
	u.changed = false
	u.operated = false
	u.Error = nil
	//u.ReadOnly = false
	hs := u.Handles()
	for i := len(hs) - 1; i >= 0; i-- {
		hs[i].release()
	}
	return
}

func (u *Updater) emit(t EventType) {
	u.Events.emit(u, t)
}

func (u *Updater) Get(id any) (r any) {
	if w := u.handle(id); w != nil {
		r = w.Get(id)
	}
	return
}

func (u *Updater) Set(id any, v ...any) {
	if w := u.handle(id); w != nil {
		w.Set(id, v...)
	}
}

func (u *Updater) Add(iid int32, num int32) {
	if w := u.handle(iid); w != nil {
		w.Add(iid, num)
	}
}

func (u *Updater) Sub(iid int32, num int32) {
	if w := u.handle(iid); w != nil {
		w.Sub(iid, num)
	}
}

func (u *Updater) Val(id any) (r int64) {
	if w := u.handle(id); w != nil {
		r = w.Val(id)
	}
	return
}

func (u *Updater) Del(id any) {
	if w := u.handle(id); w != nil {
		w.Del(id)
	}
}

func (u *Updater) Select(keys ...any) {
	for _, k := range keys {
		if w := u.handle(k); w != nil {
			w.Select(keys...)
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

func (u *Updater) submit(hs []Handle) (err error) {
	i := int8(1)
	for u.changed || u.operated {
		if err = u.data(hs); err != nil {
			return
		}
		if err = u.verify(hs); err != nil {
			return
		}
		u.emit(EventTypeSubmit)
		if i = i + 1; i >= 100 {
			return ErrSubmitEndlessLoop
		}
	}
	return
}

// Submit 按照MODEL的倒序执行
func (u *Updater) Submit() (r []*operator.Operator, err error) {
	if err = u.WriteAble(); err != nil {
		return nil, err
	}
	hs := u.Handles()
	if err = u.submit(hs); err != nil {
		return
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

// Handle 根据 name(string) || itype(int32)
func (u *Updater) Handle(name any) Handle {
	switch k := name.(type) {
	case string:
		return u.handles[k]
	case int:
		return u.handleByIType(int32(k))
	case int32:
		return u.handleByIType(int32(k))
	case int64:
		return u.handleByIType(int32(k))
	}
	return nil
}

// Handle 根据iid,oid获取模型不支持Hash,Document的字段查询
func (u *Updater) handle(k any) Handle {
	iid, err := u.ParseId(k)
	if err != nil {
		logger.Alert("%v", err)
		return nil
	}
	itk := Config.IType(iid)
	model, ok := modelsDict[itk]
	if !ok {
		logger.Alert("Updater.handle not exists,iid:%v IType:%v", k, itk)
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

// Create 创建一批新对象,仅仅适用于coll类型，不触发任何事件
func (u *Updater) Create(data dataset.Model) (err error) {
	op := &operator.Operator{OID: data.GetOID(), IID: data.GetIID(), Type: operator.TypesNew, Value: 1, Result: []any{data}}
	return u.Operator(op)
}

// Operator 直接插入，不触发任何事件
func (u *Updater) Operator(op *operator.Operator, before ...bool) error {
	iid := op.IID
	if iid <= 0 {
		return errors.New("operator iid empty")
	}
	handle := u.handle(iid)
	if handle == nil {
		return errors.New("handle unknown")
	}
	if op.Type == operator.TypesSet && handle.Parser() == ParserTypeCollection {
		if _, ok := op.Result.(dataset.Update); !ok {
			return errors.New("operator set result type must be dataset.Update")
		}
	}
	handle.Operator(op, before...)
	return nil
}

// Destroy 销毁用户实例,强制将缓存数据改变写入数据库,返回错误时无法写入数据库,应该排除问题后后再次尝试销毁
// 仅缓存模式下需要且必要执行
func (u *Updater) Destroy() (err error) {
	//u.emit(PlugsTypeDestroy)
	hs := u.Handles()
	for i := len(hs) - 1; i >= 0; i-- {
		if err = hs[i].destroy(); err != nil {
			return
		}
	}
	u.player = nil
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
