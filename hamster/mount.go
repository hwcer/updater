package hamster

import (
	"fmt"

	"github.com/hwcer/cosgo/schema"
	"github.com/hwcer/logger"
	"github.com/hwcer/updater/dataset"
	"github.com/hwcer/updater/operator"
)

// 临时句柄挂载：为"Store 之外、但要与属主数据同批次原子写库"的数据准备
// （邮件领取标记、兑换码占用、充值订单、临时战斗副本）。
//
// 🔴 **独立封装，不内嵌 Collection** —— Go 方法提升没有虚派发，内嵌后
// "覆盖了却不生效""以为继承了其实语义不同"接连出事；挂载真正需要的只有
// Set/Unset/Del/New 四种操作，自己写清楚反而短（主干三稿教训）。
//
// 与 Collection 的分界（主干口径）：
//   - 无 Add/Sub / 无溢出检查 / 不做跨天重置 / 缺文档直接报错（不咨询 Upsert）
//   - key 只能是文档 _id（string），parseDel 不戳删除数值
//   - RAMType 强制 Maybe、默认 Discard 接收器（operator IType 恒 0，默认不下发）

// MountModel 临时数据模型：组合 schema.Tabler 是必需的（挂载名取自 TableName()）。
// ⚠️ Getter 只会收到**非空**的 keys —— 挂载是按需加载的，从不做全量拉取。
type MountModel interface {
	CollectionModel
	schema.Tabler
}

// Mount 临时挂载集合：与属主数据同批次原子写库。
//
// 🔴 走的是与 Collection 相同的那条流水线：改动先变成 operator 入队，
// verify 阶段消费（写进内存并记脏），commit 阶段经 model.Setter 落进共享 bulkWrite。
// 由此改数据一律经 operator，请求失败时 release 自动丢弃，内存不会留下库里没有的状态
// —— 这就是「取到的指针一律只读」在挂载上的落点。
type Mount struct {
	statement Statement
	name      string
	// schema 首次取到后缓存。model 在构造之后不再变，其 schema 也就固定
	//（业务侧常见实现是 schema.Parse(this)，自己不缓存，而 format 里每个操作都要取一次）
	schema  *schema.Schema
	model   MountModel
	remove  []string //待从内存移除的 _id，submit 时统一处理（落库之后再摘，别丢掉未保存的改动）
	dataset *dataset.Collection
	unmount bool //已标记卸载，Release 阶段才真正摘除，见 Unmount
}

func newMount(s *Store, m MountModel) *Mount {
	r := &Mount{name: m.TableName(), model: m, dataset: dataset.NewColl()}
	//ram 强制 RAMTypeMaybe：只影响 statement.Has 里 `Always && loader` 那条短路，
	//绝不能命中——命中之后 Select 会跳过每一个 key，Data 永不执行、Get 全 nil 且不报错。
	r.statement = *NewStatement(s, RAMTypeMaybe, r.exist)
	//核心版产出的 operator IType 恒 0，客户端认不出无主数据，默认别进通用更新通道。
	//要转发客户端的挂载由封装层装带盖键的接收器（见 updater.Mount）。
	r.statement.Receiver(DiscardReceiver)
	return r
}

// Receiver 装载变更接收器（默认 DiscardReceiver）；nil 恢复默认。
// 扩展层在此接管，给 operator 盖上 IType 并转进变更流水（见 updater.Mount）。
func (this *Mount) Receiver(f func(*Store, []*operator.Operator)) {
	if f == nil {
		this.statement.Receiver(DiscardReceiver)
		return
	}
	this.statement.Receiver(f)
}

// ===================== 操作入口（全部产 operator，不直接改内存）=====================

