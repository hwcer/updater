package updater

import (
	"reflect"
	"sync"

	"github.com/hwcer/logger"
	"github.com/hwcer/updater/dataset"
	"github.com/hwcer/updater/hamster"
	"github.com/hwcer/updater/operator"
)

// Updater 玩家数据更新器（核心版 hamster 之上的道具扩展层）。
//
// 🔴 **内嵌 *hamster.Store**：根包就是对核心包的封装，生命周期/错误状态/Cache/
// CreditAllowed/变更流水全部提升自核心（单字段，无镜像无同步）。
// 内嵌纪律：本类型**只加方法**，仅覆盖 Loading（Config 预检）与 Destroy（反查表清理）——
// 这两个 store 内部从不自调，无虚派发陷阱；其余一律提升。
//
// 本层新增的只有道具概念：IType 路由（modelsDict/itypesDict）、Add/Sub/Get/Val/Select
// 便捷 API、溢出分解（funcs.go）。
type Updater struct {
	*hamster.Store

	Events     Events      //生命周期事件（扩展层签名 Listener(*Updater)，遮蔽提升的同名字段）
	Middleware Middlewares //中间件（同上）

	mountViews sync.Map //挂载包装缓存：*hamster.Collection → *Mount（保证句柄指针同一）
}

// Entity 数据属主（取代 Player/Uid 用词 —— 核心版不绑玩家域）。
type Entity = hamster.Entity

// New 创建更新器。
func New(e Entity) *Updater {
	st := hamster.New(e)
	u := &Updater{Store: st, Events: Events{}, Middleware: Middlewares{}}
	st.SetEmitHook(func(_ *hamster.Store, t hamster.EventType) {
		u.emitRoot(EventType(t)) //扩展层事件按根包语义分发（Events 带错误闸门，Middleware 不带）
	})
	updaters.Store(st, u)
	return u
}

// emitRoot 根包事件分发（Events 带错误闸门，Middleware 不带 —— 拆分前 Emit 的原语义）
func (u *Updater) emitRoot(t EventType) {
	u.Events.emit(u, t)
	u.Middleware.emit(u, t)
}

func (u *Updater) On(t EventType, handle Listener) {
	u.Events.On(t, handle)
}

// loadGlobalCache 把 RegisterGlobalCache 注册的全局缓存载入实例
func (u *Updater) loadGlobalCache() {
	for k, v := range globalCache {
		if _, ok := u.Cache[k]; !ok {
			u.Cache[k] = v(u)
		}
	}
}

// Loading 重新加载数据,自动关闭异步数据。init 立即加载玩家所有数据。
// 🔴 开服自检：Config.BulkWrite 没配的话，**所有句柄的落库都会静默失效** ——
// 工厂变量是本包配置面，nil 判定必须以它为准（先于核心 Loading）。
func (u *Updater) Loading(cb ...func()) (err error) {
	if Config.BulkWrite == nil {
		return ErrBulkWriteNotInit
	}
	//全局缓存载入插在 cb 尾部：hamster.Loading 内部的顺序是
	//handles → 时钟 → cb → hamster全局缓存 → Emit(Init)，
	//追加的闭包在 Emit 之前执行，与「globalCache 先于 Init 事件」的时序一致。
	cb = append(cb, u.loadGlobalCache)
	return u.Store.Loading(cb...)
}

// Destroy 销毁用户实例,强制将缓存数据改变写入数据库。
// 覆盖提升方法只为清理 store→Updater 反查表（模型回调需要 *Updater）。
func (u *Updater) Destroy() error {
	err := u.Store.Destroy()
	updaters.Delete(u.Store)
	return err
}

// Add 添加道具,num 支持 int32|int64
func (u *Updater) Add(iid int32, num any) {
	u.itemChange(operator.TypesAdd, iid, num)
}

// Sub 扣除道具,num 支持 int32|int64
func (u *Updater) Sub(iid int32, num any) {
	u.itemChange(operator.TypesSub, iid, num)
}

// itemChange 顶层道具增减：路由到核心句柄，把 IID 换算成句柄的 key 形态
//（Values 数值键直用；Document/Virtual 换算字段名；Collection 直传数值键，
// 由核心经模型的 OIDMaker/Stacker 完成 OID 换算、按件生成与溢出控制）。
func (u *Updater) itemChange(t operator.Types, iid int32, num any) {
	w := u.handleWithKey(iid)
	if w == nil {
		return
	}
	add := t == operator.TypesAdd
	switch h := w.(type) {
	case *hamster.Values:
		if add {
			h.Add(iid, num)
		} else {
			h.Sub(iid, num)
		}
	case *hamster.Document:
		//IID → 字段名（模型 DocumentModel.Field）
		mod, _ := modelsDict[u.itemModelKey(iid)]
		if dm, ok := mod.model.(DocumentModel); ok {
			if field, err := dm.Field(u, iid); err == nil {
				if add {
					h.Add(field, num)
				} else {
					h.Sub(field, num)
				}
			}
		}
	case *hamster.Collection:
		//数值键直传：核心经模型的 Stacker/OIDMaker 完成换算、按件生成与溢出控制
		if add {
			h.Add(iid, h.Field(), num)
		} else {
			h.Sub(iid, h.Field(), num)
		}
	case *hamster.Virtual:
		//IID → 委托键（模型 VirtualModel.Field）
		mod, _ := modelsDict[u.itemModelKey(iid)]
		if vm, ok := mod.model.(VirtualModel); ok {
			if field, ok := vm.Field(iid); ok {
				if add {
					h.Add(field, num)
				} else {
					h.Sub(field, num)
				}
			}
		}
	}
}

// itemModelKey 查 iid 所属模型的注册名
func (u *Updater) itemModelKey(iid int32) int32 {
	return Config.IType(iid)
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
		return u.Handle(k)
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
	return u.Handle(mod.name)
}

// 类型访问器：参数放宽为 any（string 表名或命名整型 IType），返回**核心句柄**。
// 道具语义（iid 解析、IType、溢出）已由适配器注入这些句柄 —— 根包没有第二套句柄类型。
func (u *Updater) Values(name any) *hamster.Values {
	i := u.handleWithAny(name)
	if i == nil {
		return nil
	}
	r, _ := i.(*hamster.Values)
	return r
}
func (u *Updater) Virtual(name any) *hamster.Virtual {
	i := u.handleWithAny(name)
	if i == nil {
		return nil
	}
	r, _ := i.(*hamster.Virtual)
	return r
}

// Document 遮蔽提升的同名方法（参数放宽为 any）
func (u *Updater) Document(name any) *hamster.Document {
	i := u.handleWithAny(name)
	if i == nil {
		return nil
	}
	r, _ := i.(*hamster.Document)
	return r
}

// Collection 遮蔽提升的同名方法（参数放宽为 any）
func (u *Updater) Collection(name any) *hamster.Collection {
	i := u.handleWithAny(name)
	if i == nil {
		return nil
	}
	r, _ := i.(*hamster.Collection)
	return r
}
