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
// 拆分后实现整体下沉 hamster（hamster.Collection 就是本 Mount 的泛化 ——
// "无 IType Collection"的全尺度原型），根包保留 Mount 类型作为 API 冻结面的薄包装，
// 方法逐个显式委托。设计取舍见 HANDLER_MOUNT_PLAN.md 与 HAMSTER_PLAN.md。

// MountModel 临时数据模型。
//
// 组合 schema.Tabler 是必需的：挂载名取自 TableName()，而 CollectionModel 本身不含它 ——
// 只声明 CollectionModel 的话就得在 Mount 内部做运行时类型断言，"传错模型编不过"这句就不成立。
//
// ⚠️ 取名规则与 Register 并不一致，这是有意的：Register 对非 schema.Tabler 的模型有
// schema.Kind(model).Name() 兜底，Mount 没有兜底路径。
//
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
//
// 带 keys 时等价于 Select(keys...) + Data()，**当场查库**（不等框架的 Data 阶段）。
// 已在内存里的 key 会被 Select 跳过，所以长命句柄反复这么调不会重复查库。
//
// 挂载名取 model.TableName()，与已注册的全局模型重名时报错：撞名的话同一张表在一个
// Updater 里会有两个句柄各写各的，是静默的数据竞争。
//
// ⚠️ 不带 keys 时**不做任何预加载**。
// ⚠️ **挂载与取数是两码事**：查库失败时返回 (句柄, err) —— 句柄已经挂上且完全可用，
// 唯一会返回 nil 的是重名，那时压根没挂上。
func (u *Updater) Mount(model MountModel, keys ...string) (*Mount, error) {
	//重名检查双保险：扩展层路由表（modelsDict）在这里查，核心版注册表（hamster.modelsRank）
	//由 store.Mount 查 —— 两层各自盯住自己那一份，撞名都报错而不是静默数据竞争。
	name := model.TableName()
	for _, mod := range modelsDict {
		if mod != nil && mod.name == name {
			return nil, Errorf(0, "mount name conflicts with registered model:%v", name)
		}
	}
	u.syncErrIn()
	defer u.syncErrOut()
	c, err := u.store.Mount(&mountAdapter{m: model, u: u}, keys...)
	if c == nil {
		return nil, err //唯一会返回 nil 的是重名：压根没挂上
	}
	r := u.mountOf(c)
	// 下发客户端与否由模型有没有声明 ModelIType 决定，没有开关：
	// IType(0) 非 0 才给 operator 盖分发键并接进 u.dirty（通用更新通道），
	// 否则保持核心版默认的 Discard。幂等：重复 Mount 走到这里是重复盖同一个键。
	// ⚠️ 判据是"IType(0) 返回非 0"，不是"实现了 ModelIType" —— 项目侧模型基类往往自带
	// IType(iid) 转发全局配置，对 iid=0 通常返回 0；想让挂载表走通用通道必须显式覆盖。
	if m, ok := model.(ModelIType); ok {
		if it := m.IType(0); it != 0 {
			c.SetIType(it)
			c.Receiver(func(_ *hamster.Store, ops []*operator.Operator) { u.pushDirty(nil, ops) })
		}
	}
	return r, err //查库失败时句柄已挂上且可用，连句柄一起返回（挂载与取数是两码事）
}

// mountOf 取挂载包装句柄：**同一底层集合恒返回同一 *Mount 指针**（幂等语义的组成部分，
// 业务拿它做句柄比较/长命缓存）。包装对象存在 hamster.Collection 的扩展槽里。
func (u *Updater) mountOf(c *hamster.Collection) *Mount {
	if m, ok := c.Ext().(*Mount); ok {
		return m
	}
	m := &Mount{coll: c, updater: u}
	c.SetExt(m)
	return m
}

// Mounted 取回已挂载的临时集合，未挂载返回 nil。它**只取不挂**，也不取数。
func (u *Updater) Mounted(model MountModel) *Mount {
	c := u.store.Mounted(&mountAdapter{m: model, u: u})
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
	u.store.Unmount(&mountAdapter{m: model, u: u})
}

// Mounts 挂载表（只读视图，键为挂载名）。Destroy 后为空。
func (u *Updater) Mounts() map[string]*hamster.Collection {
	return u.store.Mounts()
}

// Mount 挂载集合句柄：与玩家数据同批次原子写库的薄包装。
// 🔴 全部方法显式委托 hamster.Collection —— 不内嵌（方法提升没有虚派发）。
type Mount struct {
	coll    *hamster.Collection
	updater *Updater
}

// ===================== 操作入口（全部产 operator，不直接改内存）=====================

// Update 批量改字段。只是入队，verify 阶段才真正写进内存。
func (this *Mount) Update(id string, data dataset.Update) *operator.Operator {
	this.updater.syncErrIn()
	defer this.updater.syncErrOut() //format 失败会把错误落在 store 上，镜像回根包
	return this.coll.Update(id, data)
}

// Set 改单个字段，语义同 Update。
func (this *Mount) Set(id string, field string, value any) *operator.Operator {
	return this.coll.Update(id, dataset.NewUpdate(field, value))
}

