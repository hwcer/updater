package updater

import (
	"context"
	"fmt"
	"github.com/hwcer/cosgo/logger"
	"github.com/hwcer/updater/v2/operator"
	"time"
)

type Updater struct {
	ctx      context.Context
	uid      string
	Time     time.Time
	Error    error
	changed  bool
	emitter  emitter
	process  updaterProcess
	handles  map[string]Handle
	operator []*operator.Operator //临时操作,不涉及数据,直接返回给客户端,此类消息无视错误,直至成功
	tolerate bool                 //宽容模式下,扣除道具不足时允许扣成0,而不是报错
}

func New(uid string) (u *Updater) {
	u = &Updater{uid: uid}
	u.handles = make(map[string]Handle)
	for _, model := range modelsRank {
		u.handles[model.name] = handles[model.parser](u, model.model, model.ram)
	}
	return
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

// Reset 重置
func (u *Updater) Reset(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	u.ctx = context.WithValue(ctx, "", nil)
	u.Time = time.Now()
	for _, w := range u.handles {
		w.reset()
	}
}

// Release 释放
func (u *Updater) Release() {
	u.ctx = nil
	u.Error = nil
	u.changed = false
	u.tolerate = false
	u.process.release()
	for _, w := range u.handles {
		w.release()
	}
}

// Init 构造函数 NEW之后立即调用
func (u *Updater) Init() (err error) {
	u.Time = time.Now()
	for _, w := range u.handles {
		if err = w.init(); err != nil {
			return
		}
	}
	return u.process.emit(u, ProcessTypeInit)
}

// Flush 强制将缓存数据改变写入数据库,返回错误时无法写入数据库,应该排除问题后后再次尝试关闭
func (u *Updater) Flush() (err error) {
	for _, w := range u.handles {
		if err = w.flush(); err != nil {
			return
		}
	}
	return
}

func (u *Updater) Uid() string {
	return u.uid
}

// Tolerate 开启包容模式,道具不足时扣成0,仅生效一次
func (u *Updater) Tolerate() {
	u.tolerate = true
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
	if u.Error != nil {
		return u.Error
	}
	defer func() {
		u.changed = false
	}()
	if err = u.process.emit(u, ProcessTypePreData); err != nil {
		return
	}
	for _, w := range u.Handles() {
		if err = w.Data(); err != nil {
			return
		}
	}
	return
}

func (u *Updater) Save() (err error) {
	if u.Error != nil {
		return u.Error
	}
	if u.changed {
		if err = u.Data(); err != nil {
			return
		}
	}
	if err = u.process.emit(u, ProcessTypePreVerify); err != nil {
		return
	}
	hs := u.Handles()
	for _, w := range hs {
		if err = w.Verify(); err != nil {
			return
		}
	}
	if err = u.process.emit(u, ProcessTypePreSave); err != nil {
		return
	}
	for _, w := range hs {
		if err = w.Save(); err != nil {
			return
		}
	}
	if err = u.process.emit(u, ProcessTypePreSubmit); err != nil {
		return
	}
	u.doEvents()
	return
}

func (u *Updater) Submit() (r []*operator.Operator) {
	if u.Error != nil {
		return
	}
	if len(u.operator) > 0 {
		r = append(r, u.operator...)
		u.operator = nil
	}
	for _, w := range u.Handles() {
		r = append(r, w.submit()...)
	}
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
	return u.handles[name]
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

func (u *Updater) Handles() (r []Handle) {
	r = make([]Handle, 0, len(modelsRank))
	for _, model := range modelsRank {
		r = append(r, u.handles[model.name])
	}
	return
}

// Operator 生成一次操作结果,返回给客户端,不会修改数据
func (u *Updater) Operator(t operator.Types, i int32, v any, r any) *operator.Operator {
	op := operator.New(t, v)
	op.IID = i
	op.Result = r
	u.operator = append(u.operator, op)
	return op
}
