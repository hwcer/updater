package updater

import (
	"fmt"

	"github.com/hwcer/cosgo/schema"
	"github.com/hwcer/logger"
	"github.com/hwcer/updater/dataset"
	"github.com/hwcer/updater/operator"
)

// 临时句柄挂载。
//
// 为"Updater 之外、但要与玩家数据同批次原子写库"的数据准备：邮件领取标记、兑换码占用、
// 充值订单、临时战斗副本。共同点是要同批次原子写库 + 按需查库 + 可选内存驻留，
// **不进 IType 路由、不自动生成给客户端的 operator**。
//
// 入口是 Updater.Mount / Mounted / Unmount，句柄类型是 Mount。设计取舍见 HANDLER_MOUNT_PLAN.md。

// MountModel 临时数据模型。
//
// 组合 schema.Tabler 是必需的：挂载名取自 TableName()，而 CollectionModel 本身不含它 ——
// 只声明 CollectionModel 的话就得在 Mount 内部做运行时类型断言，"传错模型编不过"这句就不成立。
//
// ⚠️ 取名规则与 Register 并不一致，这是有意的：Register 对非 schema.Tabler 的模型有
// schema.Kind(model).Name() 兜底，Mount 没有兜底路径。
//
// ⚠️ Getter 只会收到**非空**的 keys —— 挂载是按需加载的，从不做全量拉取。
// 模型里那条 `len(keys) == 0 时直接返回` 的保护仍建议留着（它同时被别处调用时有用），
// 但挂载这条链不会走到它。
type MountModel interface {
	CollectionModel
	schema.Tabler
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
//
// 带 keys 时等价于 Select(keys...) + Data()，**当场查库**（不等框架的 Data 阶段）——
// 临时数据取回来就是要马上用的，分两步写只是多一次出错的机会。
// 已在内存里的 key 会被 Select 跳过，所以长命句柄反复这么调不会重复查库。
//
// 挂载名取 model.TableName()，与已注册的全局模型重名时报错：撞名的话同一张表在一个
// Updater 里会有两个句柄各写各的，是静默的数据竞争。
//
// ⚠️ 不带 keys 时**不做任何预加载**。别指望它像全局句柄那样开场全量拉 ——
// 那对战斗副本这类表是灾难。
//
// ⚠️ **挂载与取数是两码事**：查库失败时返回 (句柄, err) —— 句柄已经挂上且完全可用，
// 只是这几条没读回来，重试一次 Select + Data 即可。唯一会返回 nil 的是重名，
// 那时压根没挂上。
func (u *Updater) Mount(model MountModel, keys ...string) (*Mount, error) {
	name := model.TableName()
	r, exist := u.mounts[name]
	if !exist {
		for _, m := range modelsRank {
			if m.name == name {
				return nil, Errorf(0, "mount name conflicts with registered model:%v", name)
			}
		}
		if u.mounts == nil {
			u.mounts = make(map[string]*Mount)
		}
		r = newMount(u, model)
		r.reset()
		u.mounts[name] = r
	}
	r.unmount = false //改主意了:上一次标记的卸载作废
	if len(keys) == 0 {
		return r, nil
	}
	for _, k := range keys {
		r.Select(k)
	}
	return r, r.Data()
}

// Mounted 取回已挂载的临时集合，未挂载返回 nil。
//
// 与 Mount 的区别：它**只取不挂**，也不取数——用来问"挂了没"。
// 三个入口统一收 MountModel，业务层不必关心挂载名是怎么来的。
func (u *Updater) Mounted(model MountModel) *Mount {
	return u.mounts[model.TableName()]
}

// Unmount 标记卸载。**只打标记，真正摘除在 Release 阶段**（EventTypeRelease 之后）。
//
// 🔴 不在这里直接刷盘/摘除，是为了让短流程也走完整生命周期。短命场景的标准写法是
//
//	coll, err := u.Mount(&model.Mail{}, ids...)
//	defer u.Unmount(&model.Mail{})
//
// 而 defer 在 **handler 返回时**执行，框架的 Submit 排在那之后
// （`Updater.Verify` 的注释里写着"handle 返回后框架才 Submit"）。当场摘除的话，
// 这次改动就永远写不出去且一声不吭；退一步在 Unmount 里自己 save 也不对 ——
// 那是绕开 submit 另开一条旁路，与全局句柄的路径不一致，以后 submit 上加的任何东西
// 短流程都吃不到。
//
// 打完标记后句柄照旧留在 mounts 里，正常参与 Data / verify / submit，
// 直到请求结束才被摘掉 —— 长短两档走的是同一条路。
//
// ⚠️ 标记可撤销：同一请求内再次 Mount 同一模型会清掉它（业务改主意了，句柄还给它）。
//
// ⚠️ 卸载粒度是**整张表**，不是"这一条"。同一模型上并发多个实例（一个玩家两场战斗）时，
// 先结束的那场 Unmount 会把另一场的内存一起端掉 —— 数据不会丢（每次 Submit 都写穿了库），
// 但对方后续 Get 会拿到 nil 直到重新 Select。口径：**只有最后一个实例结束时才 Unmount**；
// 业务判断不了是不是最后一个，就别卸，留给玩家下线兜底（Destroy 会刷盘）。
func (u *Updater) Unmount(model MountModel) {
	if r, ok := u.mounts[model.TableName()]; ok {
		r.unmount = true
	}
}

// Mount 临时挂载集合：与玩家数据同批次原子写库。
//
// 给"Updater 之外、但要跟着玩家数据一起成败"的数据用：邮件领取标记、兑换码占用、
// 充值订单、临时战斗副本。挂载名取模型的 TableName()，见 Updater.Mount。
//
// 🔴 **它走的是与 Collection 相同的那条流水线**：改动先变成 operator 入队，
// verify 阶段消费（写进内存并记脏），submit 阶段经 model.Setter 落进共享 bulkWrite。
// 由此改数据一律经 operator，请求失败时 release 自动丢弃，
// 内存不会留下库里没有的状态 —— 这就是「取到的指针一律只读」在挂载上的落点。
//
// ⚠️ **但它不内嵌 Collection，整套接口自己实现。** 曾经内嵌过一版，反复栽在同一件事上：
// Go 的方法提升**没有虚派发**，Collection 内部调到的永远是它自己的方法。于是
// "覆盖了却不生效"（IType/IMax）、"以为继承了其实语义不同"（Select 走全局 IType 换 oid）、
// "抄了一半"（submit 漏掉 remove）接连出现，每修一处都要重新推演一遍派发路径。
// 挂载真正需要的只有 Set / New / Del / Unset 四种操作，自己写清楚反而短。
//
// 与 Collection 的分界：
//   - 不进 IType 体系：operator 的 IID 恒 0，没有溢出检查与自动分解
//     （IMax/IType 压根不实现 —— 它们已经从 Handle 接口上摘掉了，见 funcs.go 的 overflowHandle）；
//     u.Add/u.Sub/u.Set/u.Select 一律路由不到这里，只能拿本对象操作；
//   - 没有 Add/Sub：那两条的语义是"按 iid 增减持有量"，挂载没有 iid；
//   - key 只能是文档 _id（string）：没有 iid→oid 的转换规则；
//   - 下发客户端与否由模型有没有声明 ModelIType 决定，没有开关，见 itype 字段。
type Mount struct {
	statement
	name string
	// schema 首次取到后缓存。model 在构造之后不再变，其 schema 也就固定
	//（业务侧常见实现是 schema.Parse(this)，自己不缓存，而 format 里每个操作都要取一次）
	schema  *schema.Schema
	model   MountModel
	remove  []string //待从内存移除的 _id，submit 时统一处理（落库之后再摘，别丢掉未保存的改动）
	dataset *dataset.Collection
	// itype 取自 ModelIType.IType(0)，取不到就是 0。
	//
	// 🔴 它同时决定**产出的 operator 要不要走通用更新下发客户端**：非 0 才下发。
	//
	// 这**不是一个可以拨的开关，而是物理事实**：IType 是客户端找到"这条变更属于哪张表"
	// 的唯一钥匙(operator 里只有 OID、字段和值)。客户端的分发入口第一行就是
	// `if (op.IType == 0) return;` —— 带 0 发过去连丢在哪都不知道，Display 飘字也不会触发。
	// 所以这段逻辑订死，不必再找别的策略。不下发时用 Operators() 自己组协议。
	//
	// ⚠️ **判据是"IType(0) 返回非 0"，不是"实现了 ModelIType"** —— 后者几乎恒成立：
	// 项目侧的模型基类往往自带一个 IType(iid) 转发给全局配置，于是每个模型都"自动满足"
	// 这个接口，而全局配置对 iid=0 通常返回 0。想让某张挂载表走通用通道，
	// 模型必须**显式覆盖** IType 返回该表自己的类型，光加一行接口断言不起作用。
	itype   int32
	unmount bool //已标记卸载，Release 阶段才真正摘除，见 Updater.Unmount
}

// mountDiscard 不下发客户端时用的接收器：把 operator 从"默认进 Updater.dirty"那条路上摘下来。
//
// 故意什么都不做、**不 Release**：业务可能刚从 Operators() 里把同一批对象拿在手上。
// 不还池子只是少一点复用，交给 GC 就好 —— 拿"省一次分配"去换悬垂引用不值。
func mountDiscard(*Updater, []*operator.Operator) {}

func newMount(u *Updater, model MountModel) *Mount {
	r := &Mount{name: model.TableName(), model: model, dataset: dataset.NewColl()}
	if m, ok := model.(ModelIType); ok {
		r.itype = m.IType(0) //挂载没有 iid，按 ModelIType 的约定传 0 取默认值
	}
	//ram 只影响 statement.has 里那条 `Always && loader` 的短路。挂载按需加载，
	//绝不能让它命中——命中之后 Select 会认为"全都在内存里"而跳过每一个 key，
	//Data 永不执行、Get 全返回 nil，且不报错。
	mod := &Model{ram: RAMTypeMaybe, name: r.name, model: model, parser: ParserTypeCollection}
	r.statement = *newStatement(u, mod, r.exist)
	if r.itype == 0 {
		//客户端认不出没有 IType 的变更，别往通用更新里塞。
		//留 statement 的默认接收器(进 Updater.dirty)才是"下发"。
		r.statement.Receiver(mountDiscard)
	}
	return r
}

// ===================== 操作入口（全部产 operator，不直接改内存）=====================

// operator 构造并入队一条 Operator。
//
// 🔴 与 Collection.operator 的分界就在这里：那边对 string 型 id 会调 Config.ParseId
// 去解析 iid —— 挂载的 _id 是业务自己的主键（uid-code、平台订单号…），根本不是项目的
// OID 格式，解析失败会把 Updater.Error 打脏、整个请求失败；那边接着调的 mayChange
// 又硬要一个 IType。挂载两样都不需要，所以 IID 恒 0。
func (this *Mount) operator(t operator.Types, id string, v int64, r any) *operator.Operator {
	if err := this.Updater.WriteAble(); err != nil {
		return nil
	}
	if id == "" {
		this.Updater.Error = ErrObjectIdEmpty(t.ToString())
		return nil
	}
	op := operator.New(t, "", v, r)
	op.OID = id
	op.IType = this.itype //0 表示模型没声明，这条 operator 也就不会下发客户端
	this.format(op)
	if this.Updater.Error != nil {
		op.Release()
		return nil
	}
	this.statement.insert(op)
	return op
}

// Update 批量改字段。只是入队，verify 阶段才真正写进内存。
func (this *Mount) Update(id string, data dataset.Update) *operator.Operator {
	return this.operator(operator.TypesSet, id, 0, data)
}

// Set 改单个字段，语义同 Update。
func (this *Mount) Set(id string, field string, value any) *operator.Operator {
	return this.Update(id, dataset.NewUpdate(field, value))
}

// Unset 删字段。
func (this *Mount) Unset(id string, fields ...string) *operator.Operator {
	data := dataset.Update{}
	for _, f := range fields {
		data[f] = nil
	}
	return this.operator(operator.TypesUnset, id, 0, data)
}

// Delete 删文档。
func (this *Mount) Delete(id string) *operator.Operator {
	return this.operator(operator.TypesDel, id, 0, nil)
}

// Insert 插入新文档，_id 从对象上取，取不到即报错。
//
// ⚠️ **不要另给一个 id 参数**：真正决定落库主键的是对象自己的 _id，
// 额外那个只会进 operator.OID。两者一旦不一致，库里存的是一个键、
// 发给客户端的 operator 说的是另一个键，而且不报错。
func (this *Mount) Insert(v any) *operator.Operator {
	doc := dataset.NewDoc(v)
	//Value 取对象上的数值字段（字段名由 Field() 定），**对象上没有这个字段才回落 1** ——
	//与 Collection.New 同口径，字段存在且为 0 时就是 0。
	//挂载文档多半没有"数量"概念，这个值只在模型声明了 ModelIType、
	//operator 要下发客户端时才有意义（对面按道具变更渲染）。
	n := int64(1)
	if i, ok := doc.Get(this.Field()); ok && i != nil {
		n = dataset.ParseInt64(i)
	}
	return this.operator(operator.TypesNew, doc.GetString(dataset.Fields.OID), n, []any{v})
}

// ===================== operator 的去向 =====================

// 下发客户端与否**没有开关**：模型声明了 ModelIType（IType(0) 非 0）就走通用更新
// （operator 进 Updater.dirty → S2CUpdate），否则不下发。理由见 Mount.itype 字段。
//
// 临时数据大多属于后者：邮件领取标记、订单状态、兑换码占用，客户端都有自己的协议，
// 混进道具变更推送只会让两边都看不懂。

// Operators 本次请求**已通过 verify、尚未 submit** 的 operator 列表（只读），给业务层用。
//
// 用在"要自己决定怎么告诉客户端"的场合：拿到之后自行组装协议 ——
// 模型没声明 ModelIType 的挂载本来就不走通用更新，客户端全靠这条路知情。
//
// 🔴 **它读的就是 statement.cache**，所以时机很具体：
//   - operator 在 verify 阶段进 cache —— 业务手动 u.Verify() 之后就能读到；
//   - submit 阶段 cache 交给接收器并置空 —— 那之后再读是空的；
//   - 请求失败没走到 submit 时，由 statement.release 统一回收。
//
// 不另存一份的理由：cache 的填充与清理 statement 已经管好了，
// 再抄一个切片就是两套生命周期，迟早对不上。
func (this *Mount) Operators() []*operator.Operator {
	return this.statement.cache
}

// ===================== 读取 =====================

func (this *Mount) Name() string {
	return this.name
}

func (this *Mount) Schema() *schema.Schema {
	if this.schema == nil {
		this.schema = this.model.Schema()
	}
	return this.schema
}

// Field 解析数值字段名：传参优先，其次模型实现的 GetValueJSName()，最后 dataset.Fields.VAL。
func (this *Mount) Field(field ...string) string {
	if len(field) > 0 {
		return field[0]
	}
	if f, ok := this.model.(CollectionModelValueJSName); ok {
		return f.GetValueJSName()
	}
	return dataset.Fields.VAL
}

// Document 取文档，不存在返回 nil。
func (this *Mount) Document(id string) *dataset.Document {
	return this.dataset.Val(id)
}

func (this *Mount) Has(id string) bool {
	return this.dataset.Has(id)
}

func (this *Mount) Len() int {
	return this.dataset.Len()
}

func (this *Mount) Range(handle func(string, *dataset.Document) bool) {
	this.dataset.Range(handle)
}

// Remove 仅从内存移除，不动数据库。**submit 落库之后才真正摘除** ——
// 立即摘的话会把这条尚未保存的改动一起丢掉。
func (this *Mount) Remove(id ...string) {
	this.remove = append(this.remove, id...)
}

// Submit 把**本挂载**的改动单独落库，不等 Updater 整体提交。
//
// 给「这一趟末尾要 return error、但这份数据必须留下」的场合用：平台回来的订单状态、
// 三方结算回执这类**已经发生的事实**，不该跟着业务失败一起回滚。
// 以前只能绕开事务直接写库（自己拼 update、还要手动把新值同步回内存），
// 现在走的仍是 operator 流水线 —— 格式化、schema 校验、内存同步全都照旧。
//
// 🔴 用一份**独立的 BulkWrite**，不碰 Updater 那份共享实例：
// 共享那份里装着玩家数据的改动，提交它等于把整个请求提前落库，那不叫单表提交。
//
// ⚠️ 提交之后这个挂载会处于「已落库」状态，而请求可能还会失败回滚。三条后果要清楚：
//
//	内存    已是新值(verify 时写的)，与库一致 —— 这正是要的
//	客户端  operator 还在 cache 里等 Updater.Submit 交付；请求失败就不会下发。
//	        本表若声明了 IType，就会出现"库变了、客户端不知道"，得自己补推
//	玩家数据 完全不受影响，该回滚照样回滚
//
// ⚠️ **失败时内存已经是新值、库还是旧的**（verify 在落库之前）。别当没事发生：
// 要么重试，要么 Unmount 整张表（Release 时摘除，下次请求重新从库加载）。
//
// ⚠️ 全局的 StatusOperated **不会**被清除：它是所有 handle 共用的一个标志，
// 为了"我这张表校验过了"去清它，会让其它 handle 的待校验操作被整体跳过、
// 玩家数据静默丢失。留着的代价只是后续 converge 对本挂载空转一次
// （statement.verify 消费完已把队列置 nil）。
func (this *Mount) Submit() error {
	if err := this.Updater.WriteAble(); err != nil {
		return err
	}
	if err := this.verify(); err != nil {
		return err
	}
	//没有脏数据就别打库:业务不必自己判断"这次到底改没改",重复调用也是零成本。
	//verify 在上面已经跑过,该进 dirty 的都进了。
	if len(this.dataset.Dirty()) == 0 {
		return nil
	}
	if Config.BulkWrite == nil {
		return ErrBulkWriteNotInit
	}
	bulk := Config.BulkWrite(this.Updater)
	if bulk == nil {
		return ErrBulkWriteNotInit
	}
	if err := this.dataset.Save(newCollectionBulkWrite(this.Updater, this.model, bulk)); err != nil {
		return err
	}
	//测试模式只改内存不写库，与 Updater.Submit 的口径一致:数据已经进了这份 bulk，
	//丢掉不提交即可。
	if this.Updater.status.Has(StatusTesting) {
		return nil
	}
	if err := bulk.Submit(); err != nil {
		return err
	}
	//落库了才摘，与 submit() 同一条规矩(见 Remove)
	if len(this.remove) > 0 {
		this.dataset.Remove(this.remove...)
		this.remove = nil
	}
	return nil
}

// Receive 把**已经在手上的**文档直接塞进内存，跳过 Select + Data 那次查库。
//
// 挂载不只是"把写操作并进事务"，它同时是这次会话里的一份**缓存** —— 业务常常在别处
// 刚查过、或者刚刚亲手创建了这条数据（充值的下单 → 核销横跨多次请求就是典型）。
//
// ⚠️ 塞进来的对象必须是这张表的模型、且它的 _id 与 id 一致，框架不校验。
// ⚠️ 只进内存、**不记脏、不会被写库**。要落库仍然走 Insert / Update。
// ⚠️ 塞进来之后 Select 会认为这条已在内存而跳过，也就是说**后续不会再从库里刷新它** ——
// 别拿它缓存"别处可能改动"的数据。
func (this *Mount) Receive(id string, data any) {
	this.dataset.Receive(id, data)
}

// ===================== Handle 接口 =====================

// Get 取文档原始对象（model.Getter 塞进来的那个），不存在返回 nil。
//
// 🔴 **拿到的是指针，只读**。直接改它上面的字段不记脏，改动**只留在内存里、
// 永远写不出去** —— 长命挂载下后续请求还能读到那个改动，看着像是成功了。
// 要改数据一律走 Update / Set。
func (this *Mount) Get(key any) (r any) {
	if doc := this.document(key); doc != nil {
		r = doc.Any()
	}
	return
}

// Val 取文档上数值字段的值，字段名由 Field() 决定（默认 dataset.Fields.VAL）。
// 要读别的字段走 Document：`coll.Document(id).GetInt64("xxx")`。
func (this *Mount) Val(key any) (r int64) {
	if doc := this.document(key); doc != nil {
		r = doc.GetInt64(this.Field())
	}
	return
}

// Data 拉取 Select 标记的文档。keys 为空时不查库。
//
// ⚠️ 只有 Updater.Data()/Submit() 会驱动它，而 Updater.data() 开头有
// `if !status.Has(StatusChanged) { return }` 的闸门 —— 该位由 Select 置起。
func (this *Mount) Data() (err error) {
	if err = this.Updater.Error; err != nil {
		return
	}
	if len(this.keys) == 0 {
		return nil
	}
	if err = this.model.Getter(this.Updater, this.dataset, this.keys.ToString()); err == nil {
		this.statement.date() //keys = nil
	}
	return
}

// Count 统计 iid 匹配的文档数，iid 传 0 统计全部。
//
// ⚠️ **这是不完全统计**：挂载按 key 惰性加载，只装了 Select 过的那几条，
// 所以它数的是内存不是库。
func (this *Mount) Count(iid int32) int64 {
	return this.dataset.Count(func(doc *dataset.Document) bool {
		return iid == 0 || docIID(doc) == iid
	})
}

// Select 标记待拉取的 _id，随后由 Updater.Data() 统一查库。
// 已在内存中的 key 直接跳过，不重复查。
func (this *Mount) Select(keys ...any) {
	for _, k := range keys {
		if id, ok := k.(string); ok {
			this.statement.Select(id)
		} else {
			logger.Alert("Mount(%v).Select key 必须是文档 _id(string):%v", this.name, k)
		}
	}
}

// Parser 挂载不在全局注册表里，这个返回值没有任何消费者；
// 形态上最接近 Collection，就报它。
func (this *Mount) Parser() Parser {
	return ParserTypeCollection
}

// ===================== Handle 接口私有方法 =====================

// increase/decrease 由 Updater.Add/Sub 经 IType 路由调用，挂载不在路由表里，
// 这两个方法**不可达**。留空实现只为满足 Handle 接口。
func (this *Mount) increase(int32, int64) {}
func (this *Mount) decrease(int32, int64) {}

// verify 消费待处理的 operator：写进内存并记脏。
func (this *Mount) verify() (err error) {
	if err = this.Updater.WriteAble(); err != nil {
		return
	}
	//下标遍历而非 range：当前四个 parse 分支都不会往 statement.operator 追加，
	//但那是实现的性质、不是接口保证 —— range 按初始长度迭代，
	//哪天有分支开始追加就会静默漏掉新增的那几条（Collection 那边正是为此踩过）。
	for i := 0; i < len(this.statement.operator); i++ {
		if err = this.parse(this.statement.operator[i]); err != nil {
			return
		}
	}
	this.statement.verify()
	return
}

// submit 与全局句柄同批次：都往 u.bulkWrite 里写，由 Updater.Submit 末尾一次提交。
//
// ⚠️ save 失败**只告警不返回**，与 Collection 逐字同口径（包括 Config.BulkWrite 没配
// 这种情况——那是"数据库没配"级别的问题，不归某一个句柄在运行期发明更严的行为）：
// 走到这一步内存已经改完了
// （dataset.Save 里 doc.Save() 排在 Setter 之前），返回错误既回滚不了内存，
// 还会连累 bulkWrite 整个不提交，分歧只会更大。Updater 要的是最终一致；
// 真正的同批次原子保证在这之前（业务错误、verify 失败都拦在 bulkWrite 提交之前）。
func (this *Mount) submit() (err error) {
	if err = this.Updater.WriteAble(); err != nil {
		return
	}
	this.statement.submit()
	if err = this.save(); err != nil {
		logger.Alert("挂载集合同步数据失败,name:%v,err:%v", this.name, err)
		err = nil
	}
	if len(this.remove) > 0 {
		this.dataset.Remove(this.remove...)
		this.remove = nil
	}
	return
}

// save 把脏数据经 model.Setter 写进共享 bulkWrite（此时尚未提交）。
func (this *Mount) save() error {
	if this.Updater.BulkWrite() == nil {
		return ErrBulkWriteNotInit
	}
	return this.dataset.Save(newCollectionBulkWrite(this.Updater, this.model))
}

// reset 每次请求开始。
//
// ⚠️ 不走 ModelReset：那是全局句柄的跨天重置，挂载的生命周期由 Mount/Unmount 决定。
func (this *Mount) reset() {
	if this.dataset == nil {
		this.dataset = dataset.NewColl()
	}
}

// loading 一律惰性，不预热 —— 挂载从不做 keys 为 nil 的全量拉取。
func (this *Mount) loading() error {
	return nil
}

// reload 丢弃内存，下次 Select+Data 重新查库。
func (this *Mount) reload() error {
	this.dataset = dataset.NewColl()
	this.statement.reload()
	return nil
}

// release 每次请求结束。
//
// ⚠️ 只清 dirty 与待拉取标记，**保留内存数据** —— 长命挂载的跨请求驻留靠这条。
// 想连内存一起丢是 Unmount 的事，两者别混。
func (this *Mount) release() {
	this.statement.release()
	this.remove = nil
	this.dataset.Release()
}

// destroy 玩家下线：刷盘。
func (this *Mount) destroy() error {
	return this.save()
}

// ===================== 内部 =====================

// exist 交给 statement 判断 key 是否已在内存（Select 去重用）。
func (this *Mount) exist(k any) bool {
	id, ok := k.(string)
	return ok && this.dataset.Has(id)
}

// document Get/Val 用：key 必须是文档 _id，非字符串记一条告警后当作查不到。
func (this *Mount) document(key any) *dataset.Document {
	id, ok := key.(string)
	if !ok {
		logger.Alert("Mount(%v) key 必须是文档 _id(string):%v", this.name, key)
		return nil
	}
	return this.dataset.Val(id)
}

// format 把 op.Result 里的字段名统一成 JSName，与 Collection 同口径：
// op.Result 既是发客户端的 payload、又经 dataset 进 dirty 落库，
// 落库那侧由 cosmo 的 Update.Transform 在边界换成 DBName。
func (this *Mount) format(op *operator.Operator) {
	if op.OType != operator.TypesSet && op.OType != operator.TypesUnset {
		return
	}
	result, ok := op.Result.(dataset.Update)
	if !ok {
		this.Updater.Error = fmt.Errorf("mount[%s] operator result must be dataset.Update:%v", this.name, op.Result)
		return
	}
	sch := this.Schema()
	if sch == nil {
		this.Updater.Error = fmt.Errorf("mount[%s] schema empty", this.name)
		return
	}
	data := dataset.Update{}
	for k, v := range result {
		name, err := sch.JSName(k)
		if err != nil {
			this.Updater.Error = fmt.Errorf("mount[%s] field error,field:%s,error:%v", this.name, k, err)
			return
		}
		data[name] = v
	}
	op.Result = data
}

// parse 挂载只认这四种操作：没有 Add/Sub（那是按 iid 增减持有量，挂载没有 iid），
// 没有 Drop/Resolve（那是溢出分解，挂载不参与溢出检查）。
func (this *Mount) parse(op *operator.Operator) error {
	switch op.OType {
	case operator.TypesSet:
		return this.parseSet(op)
	case operator.TypesUnset:
		return this.parseUnset(op)
	case operator.TypesDel:
		return this.parseDel(op)
	case operator.TypesNew:
		return this.parseNew(op)
	}
	return fmt.Errorf("mount[%s] operator type not supported:%v", this.name, op.OType.ToString())
}

func (this *Mount) parseSet(op *operator.Operator) error {
	update, ok := op.Result.(dataset.Update)
	if !ok {
		return ErrArgsIllegal(op.OID, op.Result)
	}
	if !this.dataset.Has(op.OID) {
		return ErrItemNotExist(op.OID)
	}
	return this.dataset.Update(op.OID, update)
}

func (this *Mount) parseUnset(op *operator.Operator) error {
	doc := this.dataset.Val(op.OID)
	if doc == nil {
		return ErrItemNotExist(op.OID)
	}
	fields, _ := op.Result.(dataset.Update)
	for k := range fields {
		doc.Unset(k)
	}
	this.dataset.Dirty().Update(op.OID)
	return nil
}

func (this *Mount) parseDel(op *operator.Operator) error {
	if !this.dataset.Has(op.OID) {
		return ErrItemNotExist(op.OID)
	}
	this.dataset.Delete(op.OID)
	return nil
}

func (this *Mount) parseNew(op *operator.Operator) error {
	items, ok := op.Result.([]any)
	if !ok {
		return ErrArgsIllegal(op.OID, op.Result)
	}
	for _, v := range items {
		if err := this.dataset.Insert(v); err != nil {
			return err
		}
	}
	return nil
}
