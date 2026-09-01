package updater

import (
	"github.com/hwcer/cosgo/schema"
	"github.com/hwcer/logger"
	"github.com/hwcer/updater/dataset"
	"github.com/hwcer/updater/operator"
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

// MountCollection 临时数据集合：与玩家数据同批次原子写库。
//
// 🔴 **它走的是与 Collection 完全相同的那条流水线**：改动先变成 operator 入队，
// verify 阶段经 Collection.Parse 消费、写进内存并记脏，submit 阶段经 model.Setter
// 落进共享 bulkWrite。所以它内嵌 Collection —— 不是"像"，就是同一套。
//
// 这样做的直接好处：
//   - 改数据一律经 operator，请求失败时 release 自动丢弃，内存不会留下库里没有的状态；
//   - 产出的 operator 可以像普通道具变更一样下发客户端（Forward），也可以手动取（Operators）。
//
// 与通用 Collection 的四点差别，用之前看清楚：
//
//  1. **不进 IType 路由**：IType/IMax 恒空，operator 的 IID 恒 0。
//     u.Add/u.Sub/u.Set/u.Select 一律路由不到这里，只能拿本对象操作；
//  2. **不支持 Add/Sub**：那两条要靠 IType 建对象（ITypeCollection.New），临时数据没有 IType。
//     调用会记一条告警并返回 nil，不会静默；
//  3. **key 只能是文档 _id（string）**：没有 iid→oid 的转换规则，数字键在这里没有可靠含义；
//  4. **operator 默认不下发客户端**：临时数据（邮件领取标记、订单状态、兑换码占用）
//     客户端有自己的协议，不该混进 S2CUpdate。要下发显式 Forward(true)。
type MountCollection struct {
	Collection
	ops     []*operator.Operator //本次请求产生的 operator，见 Operators
	forward bool                 //产生的 operator 是否随本次请求下发客户端
	unmount bool                 //已标记卸载，Release 阶段才真正摘除，见 Updater.Unmount
}

func newMountCollection(u *Updater, model MountModel) *MountCollection {
	r := &MountCollection{}
	r.name = model.TableName()
	r.model = model
	r.dataset = dataset.NewColl()
	//ram 取 Always：release 时只清 dirty、保留内存数据，长命挂载的跨请求驻留靠它。
	//（loader 始终为 false —— loading 被覆盖成不预热，所以 statement.has 不会误判"全都在"）
	mod := &Model{ram: RAMTypeAlways, name: r.name, model: model, parser: ParserTypeMount}
	r.statement = *newStatement(u, mod, r.Has)
	r.statement.Receiver(r.receive)
	return r
}

// ===================== operator 入口（全部产 operator，不直接改内存）=====================

// operator 挂载专用的 Operator 构造，**不能复用 Collection.operator**，两处原因：
//
//  1. 那边对 string 型 id 会调 Config.ParseId 去解析 iid —— 挂载的 _id 是业务自己的主键
//     （uid-code、平台订单号…），根本不是项目的 OID 格式，解析失败会**把
//     Updater.Error 打脏、整个请求失败**；
//  2. 那边接着调 mayChange，而 mayChange 拿不到 IType 直接返回 ErrITypeNotExist。
//
// 所以这里 IID 恒 0（临时数据没有 iid），但仍然走 format —— 那一步把字段名统一成
// JSName，与 Collection 同口径，落库时再由 cosmo 在边界换成 DBName。
func (this *MountCollection) operator(t operator.Types, id string, v int64, r any) *operator.Operator {
	if err := this.Updater.WriteAble(); err != nil {
		return nil
	}
	if id == "" {
		this.Updater.Error = ErrObjectIdEmpty(t.ToString())
		return nil
	}
	op := operator.New(t, "", v, r)
	op.OID = id
	this.format(op)
	if this.Updater.Error != nil {
		op.Release()
		return nil
	}
	this.statement.insert(op)
	return op
}

// Update 批量改字段。产生一条 TypesSet operator，verify 阶段才真正写进内存。
func (this *MountCollection) Update(id string, data dataset.Update) *operator.Operator {
	return this.operator(operator.TypesSet, id, 0, data)
}

// Set 改单个字段，语义同 Update。
func (this *MountCollection) Set(id string, field string, value any) *operator.Operator {
	return this.Update(id, dataset.NewUpdate(field, value))
}

// Unset 删字段。
func (this *MountCollection) Unset(id string, fields ...string) *operator.Operator {
	data := dataset.Update{}
	for _, f := range fields {
		data[f] = nil
	}
	return this.operator(operator.TypesUnset, id, 0, data)
}

// Delete 删文档。
func (this *MountCollection) Delete(id string) *operator.Operator {
	return this.operator(operator.TypesDel, id, 0, nil)
}

// Insert 插入新文档。id 必须与对象里的 _id 一致。
//
// ⚠️ 不复用 Collection.New：那个要求对象实现 dataset.Model（GetOID/GetIID），
// 而 GetIID 返回的业务 iid 会被当成 updater 的 iid 一路带进溢出检查。挂载显式传 id，
// IID 恒 0。
func (this *MountCollection) Insert(id string, v any) *operator.Operator {
	return this.operator(operator.TypesNew, id, 1, []any{v})
}

// Add 不支持：要靠 IType 建对象（ITypeCollection.New），而临时数据不进 IType 体系。
// 记一条告警返回 nil，不静默。
func (this *MountCollection) Add(id any, _ any, _ ...string) *operator.Operator {
	logger.Alert("MountCollection(%v) 不支持 Add，临时数据没有 IType：%v", this.name, id)
	return nil
}

// Sub 不支持，理由同 Add。
func (this *MountCollection) Sub(id any, _ any, _ ...string) *operator.Operator {
	logger.Alert("MountCollection(%v) 不支持 Sub，临时数据没有 IType：%v", this.name, id)
	return nil
}

// ===================== operator 的去向 =====================

// Forward 本挂载产生的 operator 是否随本次请求下发给客户端（进 Updater.dirty → S2CUpdate）。
//
// 默认 **false**：临时数据（邮件领取标记、订单状态、兑换码占用）客户端有自己的协议，
// 混进道具变更推送只会让两边都看不懂。确实要让客户端按统一通道感知时才打开。
//
// ⚠️ 无论开关如何，Operators() 都取得到 —— 两件事互不影响。
func (this *MountCollection) Forward(v bool) {
	this.forward = v
}

// Operators 本次请求已经过 verify 的 operator 列表（只读）。
//
// 给"要自己决定怎么告诉客户端"的场合用：拿到之后自行组装协议，
// 而不是走 Forward 那条统一通道。
//
// ⚠️ 只在本次请求内有效：Release 之后 operator 会被回收进池子，再读就是脏数据。
func (this *MountCollection) Operators() []*operator.Operator {
	return this.ops
}

// receive statement 的操作接收器：一律留一份给 Operators，按需再放进 Updater.dirty。
func (this *MountCollection) receive(u *Updater, ops []*operator.Operator) {
	this.ops = append(this.ops, ops...)
	if this.forward {
		u.dirty = append(u.dirty, ops...)
	}
}

// ===================== 缓存 =====================

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

// ===================== 覆盖 Collection 的 IType 相关行为 =====================

func (this *MountCollection) Parser() Parser {
	return ParserTypeMount
}

// IType 临时集合不进 IType 流水线，恒返回 nil。
//
// ⚠️ 必须覆盖：Collection.IType 会回落到全局 Config.IType，
// 拿一个不属于它的 iid 去查全局配置，得到的是随机结果。
func (this *MountCollection) IType(int32) IType {
	return nil
}

// IMax 临时集合不参与溢出检查，恒返回 0（无限）。理由同 IType。
func (this *MountCollection) IMax(int32) int64 {
	return 0
}

// Count 统计 iid 匹配的文档数，iid 传 0 统计全部。
//
// ⚠️ **这是不完全统计**：挂载按 key 惰性加载，只装了 Select 过的那几条，
// 所以它统计的是内存里的部分，不是库里的全量。
func (this *MountCollection) Count(iid int32) int64 {
	return this.dataset.Count(func(doc *dataset.Document) bool {
		return iid == 0 || docIID(doc) == iid
	})
}

// ===================== 生命周期 =====================

// loading 一律惰性，不预热。
//
// ⚠️ 别用 Collection.loading()：它走 model.Getter(u, data, nil) —— keys 为 nil 的
// **全量拉取**。那是给全局句柄开服预热用的，临时表（尤其战斗副本）全量拉不可接受。
// 要预热就显式 Select + Data。
func (this *MountCollection) loading() error {
	return nil
}

// reload 丢弃内存，下次 Select+Data 重新查库。
// ⚠️ 不能用 Collection.reload()：它置 dataset=nil 后指望 loading() 重建，而这里不预热。
func (this *MountCollection) reload() error {
	this.dataset = dataset.NewColl()
	this.statement.reload()
	return nil
}

// release 每次请求结束。
//
// ⚠️ 只清 dirty 与待拉取标记，**保留内存数据** —— 长命挂载的跨请求驻留靠这条。
// 想连内存一起丢是 Unmount 的事，两者别混。
//
// 没下发客户端的 operator 由这里回收：下发过的已经在 Updater.dirty 里被回收过一轮，
// 再放一次就是双重释放。
func (this *MountCollection) release() {
	if !this.forward {
		for _, op := range this.ops {
			op.Release()
		}
	}
	this.ops = nil
	this.Collection.release()
}

// submit 与全局句柄同批次：都往 u.bulkWrite 里写，由 Updater.Submit 末尾一次提交。
//
// ⚠️ 与 Collection.submit 不同，save 失败**不吞**：通用 Collection 在 ram != None 时
// 可以"等下次同步"，而临时句柄随时会被 Unmount 掉，没有下次。
func (this *MountCollection) submit() (err error) {
	if err = this.Updater.WriteAble(); err != nil {
		return
	}
	this.statement.submit()
	return this.save()
}
