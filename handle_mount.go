package updater

import (
	"github.com/hwcer/cosgo/schema"
	"github.com/hwcer/logger"
	"github.com/hwcer/updater/dataset"
)

// MountModel 临时数据模型。
//
// 组合 schema.Tabler 是必需的：挂载名取自 TableName()，而 CollectionModel 本身不含它 ——
// 只声明 CollectionModel 的话就得在 Mount 内部做运行时类型断言，"传错模型编不过"这句就不成立。
//
// ⚠️ 取名规则与 Register 并不一致，这是有意的：Register 对非 schema.Tabler 的模型有
// schema.Kind(model).Name() 兜底，Mount 没有兜底路径。
type MountModel interface {
	CollectionModel
	schema.Tabler
}

// MountCollection 临时数据集合：与玩家数据同批次原子写库，但不进 IType/operator 流水线。
//
// 复用 dataset.Collection（纯容器：内存 map + dirty 跟踪 + Save(CollectionWriter)），
// 不复用 statement（算子流水线）—— 临时数据要的是前者，不需要后者一条。
//
// 与通用 Collection 的区别，用之前先看清楚：
//   - 不产 operator，不下发客户端。要客户端感知自行 u.Dirty(operator.New(...))；
//   - 不参与 verify（无溢出检查、无扣量检查、无自动分解）；
//   - 通用入口 u.Add/u.Sub/u.Set/u.Select **一律路由不到这里** ——
//     那几个走 Config.IType 查 modelsDict，临时模型压根不在里面。只能拿本对象操作；
//   - 🔴 **key 只能是文档 _id（string）**。不进 IType 路由 → 没有 iid→oid 的转换规则 →
//     数字键在这里没有任何可靠含义，格式化成十进制只是凭空发明一套约定，
//     与业务真实的 _id 对不上时是**静默查不到**。
//     类型特有方法直接收 string 编译期挡住；Get/Val/Select 的 any 是 Handle 接口锁死的，
//     非字符串键运行时告警并跳过。
type MountCollection struct {
	name    string
	keys    Keys //待拉取的 _id 集合，Data 阶段消费后清空
	model   MountModel
	unmount bool //已标记卸载，Release 阶段才真正摘除，见 Updater.Unmount
	dataset *dataset.Collection
	Updater *Updater
}

func newMountCollection(u *Updater, model MountModel) *MountCollection {
	return &MountCollection{
		name:    model.TableName(),
		model:   model,
		dataset: dataset.NewColl(),
		Updater: u,
	}
}

// ===================== Handle 接口公开方法 =====================

// Get 取文档原始对象（model.Getter 塞进来的那个），不存在返回 nil。
//
// 🔴 **拿到的是指针，只读**。直接改它上面的字段不会记脏，改动**只留在内存里、
// 永远写不出去** —— 长命句柄下后续请求还能读到那个改动，看着像是成功了。
// 要改数据一律走 Update / Set。
func (this *MountCollection) Get(key any) (r any) {
	if doc := this.document(key); doc != nil {
		r = doc.Any()
	}
	return
}

// document Get/Val 用：key 必须是文档 _id，非字符串记一条告警后当作查不到。
func (this *MountCollection) document(key any) *dataset.Document {
	id, ok := key.(string)
	if !ok {
		logger.Alert("MountCollection(%v) key 必须是文档 _id(string):%v", this.name, key)
		return nil
	}
	return this.dataset.Val(id)
}

// Val 取文档上数值字段的值，字段名由 Field() 决定（默认 dataset.Fields.VAL）。
// 文档不存在或字段不是数值时返回 0。
//
// 要读别的字段直接走 Document：`coll.Document(id).GetInt64("xxx")` ——
// Val 的签名被 Handle 接口锁死，加不了可选参数。
func (this *MountCollection) Val(key any) (r int64) {
	if doc := this.document(key); doc != nil {
		r = doc.GetInt64(this.Field())
	}
	return
}

