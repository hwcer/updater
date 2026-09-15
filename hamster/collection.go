package hamster

import (
	"fmt"

	"github.com/hwcer/cosgo/schema"
	"github.com/hwcer/logger"
	"github.com/hwcer/updater/dataset"
	"github.com/hwcer/updater/operator"
)

// CollectionModel 核心版集合模型接口（纯存储，无任何道具概念）。
// 与扩展层 updater.CollectionModel 的差异仅是首参 *Store。
type CollectionModel interface {
	Upsert(s *Store, op *operator.Operator) bool
	Schema() *schema.Schema
	Getter(s *Store, data *dataset.Collection, keys []string) error
	Setter(s *Store, bw BulkWrite, _id string, dirty dataset.Update, unset []string) error
}

// ValueJSName 可选接口：声明数值字段名
type ValueJSName interface {
	GetValueJSName() string
}

// Collection 文档集合存储（核心版纯净实现）。
//
// 蓝本是 handle_mount.go 的 Mount（"无 IType Collection"的全尺度原型）：
// string 主键、无 ParseId/mayChange、operator 不设 IID、IType 恒 0。
// 比 Mount 多两件事：字段级 Add/Sub（公会资金/成员贡献够用，无溢出检查），
// 以及可经 RegisterCollection 进全局注册表（Mount 是它的临时形态）。
type Collection struct {
	Statement
	name string
	// schema 首次取到后缓存。model 在构造之后不再变，其 schema 也就固定
	schema  *schema.Schema
	model   CollectionModel
	remove  []string //待从内存移除的 _id，submit 时统一处理（落库之后再摘，别丢掉未保存的改动）
	dataset *dataset.Collection
	unmount bool //已标记卸载，Release 阶段才真正摘除（仅挂载形态使用）
	ext     any  //扩展层挂载的包装对象槽（hamster 不解读），保证包装句柄指针同一
	// itype 每条 operator 的客户端分发键。恒 0 = 无主数据（默认 Discard 接收器，不进通用更新）；
	// 非 0 时新产出的 operator 携带该键（语义同拆分前 Mount.itype，见 HAMSTER_PLAN.md 第八节）。
	itype int32
}

// SetIType 声明客户端分发键（显式opt-in；0 恢复无主数据）。
// 只管给 operator 盖键；要不要进通用更新通道由调用方自行装接收器。
func (c *Collection) SetIType(n int32) { c.itype = n }

// Ext 取扩展层挂载的包装对象
func (c *Collection) Ext() any { return c.ext }

// SetExt 存扩展层挂载的包装对象
func (c *Collection) SetExt(v any) { c.ext = v }

func newCollection(s *Store, m *Model) Handle {
	r := &Collection{}
	r.name = m.name
	r.model = m.model.(CollectionModel)
	r.Statement = *NewStatement(s, m.ram, r.exist)
	//核心版产出的 operator IType 恒 0，客户端认不出无主数据，默认别进通用更新通道。
	//要变更记录的业务装自己的接收器，或用 Operators()/Submit() 返回值自组协议。
	r.Statement.Receiver(DiscardReceiver)
	return r
}

// ===================== 操作入口（全部产 operator，不直接改内存）=====================

// operator 构造并入队一条 Operator。
//
// 🔴 与扩展层 Collection.operator 的分界在这里：那边对 string 型 id 会调 Config.ParseId
// 解析 iid —— 本集合的 _id 是业务自己的主键（uid-code、平台订单号…），不是项目的 OID 格式；
// IID 恒 0，IType 恒 0。
func (this *Collection) operator(t operator.Types, id string, field string, v int64, r any) *operator.Operator {
	if err := this.Store.WriteAble(); err != nil {
		return nil
	}
	if id == "" {
		this.Store.Error = ErrObjectIdEmpty(t.ToString())
		return nil
	}
	if v <= 0 && (t == operator.TypesAdd || t == operator.TypesSub) {
		return nil
	}
	op := operator.New(t, field, v, r)
	op.OID = id
	op.IType = this.itype //客户端分发键：0 即无主数据，对面按 IType 分发认不出
	this.format(op)
	if this.Store.Error != nil {
		op.Release()
		return nil
	}
	this.Statement.Insert(op)
	return op
}