// operator 构造并入队一条 Operator。
//
// 🔴 与 Collection.operator 的分界：没有 Config.ParseId / mayChange ——
// 挂载的 _id 是业务自己的主键（uid-code、平台订单号…），IID 恒 0、IType 恒 0。
func (this *Mount) operator(t operator.Types, id string, v int64, r any) *operator.Operator {
	if err := this.statement.Store.WriteAble(); err != nil {
		return nil
	}
	if id == "" {
		this.statement.Store.Error = ErrObjectIdEmpty(t.ToString())
		return nil
	}
	op := operator.New(t, "", v, r)
	op.OID = id
	this.format(op)
	if this.statement.Store.Error != nil {
		op.Release()
		return nil
	}
	this.statement.Insert(op)
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
	//Value 取对象上的数值字段（字段名由 Field() 定），对象上没有这个字段才回落 1。
	n := int64(1)
	if i, ok := doc.Get(this.Field()); ok && i != nil {
		n = dataset.ParseInt64(i)
	}
	return this.operator(operator.TypesNew, doc.GetString(dataset.Fields.OID), n, []any{v})
}

// Operators 本次请求**已通过 verify、尚未 submit** 的 operator 列表（只读）。
//
// 🔴 它读的就是 statement.cache：verify 之后、submit 之前有效。
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
// 🔴 GetValueJSName 返回空串视作未声明（扩展层适配器恒实现该接口、缺省时返回 ""），
// 与 Collection.Field 同口径，别让空串截断回落链。
func (this *Mount) Field(field ...string) string {
	if len(field) > 0 {
		return field[0]
	}
	if f, ok := this.model.(ValueJSName); ok {
		if name := f.GetValueJSName(); name != "" {
			return name
		}
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

// Receive 把**已经在手上的**文档直接塞进内存，跳过 Select + Data 那次查库。
//
// 挂载不只是"把写操作并进事务"，它同时是这次会话里的一份**缓存**。
// ⚠️ 塞进来的对象必须是这张表的模型、且它的 _id 与 id 一致，框架不校验。
// ⚠️ 只进内存、**不记脏、不会被写库**。要落库仍然走 Insert / Update。
// ⚠️ 塞进来之后 Select 会认为这条已在内存而跳过，也就是说**后续不会再从库里刷新它**。
func (this *Mount) Receive(id string, data any) {
	this.dataset.Receive(id, data)
}

// Submit 把**本挂载**的改动单独落库，不等 Store 整体提交。
//
// 给「这一趟末尾要 return error、但这份数据必须留下」的场合用：平台回来的订单状态、
// 三方结算回执这类**已经发生的事实**，不该跟着业务失败一起回滚。
//
// 🔴 用一份**独立的 BulkWrite**，不碰 Store 那份共享实例：
// 共享那份里装着属主数据的改动，提交它等于把整个请求提前落库，那不叫单表提交。
//
// ⚠️ 提交之后这个挂载会处于「已落库」状态，而请求可能还会失败回滚。三条后果要清楚：
//
//	内存    已是新值(verify 时写的)，与库一致 —— 这正是要的
//	客户端  operator 还在 cache 里等 Store.Submit 交付；请求失败就不会下发。
//	属主数据 完全不受影响，该回滚照样回滚
//
// ⚠️ **失败时内存已经是新值、库还是旧的**（verify 在落库之前）。别当没事发生：
// 要么重试，要么 Unmount 整张表（Release 时摘除，下次请求重新从库加载）。
//
// ⚠️ 全局的 StatusOperated **不会**被清除：它是所有 handle 共用的一个标志，
// 为了"我这张表校验过了"去清它，会让其它 handle 的待校验操作被整体跳过。
// 留着的代价只是后续 converge 对本挂载空转一次（statement.Verify 消费完已把队列置 nil）。
func (this *Mount) Submit() error {
	if err := this.statement.Store.WriteAble(); err != nil {
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
	bulk := Config.BulkWrite(this.statement.Store)
	if bulk == nil {
		return ErrBulkWriteNotInit
	}
	if err := this.dataset.Save(this.bulkWriter(bulk)); err != nil {
		return err
	}
	//测试模式只改内存不写库，与 Store.Submit 的口径一致:数据已经进了这份 bulk，
	//丢掉不提交即可。
	if this.statement.Store.status.Has(StatusTesting) {
		return nil
	}
	if err := bulk.Submit(); err != nil {
		return err
	}
	//落库了才摘，与 commit 同一条规矩(见 Remove)
	if len(this.remove) > 0 {
		this.dataset.Remove(this.remove...)
		this.remove = nil
	}
	return nil
}

// ===================== Handle 接口公开方法 =====================

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
// ⚠️ 只有 Store.Data()/Submit() 会驱动它，而 Store.data() 开头有
// `if !status.Has(StatusChanged) { return }` 的闸门 —— 该位由 Select 置起。
func (this *Mount) Data() (err error) {
	if err = this.statement.Store.Error; err != nil {
		return
	}
	if len(this.statement.keys) == 0 {
		return nil
	}
	if err = this.model.Getter(this.statement.Store, this.dataset, this.statement.keys.ToString()); err == nil {
		this.statement.Date() //keys = nil
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

// Select 标记待拉取的 _id，随后由 Store.Data() 统一查库。
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

// ===================== Handle 接口生命周期方法（未导出，Store 驱动） =====================

// verify 消费待处理的 operator：写进内存并记脏。
func (this *Mount) verify() (err error) {
	if err = this.statement.Store.WriteAble(); err != nil {
		return
	}
	//下标遍历而非 range：当前四个 parse 分支都不会往队列追加，
	//但那是实现的性质、不是接口保证 —— range 按初始长度迭代，
	//哪天有分支开始追加就会静默漏掉新增的那几条（Collection 那边正是为此踩过）。
	for i := 0; i < len(this.statement.operator); i++ {
		if err = this.parse(this.statement.operator[i]); err != nil {
			return
		}
	}
	this.statement.Verify()
	return
}

// commit 与属主数据同批次：都往共享 bulkWrite 里写，由 Store.Submit 末尾一次提交。
//
// ⚠️ save 失败**只告警不返回**：走到这一步内存已经改完了
// （dataset.Save 里 doc.Save() 排在 Setter 之前），返回错误既回滚不了内存，
// 还会连累 bulkWrite 整个不提交，分歧只会更大。要的是最终一致；
// 真正的同批次原子保证在这之前（业务错误、verify 失败都拦在 bulkWrite 提交之前）。
func (this *Mount) commit() (err error) {
	if err = this.statement.Store.WriteAble(); err != nil {
		return
	}
	this.statement.Submit()
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
	if this.statement.Store.BulkWrite() == nil {
		return ErrBulkWriteNotInit
	}
	return this.dataset.Save(this.bulkWriter(nil))
}

// bulkWriter 生成 CollectionWriter；bulk 非 nil 时写往独立实例（Submit 单表落库用）
func (this *Mount) bulkWriter(bulk BulkWrite) *CollectionBulkWrite {
	setter := func(bw BulkWrite, _id string, dirty dataset.Update, unset []string) error {
		return this.model.Setter(this.statement.Store, bw, _id, dirty, unset)
	}
	return NewCollectionBulkWrite(this.statement.Store, this.model, setter, bulk)
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
	this.statement.Reload()
	return nil
}

// release 每次请求结束。
//
// ⚠️ 只清 dirty 与待拉取标记，**保留内存数据** —— 长命挂载的跨请求驻留靠这条。
// 想连内存一起丢是 Unmount 的事，两者别混。
func (this *Mount) release() {
	this.statement.Release()
	this.remove = nil
	this.dataset.Release()
}

// destroy 下线：刷盘。
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
		this.statement.Store.Error = fmt.Errorf("mount[%s] operator result must be dataset.Update:%v", this.name, op.Result)
		return
	}
	sch := this.Schema()
	if sch == nil {
		this.statement.Store.Error = fmt.Errorf("mount[%s] schema empty", this.name)
		return
	}
	data := dataset.Update{}
	for k, v := range result {
		name, err := sch.JSName(k)
		if err != nil {
			this.statement.Store.Error = fmt.Errorf("mount[%s] field error,field:%s,error:%v", this.name, k, err)
			return
		}
		data[name] = v
	}
	op.Result = data
}

// parse 挂载只认这四种操作：没有 Add/Sub（那是按 iid 增减持有量，挂载没有 iid），
// 没有溢出检查/转化（核心溢出控制不适用于挂载语义）。
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