// Data 拉取 Select 标记的文档。keys 为空时不查库。
//
// ⚠️ 只有 Updater.Data()/Submit() 会驱动它，而 Updater.data() 开头有
// `if !status.Has(StatusChanged) { return }` 的闸门 —— 该位由本类型的 Select 置起。
func (this *MountCollection) Data() (err error) {
	if err = this.Updater.Error; err != nil {
		return
	}
	if len(this.keys) == 0 {
		return nil
	}
	if err = this.model.Getter(this.Updater, this.dataset, this.keys.ToString()); err == nil {
		this.keys = nil
	}
	return
}

// IMax 临时集合无持有上限概念，恒返回 0（无限）。
func (this *MountCollection) IMax(int32) int64 {
	return 0
}

// IType 临时集合不进 IType 流水线，恒返回 nil。
func (this *MountCollection) IType(int32) IType {
	return nil
}

// Count 统计 iid 匹配的文档数，iid 传 0 统计全部。取 iid 的规则同通用 Collection
// （优先 dataset.Model.GetIID()，回落 Fields.IID 字段名约定，都取不到的不计入）。
//
// ⚠️ **这是不完全统计**：临时集合按 key 惰性加载，只装了 Select 过的那几条，
// 所以它统计的是**内存里的部分**，不是库里的全量。要全量得自己查库。
// 通用 Collection 那边有 RAMTypeAlways 全量驻留可依赖，这里没有。
//
// 与 Len 的区别：Len 是纯 dataset 长度；Count 包含本次请求内尚未落库的新增、
// 排除已标记删除的，与 Collection 的口径一致。
func (this *MountCollection) Count(iid int32) int64 {
	return this.dataset.Count(func(doc *dataset.Document) bool {
		return iid == 0 || docIID(doc) == iid
	})
}

// Select 标记待拉取的 _id，随后由 Updater.Data() 统一查库。
// 已在内存中的 key 直接跳过，不重复查。
func (this *MountCollection) Select(keys ...any) {
	ids := make([]string, 0, len(keys))
	for _, k := range keys {
		id, ok := k.(string)
		if !ok {
			logger.Alert("MountCollection(%v).Select key 必须是文档 _id(string):%v", this.name, k)
			continue
		}
		ids = append(ids, id)
	}
	this.selects(ids...)
}

// selects Select 的字符串版，内部与 Updater.Mount 直接用它。
func (this *MountCollection) selects(ids ...string) {
	for _, id := range ids {
		if this.dataset.Has(id) {
			continue
		}
		if this.keys == nil {
			this.keys = Keys{}
		}
		this.keys.Select(id)
		// 🔴 必须置位：Updater.data() 不见 StatusChanged 直接返回，
		// 漏了这行的症状是"Select 了、Data 了、Get 拿到 nil、还不报错"。
		// 全局句柄由 statement.Select 置位，本类型不用 statement，只能自己置。
		this.Updater.status.Set(StatusChanged)
	}
}

func (this *MountCollection) Parser() Parser {
	return ParserTypeMount
}

// ===================== 类型特有公开方法 =====================

func (this *MountCollection) Name() string {
	return this.name
}

// Field 解析数值字段名：传参优先，其次模型实现的 GetValueJSName()，最后 dataset.Fields.VAL。
// 与通用 Collection 同一套规则。
func (this *MountCollection) Field(field ...string) string {
	if len(field) > 0 {
		return field[0]
	}
	if f, ok := this.model.(CollectionModelValueJSName); ok {
		return f.GetValueJSName()
	}
	return dataset.Fields.VAL
}

// Document 取文档，不存在返回 nil。id 是文档 _id，不是 iid（临时集合没有 iid 路由）。
func (this *MountCollection) Document(id string) *dataset.Document {
	return this.dataset.Val(id)
}

func (this *MountCollection) Has(id string) bool {
	return this.dataset.Has(id)
}

// Receive 把**已经在手上的**文档直接塞进内存，跳过 Select + Data 那次查库。
//
// 挂载不只是"把写操作并进事务"，它同时是这次会话里的一份**缓存** —— 业务常常在别处
// 刚查过、或者刚刚亲手创建了这条数据（下单就是典型：创建完很快会在核销时用到），
// 再走 Mount(keys...) 就是照着 _id 把自己刚写的那条又查一遍。
//
// ⚠️ 塞进来的对象必须是这张表的模型、且它的 _id 与 id 一致，框架不校验。
// ⚠️ 只进内存、**不记脏、不会被写库**。要落库仍然走 Insert / Update。
// ⚠️ 塞进来之后 Select 会认为这条已在内存而跳过，也就是说**后续不会再从库里刷新它** ——
// 别拿它缓存"别处可能改动"的数据。
func (this *MountCollection) Receive(id string, data any) {
	this.dataset.Receive(id, data)
}