// Update 批量改字段。只是入队，verify 阶段才真正写进内存。
func (this *Collection) Update(id string, data dataset.Update) *operator.Operator {
	return this.operator(operator.TypesSet, id, "", 0, data)
}

// Set 改单个字段，语义同 Update。
func (this *Collection) Set(id string, field string, value any) *operator.Operator {
	return this.Update(id, dataset.NewUpdate(field, value))
}

// Unset 删字段。
func (this *Collection) Unset(id string, fields ...string) *operator.Operator {
	data := dataset.Update{}
	for _, f := range fields {
		data[f] = nil
	}
	return this.operator(operator.TypesUnset, id, "", 0, data)
}

// Delete 删文档。
func (this *Collection) Delete(id string) *operator.Operator {
	return this.operator(operator.TypesDel, id, "", 0, nil)
}

// Insert 插入新文档，_id 从对象上取，取不到即报错。
//
// ⚠️ **不要另给一个 id 参数**：真正决定落库主键的是对象自己的 _id。
func (this *Collection) Insert(v any) *operator.Operator {
	doc := dataset.NewDoc(v)
	n := int64(1)
	if i, ok := doc.Get(this.Field()); ok && i != nil {
		n = dataset.ParseInt64(i)
	}
	return this.operator(operator.TypesNew, doc.GetString(dataset.Fields.OID), "", n, []any{v})
}

// New 使用模型对象插入（v 须实现 dataset.Model 提供主键）
func (this *Collection) New(v dataset.Model) error {
	n := int64(1)
	if getter, ok := v.(dataset.ModelGet); ok {
		if i, _ := getter.Get(this.Field()); i != nil {
			n = dataset.ParseInt64(i)
		}
	}
	op := operator.New(operator.TypesNew, "", n, []any{v})
	op.OID = v.GetOID()
	this.Statement.Insert(op)
	return nil
}

// Add 字段级数值增（OID+field 定位，无溢出检查 —— 公会资金/成员贡献够用）。
func (this *Collection) Add(id string, field string, v any) *operator.Operator {
	return this.operator(operator.TypesAdd, id, field, dataset.ParseInt64(v), nil)
}

// Sub 字段级数值减。余额不足时打脏 Error（CreditAllowed 放行扣负）。
func (this *Collection) Sub(id string, field string, v any) *operator.Operator {
	return this.operator(operator.TypesSub, id, field, dataset.ParseInt64(v), nil)
}

// Operators 本次请求**已通过 verify、尚未 submit** 的 operator 列表（只读）。
//
// 🔴 它读的就是 statement.cache：verify 之后、submit 之前有效。
func (this *Collection) Operators() []*operator.Operator {
	return this.Statement.cache
}

// ===================== 读取 =====================

func (this *Collection) Name() string {
	return this.name
}

func (this *Collection) Schema() *schema.Schema {
	if this.schema == nil {
		this.schema = this.model.Schema()
	}
	return this.schema
}

// Field 解析数值字段名：传参优先，其次模型实现的 GetValueJSName()，最后 dataset.Fields.VAL。
func (this *Collection) Field(field ...string) string {
	if len(field) > 0 {
		return field[0]
	}
	if f, ok := this.model.(ValueJSName); ok {
		return f.GetValueJSName()
	}
	return dataset.Fields.VAL
}

// Document 取文档，不存在返回 nil。
func (this *Collection) Document(id string) *dataset.Document {
	return this.dataset.Val(id)
}

func (this *Collection) Has(id string) bool {
	return this.dataset.Has(id)
}

func (this *Collection) Len() int {
	return this.dataset.Len()
}

func (this *Collection) Range(handle func(string, *dataset.Document) bool) {
	this.dataset.Range(handle)
}

func (this *Collection) Cursor(key string) *dataset.Cursor {
	return this.dataset.Cursor(key)
}

// Monitors 数据集变更观察者注册表
func (this *Collection) Monitors() *dataset.Monitors {
	return this.dataset.Monitors()
}

// Remove 仅从内存移除，不动数据库。**submit 落库之后才真正摘除**。
func (this *Collection) Remove(id ...string) {
	this.remove = append(this.remove, id...)
}

