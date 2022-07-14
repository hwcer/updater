package updater

import (
	"github.com/hwcer/cosgo/logger"
	"github.com/hwcer/cosgo/utils"
	"time"
)

type Updater struct {
	uid      string
	dict     map[string]Handle
	time     *utils.DateTime
	cache    []*Cache
	strict   bool //严格模式下使用sub会检查数量
	changed  bool
	events   map[EventsType][]EventsHandle
	overflow map[int32]int64 //道具溢出,需要使用邮件等其他方式处理
	Flags    flags
}

func New() (u *Updater) {
	u = &Updater{}
	u.dict = make(map[string]Handle, len(modelsDict))
	for _, model := range modelsRank {
		if model.Parse == ParseTypeHash {
			u.dict[model.Name] = NewHash(model, u)
		} else if model.Parse == ParseTypeTable {
			u.dict[model.Name] = NewTable(model, u)
		}
	}
	u.release()
	return
}

//Reset 重置
func (u *Updater) Reset(uid string) {
	if u.uid != "" {
		logger.Fatal("请不要重复调用Reset")
	}
	u.uid = uid
	u.time = utils.Time.New(time.Now())
	u.events = make(map[EventsType][]EventsHandle)
	u.Flags = flags{}
}

//Release 释放
func (u *Updater) Release() {
	u.uid = ""
	u.release()
}

func (u *Updater) release() {
	u.cache = nil
	u.changed = false
	u.overflow = make(map[int32]int64)
	u.strict = true
	u.events = nil
	u.Flags = nil
	for _, w := range u.dict {
		w.release()
	}
}

func (u *Updater) emit(t EventsType) (err error) {
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

//Time 获取Updater启动时间
func (u *Updater) Time() time.Time {
	return u.time.Time()
}
func (u *Updater) Unix() int64 {
	return u.time.Time().Unix()
}

//Strict true:检查sub, false: 不检查
func (u *Updater) Strict(b bool) {
	u.strict = b
}

func (u *Updater) Get(id interface{}) (r interface{}, ok bool) {
	if w := u.getModuleType(id); w != nil {
		r, ok = w.Get(id)
	}
	return
}

func (u *Updater) Set(id interface{}, v interface{}) {
	if w := u.getModuleType(id); w != nil {
		w.Set(id, v)
	}
}

func (u *Updater) Add(iid int32, num int32) {
	if iid == 0 || num <= 0 {
		return
	}
	if w := u.getModuleType(iid); w != nil {
		w.Add(iid, num)
	}
}

func (u *Updater) Sub(iid int32, num int32) {
	if iid == 0 || num <= 0 {
		return
	}
	if w := u.getModuleType(iid); w != nil {
		w.Sub(iid, num)
	}
}

func (u *Updater) Max(iid int32, num int32) {
	if iid == 0 {
		return
	}
	if w := u.getModuleType(iid); w != nil {
		w.Max(iid, num)
	}
}

func (u *Updater) Min(iid int32, num int32) {
	if iid == 0 {
		return
	}
	if w := u.getModuleType(iid); w != nil {
		w.Min(iid, num)
	}
}

func (u *Updater) Del(id interface{}) {
	if w := u.getModuleType(id); w != nil {
		w.Del(id)
	}
}

func (u *Updater) Val(id interface{}) (r int64) {
	if w := u.getModuleType(id); w != nil {
		r = w.Val(id)
	}
	return
}

//Keys 通过iid或者oid添加需要获取的道具信息
func (u *Updater) Keys(ids ...interface{}) {
	for _, id := range ids {
		if w := u.getModuleType(id); w != nil {
			w.Keys(id)
		}
	}
}

func (u *Updater) Data() (err error) {
	if u.uid == "" {
		return
	}
	if err = u.emit(EventsTypeBeforeData); err != nil {
		return err
	}
	for _, w := range u.handles() {
		if err = w.Data(); err != nil {
			return
		}
	}
	u.changed = false
	if err = u.emit(EventsTypeFinishData); err != nil {
		return err
	}
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
	if err = u.emit(EventsTypeBeforeVerify); err != nil {
		return err
	}
	ws := u.handles()
	for _, w := range ws {
		if err = w.Verify(); err != nil {
			return
		}
	}

	if err = u.emit(EventsTypeFinishVerify); err != nil {
		return err
	}
	if err = u.emit(EventsTypeBeforeSave); err != nil {
		return err
	}
	var cache []*Cache
	for _, w := range ws {
		if cache, err = w.Save(); err != nil {
			return
		} else {
			u.cache = append(u.cache, cache...)
		}
	}
	if err = u.emit(EventsTypeFinishSave); err != nil {
		return err
	}
	return
}

func (u *Updater) Cache() (ret []*Cache) {
	return u.cache
}

func (u *Updater) getModuleType(id interface{}) Handle {
	var iid int32
	switch id.(type) {
	case string:
		iid, _ = Config.ParseId(id.(string))
	default:
		iid, _ = ParseInt32(id)
	}
	if iid == 0 {
		logger.Warn("Updater.getModuleType id illegal: %v", id)
	}
	it := Config.IType(iid)
	if it == nil {
		logger.Warn("Updater.getModuleType IType not exists: %v", iid)
		return nil
	}
	w, ok := u.dict[it.Model()]
	if !ok {
		logger.Warn("Updater.getModuleType handles not exists: %v", it.Model)
	}
	return w
}

func (u *Updater) Handle(name string) Handle {
	return u.dict[name]
}

func (u *Updater) handles() (r []Handle) {
	for _, model := range modelsRank {
		if w, ok := u.dict[model.Name]; ok {
			r = append(r, w)
		}
	}
	return
}
