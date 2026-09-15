package updater

import (
	"github.com/hwcer/cosgo/schema"
	"github.com/hwcer/updater/dataset"
	"github.com/hwcer/updater/hamster"
	"github.com/hwcer/updater/operator"
)

// 临时句柄挂载。
//
// 为"Updater 之外、但要与玩家数据同批次原子写库"的数据准备：邮件领取标记、兑换码占用、
// 充值订单、临时战斗副本。共同点是要同批次原子写库 + 按需查库 + 可选内存驻留，
// **不进 IType 路由、不自动生成给客户端的 operator**。
//
// 核心实现是 hamster.Mount（独立封装，不内嵌 hamster.Collection —— 方法提升没有虚派发，
// 内嵌复用反复踩"覆盖了却不生效"的坑，主干三稿教训）；根包 Mount **内嵌** *hamster.Mount
// —— 全部方法提升，零委托胶水。内嵌纪律：只加方法，不覆盖任何核心内部会自调的行为。
// 设计取舍见 HANDLER_MOUNT_PLAN.md 与 HAMSTER_PLAN.md。

// MountModel 临时数据模型。
//
// 组合 schema.Tabler 是必需的：挂载名取自 TableName()，而 CollectionModel 本身不含它。
// ⚠️ Getter 只会收到**非空**的 keys —— 挂载是按需加载的，从不做全量拉取。
type MountModel interface {
	CollectionModel
	schema.Tabler
}

// mountAdapter 把扩展层 MountModel（首参 *Updater）适配成 hamster.MountModel（首参 *hamster.Store）
type mountAdapter struct {
	m MountModel
	u *Updater
}

func (a *mountAdapter) Upsert(_ *hamster.Store, op *operator.Operator) bool {
	return a.m.Upsert(a.u, op)
}
func (a *mountAdapter) Schema() *schema.Schema { return a.m.Schema() }
func (a *mountAdapter) Getter(_ *hamster.Store, data *dataset.Collection, keys []string) error {
	return a.m.Getter(a.u, data, keys)
}
func (a *mountAdapter) Setter(_ *hamster.Store, bw BulkWrite, _id string, dirty dataset.Update, unset []string) error {
	return a.m.Setter(a.u, bw, _id, dirty, unset)
}
func (a *mountAdapter) TableName() string { return a.m.TableName() }

// GetValueJSName 数值字段名（核心 Mount.Field 的回落链会用到），与 collAdapter 同款转发
func (a *mountAdapter) GetValueJSName() string {
	if f, ok := a.m.(CollectionModelValueJSName); ok {
		return f.GetValueJSName()
	}
	return ""
}

// Mount 挂载/取回一个临时数据集合，keys 非空时顺带把这几条**当场查出来**。
//
// 幂等：同模型重复 Mount 直接返回已挂句柄 —— 长命场景（战斗副本）的每个 handler 开头
// 都是这一行，首个请求创建、后续全是复用，业务不必自己记"挂没挂过"。
//
//	coll, err := u.Mount(&model.Battle{}, battleId) //挂载 + 取数，一行搞定
//	coll, err := u.Mount(&model.Mail{}, ids...)     //多条一起
//	coll, err := u.Mount(&model.Battle{})           //只挂载，稍后自己 Select + Data
//
// keys 是文档 _id（string）—— 临时集合不进 IType 路由，没有 iid 这个概念。
// 带 keys 时等价于 Select(keys...) + Data()，**当场查库**；已在内存里的 key 会被跳过。
//
// 挂载名取 model.TableName()，与已注册的全局模型重名时报错。
// ⚠️ 查库失败时返回 (句柄, err) —— 句柄已经挂上且完全可用；唯一返回 nil 的是重名。
func (u *Updater) Mount(model MountModel, keys ...string) (*Mount, error) {
	//重名检查双保险：扩展层路由表（modelsDict）在这里查，核心版注册表（hamster.modelsRank）
	//由 store.Mount 查 —— 两层各自盯住自己那一份，撞名都报错而不是静默数据竞争。
	name := model.TableName()
	for _, mod := range modelsDict {
		if mod != nil && mod.name == name {
			return nil, Errorf(0, "mount name conflicts with registered model:%v", name)
		}
	}
	c, err := u.Store.Mount(&mountAdapter{m: model, u: u}, keys...)
	if c == nil {
		return nil, err //唯一会返回 nil 的是重名：压根没挂上
	}
	r := u.mountOf(c)
	// 下发客户端与否由模型有没有声明 ModelIType 决定，没有开关：
	// IType(0) 非 0 才在提交时给 operator 盖分发键并接进变更流水（通用更新通道），
	// 否则保持核心版默认的 Discard。盖键在 Receiver 里做（核心 ops 恒 IType=0）。
	// ⚠️ 判据是"IType(0) 返回非 0"，不是"实现了 ModelIType" —— 项目侧模型基类往往自带
	// IType(iid) 转发全局配置，对 iid=0 通常返回 0；想让挂载表走通用通道必须显式覆盖。
	if m, ok := model.(ModelIType); ok {
		if it := m.IType(0); it != 0 {
			c.Receiver(func(_ *hamster.Store, ops []*operator.Operator) {
				for _, o := range ops {
					o.IType = it
				}
				u.Dirty(ops...)
			})
		}
	}
	return r, err //查库失败时句柄已挂上且可用，连句柄一起返回（挂载与取数是两码事）
}

// mountOf 取挂载包装句柄：**同一底层集合恒返回同一 *Mount 指针**（幂等语义的组成部分，
// 业务拿它做句柄比较/长命缓存）。包装缓存放在根包（核心保持干净）。
func (u *Updater) mountOf(c *hamster.Mount) *Mount {
	if v, ok := u.mountViews.Load(c); ok {
		return v.(*Mount)
	}
	m := &Mount{Mount: c}
	v, _ := u.mountViews.LoadOrStore(c, m)
	return v.(*Mount)
}

// Mounted 取回已挂载的临时集合，未挂载返回 nil。它**只取不挂**，也不取数。
func (u *Updater) Mounted(model MountModel) *Mount {
	c := u.Store.Mounted(&mountAdapter{m: model, u: u})
	if c == nil {
		return nil
	}
	return u.mountOf(c)
}

// Unmount 标记卸载。**只打标记，真正摘除在 Release 阶段**（EventTypeRelease 之后）。
//
// 🔴 不在这里直接刷盘/摘除，是为了让短流程也走完整生命周期。短命场景的标准写法是
//
//	coll, err := u.Mount(&model.Mail{}, ids...)
//	defer u.Unmount(&model.Mail{})
//
// 打完标记后句柄照旧留在挂载表里，正常参与 Data / verify / submit，直到请求结束才被摘掉。
// ⚠️ 卸载粒度是**整张表**：只有最后一个实例结束时才 Unmount；判断不了就别卸，留给下线兜底。
func (u *Updater) Unmount(model MountModel) {
	u.Store.Unmount(&mountAdapter{m: model, u: u})
}

// Mounts 挂载表（只读视图，键为挂载名）。Destroy 后为空。
func (u *Updater) Mounts() map[string]*hamster.Mount {
	return u.Store.Mounts()
}

// Mount 挂载集合句柄：**内嵌 hamster.Mount**（核心独立封装，不内嵌 Collection，
// 语义见 hamster/mount.go）。Get/Val/Data/Select/Set/Update/Unset/Delete/Insert/
// Submit/Receive/Remove/Operators/Count/Document/Range 等全部方法提升自核心版。
type Mount struct {
	*hamster.Mount
}