// Receive 把**已经在手上的**文档直接塞进内存，跳过 Select + Data 那次查库。
// ⚠️ 只进内存、**不记脏、不会被写库**；塞进来之后 Select 会跳过这条（后续不再从库里刷新）。
func (this *Collection) Receive(id string, data any) {
	this.dataset.Receive(id, data)
}

// Submit 把**本集合**的改动单独落库，不等 Store 整体提交。
//
// 给「这一趟末尾要 return error、但这份数据必须留下」的场合用。
// 🔴 用一份**独立的 BulkWrite**，不碰 Store 那份共享实例。
//
// ⚠️ 失败时内存已经是新值、库还是旧的（verify 在落库之前）。要么重试，要么整表卸载。
// ⚠️ 全局的 StatusOperated **不会**被清除：它是所有 handle 共用的标志，
// 为了"我这张表校验过了"去清它，会让其它 handle 的待校验操作被整体跳过。
func (this *Collection) Submit() error {
	if err := this.Store.WriteAble(); err != nil {
		return err
	}
	if err := this.Verify(); err != nil {
		return err
	}
	if len(this.dataset.Dirty()) == 0 {
		return nil
	}
	if Config.BulkWrite == nil {
		return ErrBulkWriteNotInit
	}
	bulk := Config.BulkWrite(this.Store)
	if bulk == nil {
		return ErrBulkWriteNotInit
	}
	if err := this.dataset.Save(this.bulkWriter(bulk)); err != nil {
		return err
	}
	//测试模式只改内存不写库:数据已经进了这份 bulk，丢掉不提交即可。
	if this.Store.status.Has(StatusTesting) {
		return nil
	}
	if err := bulk.Submit(); err != nil {
		return err
	}
	//落库了才摘，与 Commit 同一条规矩(见 Remove)
	if len(this.remove) > 0 {
		this.dataset.Remove(this.remove...)
		this.remove = nil
	}
	return nil
}

// ===================== Handle 接口公开方法 =====================

// Get 取文档原始对象（model.Getter 塞进来的那个），key 必须是文档 _id(string)。
//
// 🔴 **拿到的是指针，只读**。直接改它上面的字段不记脏，改动只留在内存里。
// 要改数据一律走 Update / Set。
func (this *Collection) Get(key any) (r any) {
	if doc := this.document(key); doc != nil {
		r = doc.Any()
	}
	return
}

// Val 取文档上数值字段的值，字段名由 Field() 决定。
// 要读别的字段走 Document：`coll.Document(id).GetInt64("xxx")`。
func (this *Collection) Val(key any) (r int64) {
	if doc := this.document(key); doc != nil {
		r = doc.GetInt64(this.Field())
	}
	return
}

// Data 拉取 Select 标记的文档。keys 为空时不查库。
func (this *Collection) Data() (err error) {
	if err = this.Store.Error; err != nil {
		return
	}
	if len(this.keys) == 0 {
		return nil
	}
	if err = this.model.Getter(this.Store, this.dataset, this.keys.ToString()); err == nil {
		this.Statement.Date()
	}
	return
}

// Select 标记待拉取的 _id，随后由 Store.Data() 统一查库。
// 已在内存中的 key 直接跳过，不重复查。
func (this *Collection) Select(keys ...any) {
	for _, k := range keys {
		if id, ok := k.(string); ok {
			this.Statement.Select(id)
		} else {
			logger.Alert("hamster.Collection(%v).Select key 必须是文档 _id(string):%v", this.name, k)
		}
	}
}

func (this *Collection) Parser() Parser {
	return ParserTypeCollection
}

// ===================== Handle 接口生命周期方法 =====================

// Verify 消费待处理的 operator：写进内存并记脏。
func (this *Collection) Verify() (err error) {
	if err = this.Store.WriteAble(); err != nil {
		return
	}
	//下标遍历而非 range：当前 parse 分支都不会往队列追加，但那是实现的性质、不是接口保证。
	//🔴 operator 字段被同名方法遮蔽，必须用 this.Statement.operator 限定。
	for i := 0; i < len(this.Statement.operator); i++ {
		if err = this.parse(this.Statement.operator[i]); err != nil {
			return
		}
	}
	this.Statement.Verify()
	return
}

