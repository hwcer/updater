package updater

import (
	"context"
	"fmt"
	"github.com/hwcer/cosgo/logger"
	"github.com/hwcer/cosgo/utils"
	"github.com/hwcer/updater/bson"
	"time"
)

type Updater struct {
	ctx     context.Context
	uid     string
	dict    map[string]Handle
	time    *utils.DateTime
	cache   []*Cache
	strict  bool //严格模式下使用sub会检查数量
	events  map[EventsType][]EventsHandle
	changed bool
	//overflow map[int32]int64 //道具溢出,需要使用邮件等其他方式处理
}

func New() (u *Updater) {
	u = &Updater{}
	u.dict = make(map[string]Handle)
	for _, model := range modelsRank {
		parser := model.Parser()
		u.dict[model.Name] = handles[parser](u, model.iModel)
	}
	u.Release()
	return
}

// Reset 重置
func (u *Updater) Reset(uid string, ctx context.Context) {
	if u.uid != "" {
		logger.Fatal("请不要重复调用Reset")
	}
	u.uid = uid
	if ctx == nil {
		ctx = context.Background()
	}
	u.ctx = context.WithValue(ctx, "", nil)
	u.time = utils.Time.New(time.Now())
	u.events = make(map[EventsType][]EventsHandle)
	//u.overflow = make(map[int32]int64)
	for _, w := range u.dict {
		w.reset()
	}
}

// Release 释放
func (u *Updater) Release() {
	u.uid = ""
	u.ctx = nil
	u.cache = nil
	u.strict = true
	u.events = nil
	u.changed = false
	for _, w := range u.dict {
		w.release()
	}
}

func (u *Updater) Emit(t EventsType) (err error) {
	for _, f := range u.events[t] {
		if err = f(u); err != nil {
			return
		}
	}
	return
}

func (u *Updater) On(t EventsType, f EventsHandle) {
	u.events[t] = append(u.events[t], f)
}

func (u *Updater) Uid() string {
	return u.uid
}
func (u *Updater) Time() *utils.DateTime {
	return u.time
}

// Strict true:检查sub, false: 不检查
func (u *Updater) Strict(b bool) {
	u.strict = b
}

func (u *Updater) Context() context.Context {
	return u.ctx
}

func (u *Updater) Get(id ikey) (r any) {
	if w := u.handle(id); w != nil {
		r = w.Get(id)
	}
	return
}

func (u *Updater) Set(id ikey, v any) {
	if w := u.handle(id); w != nil {
		w.Set(id, v)
	}
}

func (u *Updater) Add(iid ikey, num ival) {
	if w := u.handle(iid); w != nil {
		w.Add(iid, num)
	}
}

func (u *Updater) Sub(iid ikey, num ival) {
	if w := u.handle(iid); w != nil {
		w.Sub(iid, num)
	}
}

func (u *Updater) Max(iid ikey, num ival) {
	if w := u.handle(iid); w != nil {
		w.Max(iid, num)
	}
}

func (u *Updater) Min(iid ikey, num ival) {
	if w := u.handle(iid); w != nil {
		w.Min(iid, num)
	}
}

func (u *Updater) Val(id ikey) (r int64) {
	if w := u.handle(id); w != nil {
		r = w.Val(id)
	}
	return
}

func (u *Updater) Del(id ikey) {
	if w := u.handle(id); w != nil {
		w.Del(id)
	}
}

func (u *Updater) Bind(id string, i any) (err error) {
	if w := u.handle(id); w != nil {
		err = w.Bind(id, i)
	}
	return
}

func (u *Updater) Select(fields ...ikey) {
	for _, field := range fields {
		if w := u.handle(field); w != nil {
			w.Select(field)
		}
	}
}

func (u *Updater) Data() (err error) {
	if u.uid == "" {
		return
	}
	if err = u.Emit(EventsPreData); err != nil {
		return err
	}
	for _, w := range u.handles() {
		if err = w.Data(); err != nil {
			return
		}
	}
	u.changed = false
	return
}

func (u *Updater) Save() (err error) {
	if u.uid == "" {
		return
	}
	if u.changed {
		if err = u.Data(); err != nil {
			return
		}
	}
	if err = u.Emit(EventsPreVerify); err != nil {
		return err
	}
	hs := u.handles()
	for _, w := range hs {
		if err = w.Verify(); err != nil {
			return
		}
	}
	if err = u.Emit(EventsPreSubmit); err != nil {
		return err
	}
	var cache []*Cache
	for _, w := range hs {
		if cache, err = w.Save(); err != nil {
			return
		} else {
			u.cache = append(u.cache, cache...)
		}
	}
	return
}

func (u *Updater) Cache() (ret []*Cache) {
	return u.cache
}

// IType 通过ikey获取IType
func (u *Updater) IType(k ikey) (it IType) {
	if iid, err := u.ParseId(k); err == nil {
		id := Config.IType(iid)
		it = itypesDict[id]
	}
	return
}

func (u *Updater) ParseId(id ikey) (iid int32, err error) {
	if IsIID(id) {
		return bson.ParseInt32(id), nil
	}
	switch v := id.(type) {
	case string:
		iid, err = Config.ParseId(u, v)
	default:
		err = fmt.Errorf("parse key illegal:%v", id)
	}
	return
}

// CreateId 通过IID创建OID,不可堆叠道具，不可使用此接口
func (u *Updater) CreateId(k ikey) (oid string, err error) {
	if IsOID(k) {
		return k.(string), nil
	}
	iid := bson.ParseInt32(k)
	itk := Config.IType(iid)
	itv := itypesDict[itk]
	if itv == nil {
		return "", fmt.Errorf("IType unknown:%v", k)
	}
	if !itv.Unique() {
		return "", fmt.Errorf("IType is not unique:%v", k)
	}
	return itv.CreateId(u, iid)
}

// Handle 根据 name(string)
func (u *Updater) Handle(name string) Handle {
	return u.dict[name]
}

// Handle 根据iid,oid获取模型不支持Hash,Document的字段查询
func (u *Updater) handle(k ikey) Handle {
	iid, err := u.ParseId(k)
	if err != nil {
		logger.Warn("%v", err)
		return nil
	}
	itk := Config.IType(iid)
	model, ok := modelsDict[itk]
	if !ok {
		logger.Warn("Updater.handle not exists: %v", k)
	}
	return u.Handle(model.Name)
}

func (u *Updater) handles() (r []Handle) {
	for _, model := range modelsRank {
		r = append(r, u.dict[model.Name])
	}
	return
}