func (this *MountCollection) Len() int {
	return this.dataset.Len()
}

func (this *MountCollection) Range(handle func(string, *dataset.Document) bool) {
	this.dataset.Range(handle)
}

// Update 批量改字段，改内存并记脏，Submit 时经 model.Setter 落库。
func (this *MountCollection) Update(id string, data dataset.Update) error {
	if err := this.dataset.Update(id, data); err != nil {
		return this.Updater.Errorf(err)
	}
	return nil
}

// Set 改单个字段，语义同 Update。
func (this *MountCollection) Set(id string, field string, value any) error {
	return this.Update(id, dataset.NewUpdate(field, value))
}

// Insert 插入新文档，Submit 时走 BulkWrite.Insert。
func (this *MountCollection) Insert(i any) error {
	if err := this.dataset.Insert(i); err != nil {
		return this.Updater.Errorf(err)
	}
	return nil
}

// Delete 删除文档，Submit 时走 BulkWrite.Delete。
func (this *MountCollection) Delete(id string) {
	this.dataset.Delete(id)
}

// Remove 仅从内存移除，不动数据库。
func (this *MountCollection) Remove(ids ...string) {
	this.dataset.Remove(ids...)
}

// ===================== Handle 接口私有方法 =====================

// increase/decrease 由 Updater.Add/Sub 经 IType 路由调用，临时句柄不在路由表里，
// 这两个方法**不可达**。留空实现只为满足 Handle 接口。
func (this *MountCollection) increase(int32, int64) {}
func (this *MountCollection) decrease(int32, int64) {}

// verify 临时数据不参与校验：没有 operator，也就没有溢出/扣量/自动分解可查。
func (this *MountCollection) verify() error {
	return nil
}

// reset 每次请求开始。临时句柄跨请求驻留，这里不做任何事
// （不走 ModelReset：那是全局句柄的跨天重置，临时数据的生命周期由 Mount/Unmount 决定）。
func (this *MountCollection) reset() {}

// loading 一律惰性，不预热。
//
// ⚠️ 别照抄 Collection.loading()：它走的是 model.Getter(u, data, nil) —— keys 为 nil 的
// **全量拉取**。那是给全局句柄开服预热用的，临时表（尤其战斗副本）全量拉不可接受。
// 要预热就显式 Select + Data。
func (this *MountCollection) loading() error {
	return nil
}

// reload 丢弃内存，下次 Select+Data 重新查库。由 Updater.Reload 驱动。
func (this *MountCollection) reload() error {
	this.dataset = dataset.NewColl()
	this.keys = nil
	return nil
}

// release 每次请求结束。
//
// ⚠️ 只清 dirty 与待拉取标记，**保留内存数据** —— 长命句柄的跨请求驻留就靠这条。
// 想连内存一起丢是 Unmount 的事，两者别混。
func (this *MountCollection) release() {
	this.keys = nil
	this.dataset.Release()
}

// save 把脏数据经 model.Setter 写进共享 bulkWrite（此时尚未提交）。
func (this *MountCollection) save() error {
	if this.Updater.BulkWrite() == nil {
		return ErrBulkWriteNotInit
	}
	return this.dataset.Save(newCollectionBulkWrite(this.Updater, this.model))
}

// submit 与全局句柄同批次：都往 u.bulkWrite 里写，由 Updater.Submit 末尾一次提交。
//
// ⚠️ 与 Collection.submit 不同，save 失败**不吞**：通用 Collection 在 ram != None 时
// 可以"等下次同步"，而临时句柄随时会被 Unmount 掉，没有下次。
func (this *MountCollection) submit() (err error) {
	if err = this.Updater.WriteAble(); err != nil {
		return
	}
	return this.save()
}

// destroy 玩家下线：刷盘，不是 release。
func (this *MountCollection) destroy() error {
	return this.save()
}
