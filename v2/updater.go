package updater

import (
	"context"
	"fmt"
	"github.com/hwcer/cosgo/logger"
	"github.com/hwcer/cosgo/utils"
	"github.com/hwcer/updater/v2/operator"
	"time"
)

type Updater struct {
	ctx       context.Context
	uid       string
	dict      map[string]Handle
	time      *utils.DateTime
	plugs     map[plugsType][]plugsHandle
	changed   bool
	emitter   emitter
	tolerance bool //宽容模式下,扣除道具不足时允许扣成0,而不是报错
}

func New(uid string) (u *Updater) {
	u = &Updater{uid: uid}
	u.dict = make(map[string]Handle)
	for _, model := range modelsRank {
		u.dict[model.name] = handles[model.parser](u, model.model, model.ram)
	}
	u.Release()
	return
}

// Reset 重置
func (u *Updater) Reset(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	u.ctx = context.WithValue(ctx, "", nil)
	u.time = utils.Time.New(time.Now())
	//u.plugs = map[plugsType][]plugsHandle{}
	//u.overflow = make(map[int32]int64)
	for _, w := range u.dict {
		w.reset()
	}
}

// Release 释放
func (u *Updater) Release() {
	u.ctx = nil
	u.plugs = nil
	u.changed = false
	u.tolerance = false
	for _, w := range u.dict {
		w.release()
	}
}

func (u *Updater) Uid() string {
	return u.uid
}
func (u *Updater) Time() *utils.DateTime {
	return u.time
}

// Tolerance 开启宽容模式,道具不足时扣成0
func (u *Updater) Tolerance() {
	u.tolerance = true
}

func (u *Updater) Context() context.Context {
	return u.ctx
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

func (u *Updater) Max(iid int32, num int32) {
	if w := u.handle(iid); w != nil {
		w.Max(iid, num)
	}
}

func (u *Updater) Min(iid int32, num int32) {
	if w := u.handle(iid); w != nil {
		w.Min(iid, num)
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
	if u.uid == "" {
		return ErrUidEmpty
	}
	defer func() {
		if err == nil {
			u.changed = false
		}
	}()
	if err = u.doPlugs(PlugsTypePreData); err != nil {
		return err
	}
	for _, w := range u.handles() {
		if err = w.Data(); err != nil {
			return
		}
	}
	return
}

func (u *Updater) Save() (err error) {
	if u.uid == "" {
		return ErrUidEmpty
	}
	if u.changed {
		if err = u.Data(); err != nil {
			return
		}
	}
	if err = u.doPlugs(PlugsTypePreVerify); err != nil {
		return err
	}
	hs := u.handles()
	for _, w := range hs {
		if err = w.Verify(); err != nil {
			return
		}
	}
	if err = u.doPlugs(PlugsTypePreSave); err != nil {
		return err
	}
	for _, w := range hs {
		if err = w.Save(); err != nil {
			return
		}
	}
	if err = u.doPlugs(PlugsTypePreSubmit); err != nil {
		return err
	}
	return
}

func (u *Updater) Submit() (r []*operator.Operator) {
	for _, w := range u.handles() {
		r = append(r, w.submit()...)
	}
	u.doEvents()
	return r
}

// IType 通过ikey获取IType
func (u *Updater) IType(k any) (it IType) {
	if iid, err := u.ParseId(k); err == nil {
		id := Config.IType(iid)
		it = itypesDict[id]
	}
	return
}

func (u *Updater) ParseId(key any) (iid int32, err error) {
	if v, ok := key.(string); ok {
		iid, err = Config.ParseId(u, v)
	} else {
		iid = ParseInt32(key)
	}
	return
}

// CreateId 通过IID创建OID,不可堆叠道具，不可使用此接口
func (u *Updater) CreateId(key any) (oid string, err error) {
	if v, ok := key.(string); ok {
		return v, nil
	}
	iid := ParseInt32(key)
	itk := Config.IType(iid)
	itv := itypesDict[itk]
	if itv == nil {
		return "", fmt.Errorf("IType unknown:%v", iid)
	}
	if !itv.Unique() {
		return "", fmt.Errorf("item IType not unique:%v", iid)
	}
	return itv.CreateId(u, iid)
}

// ObjectId 根据id(iid,oid)同时获取iid,oid/key
func (u *Updater) ObjectId(id any) (oid string, iid int32, err error) {
	switch v := id.(type) {
	case string:
		oid = v
		iid, err = u.ParseId(oid)
	default:
		iid = ParseInt32(id)
		oid, err = u.CreateId(iid)
	}
	return
}

// Handle 根据 name(string)
func (u *Updater) Handle(name string) Handle {
	return u.dict[name]
}

// Handle 根据iid,oid获取模型不支持Hash,Document的字段查询
func (u *Updater) handle(k any) Handle {
	iid, err := u.ParseId(k)
	if err != nil {
		logger.Warn("%v", err)
		return nil
	}
	itk := Config.IType(iid)
	model, ok := modelsDict[itk]
	if !ok {
		logger.Warn("Updater.handle not exists: %v", k)
		return nil
	}
	return u.Handle(model.name)
}

func (u *Updater) handles() (r []Handle) {
	for _, model := range modelsRank {
		r = append(r, u.dict[model.name])
	}
	return
}