// Unset 删字段。
func (this *Mount) Unset(id string, fields ...string) *operator.Operator {
	this.updater.syncErrIn()
	defer this.updater.syncErrOut()
	return this.coll.Unset(id, fields...)
}

// Delete 删文档。
func (this *Mount) Delete(id string) *operator.Operator {
	this.updater.syncErrIn()
	defer this.updater.syncErrOut()
	return this.coll.Delete(id)
}

// Insert 插入新文档，_id 从对象上取，取不到即报错。
//
// ⚠️ **不要另给一个 id 参数**：真正决定落库主键的是对象自己的 _id。
func (this *Mount) Insert(v any) *operator.Operator {
	this.updater.syncErrIn()
	defer this.updater.syncErrOut()
	return this.coll.Insert(v)
}

// Operators 本次请求**已通过 verify、尚未 submit** 的 operator 列表（只读）。
// 🔴 读的就是 statement.cache：verify 之后、submit 之前有效。
func (this *Mount) Operators() []*operator.Operator {
	return this.coll.Operators()
}

// Submit 把**本挂载**的改动单独落库，不等 Updater 整体提交。
//
// 给「这一趟末尾要 return error、但这份数据必须留下」的场合用。
// 🔴 用一份**独立的 BulkWrite**，不碰 Updater 那份共享实例。
//
// ⚠️ 失败时内存已经是新值、库还是旧的：要么重试，要么 Unmount 整张表。
// ⚠️ 全局的 StatusOperated **不会**被清除：它是所有 handle 共用的标志。
func (this *Mount) Submit() error {
	this.updater.syncErrIn()
	defer this.updater.syncErrOut()
	return this.coll.Submit()
}

// Receive 把**已经在手上的**文档直接塞进内存，跳过 Select + Data 那次查库。
// ⚠️ 只进内存、**不记脏、不会被写库**；塞进来之后 Select 会跳过这条。
func (this *Mount) Receive(id string, data any) {
	this.coll.Receive(id, data)
}

// ===================== 读取 =====================

func (this *Mount) Name() string {
	return this.coll.Name()
}

func (this *Mount) Schema() *schema.Schema {
	return this.coll.Schema()
}

// Field 解析数值字段名：传参优先，其次模型实现的 GetValueJSName()，最后 dataset.Fields.VAL。
func (this *Mount) Field(field ...string) string {
	return this.coll.Field(field...)
}

// Document 取文档，不存在返回 nil。
func (this *Mount) Document(id string) *dataset.Document {
	return this.coll.Document(id)
}

func (this *Mount) Has(id string) bool {
	return this.coll.Has(id)
}

func (this *Mount) Len() int {
	return this.coll.Len()
}

func (this *Mount) Range(handle func(string, *dataset.Document) bool) {
	this.coll.Range(handle)
}

// Remove 仅从内存移除，不动数据库。**submit 落库之后才真正摘除**。
func (this *Mount) Remove(id ...string) {
	this.coll.Remove(id...)
}

// ===================== Handle 接口 =====================

// Get 取文档原始对象，key 必须是文档 _id(string)。
// 🔴 **拿到的是指针，只读**。要改数据一律走 Update / Set。
func (this *Mount) Get(key any) (r any) {
	return this.coll.Get(key)
}

// Val 取文档上数值字段的值，字段名由 Field() 决定。
func (this *Mount) Val(key any) (r int64) {
	return this.coll.Val(key)
}

// Data 拉取 Select 标记的文档。keys 为空时不查库。
func (this *Mount) Data() (err error) {
	this.updater.syncErrIn()
	defer this.updater.syncErrOut()
	return this.coll.Data()
}

// Count 统计 iid 匹配的文档数，iid 传 0 统计全部。
// ⚠️ **这是不完全统计**：挂载按 key 惰性加载，它数的是内存不是库。
func (this *Mount) Count(iid int32) int64 {
	var n int64
	this.coll.Range(func(id string, doc *dataset.Document) bool {
		if iid == 0 || docIID(doc) == iid {
			n++
		}
		return true
	})
	return n
}

// Select 标记待拉取的 _id，随后由 Updater.Data() 统一查库。已在内存中的 key 直接跳过。
func (this *Mount) Select(keys ...any) {
	this.coll.Select(keys...)
}

// Parser 挂载不在全局注册表里，这个返回值没有任何消费者；形态上最接近 Collection，就报它。
func (this *Mount) Parser() Parser {
	return ParserTypeCollection
}

// ===================== Handle 接口生命周期方法 =====================
// Updater.Handles() 收录挂载句柄，全流程驱动；包装只做转发。

func (this *Mount) Save() error      { return this.coll.Save() }
func (this *Mount) Reset()           { this.coll.Reset() }
func (this *Mount) Reload() error    { return this.coll.Reload() }
func (this *Mount) Loading() error   { return this.coll.Loading() }
func (this *Mount) Release()         { this.coll.Release() }
func (this *Mount) Destroy() error   { return this.coll.Destroy() }
func (this *Mount) Commit() error    { return this.coll.Commit() }
func (this *Mount) Verify() error    { return this.coll.Verify() }