// Commit 与 Store 同批次：都往共享 bulkWrite 里写，由 Store.Commit 末尾一次提交。
//
// ⚠️ save 失败**只告警不返回**：走到这一步内存已经改完了，返回错误既回滚不了内存，
// 还会连累 bulkWrite 整个不提交。要的是最终一致；真正的同批次原子保证在这之前。
func (this *Collection) Commit() (err error) {
	if err = this.Store.WriteAble(); err != nil {
		return
	}
	this.Statement.Submit()
	if err = this.Save(); err != nil {
		logger.Alert("集合同步数据失败,name:%v,err:%v", this.name, err)
		err = nil
	}
	if len(this.remove) > 0 {
		this.dataset.Remove(this.remove...)
		this.remove = nil
	}
	return
}

// Save 把脏数据经 model.Setter 写进共享 bulkWrite（此时尚未提交）。
func (this *Collection) Save() error {
	if this.Store.BulkWrite() == nil {
		return ErrBulkWriteNotInit
	}
	return this.dataset.Save(this.bulkWriter(nil))
}

// bulkWriter 生成 CollectionWriter；bulk 非 nil 时写往独立实例（Submit 单表落库用）
func (this *Collection) bulkWriter(bulk BulkWrite) *CollectionBulkWrite {
	setter := func(bw BulkWrite, _id string, dirty dataset.Update, unset []string) error {
		return this.model.Setter(this.Store, bw, _id, dirty, unset)
	}
	return NewCollectionBulkWrite(this.Store, this.model, setter, bulk)
}

// Reset 每次请求开始。⚠️ 不做跨天重置（那是扩展层 ModelReset 的职责域）。
func (this *Collection) Reset() {
	this.Statement.Reset()
	if this.dataset == nil {
		this.dataset = dataset.NewColl()
	}
}

// Loading 一律惰性，不预热 —— 挂载/按需集合从不做全量拉取。
func (this *Collection) Loading() error {
	if this.dataset == nil {
		this.dataset = dataset.NewColl()
	}
	return this.Store.Error
}

// Reload 丢弃内存，下次 Select+Data 重新查库。
func (this *Collection) Reload() error {
	this.dataset = dataset.NewColl()
	this.Statement.Reload()
	return nil
}

// Release 每次请求结束。⚠️ 只清 dirty 与待拉取标记，**保留内存数据**（跨请求驻留靠这条）。
// 想连内存一起丢是 Unmount 的事，两者别混。
func (this *Collection) Release() {
	this.Statement.Release()
	this.remove = nil
	this.dataset.Release()
}

// Destroy 下线：刷盘。
func (this *Collection) Destroy() error {
	return this.Save()
}

// ===================== 内部 =====================

// exist 交给 statement 判断 key 是否已在内存（Select 去重用）。
func (this *Collection) exist(k any) bool {
	id, ok := k.(string)
	return ok && this.dataset.Has(id)
}

// document Get/Val 用：key 必须是文档 _id，非字符串记一条告警后当作查不到。
func (this *Collection) document(key any) *dataset.Document {
	id, ok := key.(string)
	if !ok {
		logger.Alert("hamster.Collection(%v) key 必须是文档 _id(string):%v", this.name, key)
		return nil
	}
	return this.dataset.Val(id)
}

// format 把 op.Result 里的字段名统一成 JSName：
// op.Result 既是发客户端的 payload、又经 dataset 进 dirty 落库，
// 落库那侧由 cosmo 的 Update.Transform 在边界换成 DBName。
func (this *Collection) format(op *operator.Operator) {
	if op.OType != operator.TypesSet && op.OType != operator.TypesUnset {
		return
	}
	result, ok := op.Result.(dataset.Update)
	if !ok {
		this.Store.Error = fmt.Errorf("collection[%s] operator result must be dataset.Update:%v", this.name, op.Result)
		return
	}
	sch := this.Schema()
	if sch == nil {
		this.Store.Error = fmt.Errorf("collection[%s] schema empty", this.name)
		return
	}
	data := dataset.Update{}
	for k, v := range result {
		name, err := sch.JSName(k)
		if err != nil {
			this.Store.Error = fmt.Errorf("collection[%s] field error,field:%s,error:%v", this.name, k, err)
			return
		}
		data[name] = v
	}
	op.Result = data
}
