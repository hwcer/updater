package updater

import (
	"fmt"

	"github.com/hwcer/cosgo/schema"
	"github.com/hwcer/logger"
	"github.com/hwcer/updater/dataset"
	"github.com/hwcer/updater/hamster"
	"github.com/hwcer/updater/operator"
)

type CollectionModel interface {
	Upsert(update *Updater, op *operator.Operator) bool
	Schema() *schema.Schema
	Getter(update *Updater, data *dataset.Collection, keys []string) error
	Setter(update *Updater, bulkWrite BulkWrite, _id string, dirty dataset.Update, unset []string) error
}

type CollectionModelValueJSName interface {
	GetValueJSName() string //获取value值的jsname
}

type Collection struct {
	updater   *Updater
	statement hamster.Statement //具名字段（非嵌入），见 newStatement
	name      string
	// schema 首次取到后缓存。model 在 NewCollection 之后不再变,其 schema 也就固定,
	// 故无需失效。业务侧的 model.Schema() 常见实现是 `schema.Parse(this)` —— 自己不缓存,
	// 而 format 里每个 Set/Unset operator 都要取一次。
	schema  *schema.Schema
	model   CollectionModel
	remove  []string //需要移除内存的数据,仅仅RAMMaybe有效
	dataset *dataset.Collection
}

func NewCollection(u *Updater, m *Model) Handle {
	r := &Collection{}
	r.name = m.name
	r.model = m.model.(CollectionModel)
	r.updater = u
	r.statement = *newStatement(u, m, r.Has)
	return r
}

// ===================== Handle 接口公开方法 =====================

// Get 返回item,不可叠加道具只能使用oid获取
func (this *Collection) Get(key any) (r any) {
	if doc := this.Document(key); doc != nil {
		r = doc.Any()
	}
	return
}

// Val 点查单个文档的 val 值,不可叠加道具只能使用oid获取(传 iid 返回 0)
// 求某个 iid 的持有总量用 Count,不要用 Val
func (this *Collection) Val(key any) (r int64) {
	if oid, err := this.GetOID(key); err == nil {
		r, _ = this.val(oid)
	}
	return
}

func (this *Collection) Data() (err error) {
	if this.updater.Error != nil {
		return this.updater.Error
	}
	if len(this.statement.Keys()) == 0 {
		return nil
	}
	keys := this.statement.Keys().ToString()
	if err = this.model.Getter(this.updater, this.dataset, keys); err == nil {
		this.statement.Date()
	}
	return
}

// Count 按 iid 汇总的持有总量,区别于 Val 的点查单个文档
// 可叠加道具一个 iid 只有一个文档,直接取其 val 值(等同 Val);
// 不可叠加道具(装备)一件一个文档、无法由 iid 定位,须扫描 dataset 按 iid 统计文档数,
// 统计含本次请求内尚未落库的新增(见 dataset.Collection.Count),代价 O(n)
// 取 iid 优先用 dataset.Model.GetIID(),模型未实现时才按 Fields.IID 字段名兜底
func (this *Collection) Count(iid int32) int64 {
	it := this.ITypeCollection(iid)
	if it == nil {
		return 0
	}
	if it.Stacked(iid) {
		return this.Val(iid)
	}
	return this.dataset.Count(func(doc *dataset.Document) bool {
		return docIID(doc) == iid
	})
}

// docIID 取文档的 iid
// 优先走 dataset.Model 接口(编译期约束),模型未实现时回落到 Fields.IID 字段名约定;
// 两者都取不到时返回 0,该文档不会被 Count 计入
func docIID(doc *dataset.Document) int32 {
	if m, ok := doc.Any().(dataset.Model); ok {
		return m.GetIID()
	}
	return doc.GetInt32(dataset.Fields.IID)
}

func (this *Collection) IMax(iid int32) int64 {
	return modelIMax(this.model, iid)
}

func (this *Collection) IType(iid int32) IType {
	return modelIType(this.model, iid)
}

func (this *Collection) Select(keys ...any) {
	for _, k := range keys {
		if oid, err := this.GetOID(k); err == nil {
			this.statement.Select(oid)
		} else {
			logger.Alert(err)
		}
	}
}

func (this *Collection) Parser() Parser {
	return ParserTypeCollection
}

// ===================== Handle 接口生命周期方法 =====================

func (this *Collection) increase(id int32, v int64) {
	field := this.Field()
	this.operator(operator.TypesAdd, id, field, v, nil)
}
func (this *Collection) decrease(id int32, v int64) {
	field := this.Field()
	this.operator(operator.TypesSub, id, field, v, nil)
}

func (this *Collection) Save() (err error) {
	if this.updater.BulkWrite() == nil {
		return ErrBulkWriteNotInit
	}
	return this.dataset.Save(newCollectionBulkWrite(this.updater, this.model))
}

func (this *Collection) Reset() {
	this.statement.Reset()
	if this.dataset == nil {
		this.dataset = dataset.NewColl()
	}
	if reset, ok := this.model.(ModelReset); ok {
		if reset.Reset(this.updater, this.updater.Last()) {
			this.updater.Error = this.Reload()
		}
	}
}

func (this *Collection) Reload() error {
	this.dataset = nil
	this.statement.Reload()
	return this.Loading()
}

func (this *Collection) Loading() error {
	if this.dataset == nil {
		this.dataset = dataset.NewColl()
	}
	if this.statement.Loading() {
		this.updater.Error = this.model.Getter(this.updater, this.dataset, nil)
		if this.updater.Error == nil {
			this.statement.SetLoaded(true)
		}
	}
	return this.updater.Error
}

func (this *Collection) Release() {
	this.statement.Release()
	this.remove = nil
	if this.statement.RAM() == RAMTypeNone {
		this.dataset = nil
	} else {
		this.dataset.Release()
	}
}

func (this *Collection) Destroy() (err error) {
	return this.Save()
}

func (this *Collection) Commit() (err error) {
	if err = this.updater.WriteAble(); err != nil {
		return
	}
	this.statement.Submit()
	if err = this.Save(); err != nil && this.statement.RAM() != RAMTypeNone {
		logger.Alert("同步数据失败,等待下次同步:%v", err)
		err = nil
	}
	if len(this.remove) > 0 {
		this.dataset.Remove(this.remove...)
		this.remove = nil
	}
	return
}

func (this *Collection) Verify() (err error) {
	if err = this.updater.WriteAble(); err != nil {
		return
	}
	// 下标遍历(而非 range):Parse 中 overflow→Resolve 可能往本 handle 追加操作
	// (如超 IMax 的道具分解出的产物,与自己同模型/同集合),range 按初始长度迭代会漏掉这些新增 op,
	// 使其被 statement.Verify() 直接搬进 cache 却未 Parse、最终不落库。
	// ⚠️ Ops() 每轮重取：append 扩容后旧切片头看不见新增。
	for i := 0; i < len(this.statement.Ops()); i++ {
		if err = this.Parse(this.statement.Ops()[i]); err != nil {
			return
		}
	}
	this.statement.Verify()
	return
}

// ===================== 类型特有公开方法 =====================

func (this *Collection) Add(id any, value any, field ...string) *operator.Operator {
	key := this.Field(field...)
	return this.operator(operator.TypesAdd, id, key, dataset.ParseInt64(value), nil)
}

func (this *Collection) Sub(id any, value any, field ...string) *operator.Operator {
	key := this.Field(field...)
	return this.operator(operator.TypesSub, id, key, dataset.ParseInt64(value), nil)
}
func (this *Collection) Delete(id any) *operator.Operator {
	return this.operator(operator.TypesDel, id, "", 0, nil)
}

func (this *Collection) Unset(id any, fields ...string) *operator.Operator {
	data := dataset.Update{}
	for _, f := range fields {
		data[f] = nil
	}
	return this.operator(operator.TypesUnset, id, "", 0, data)
}

// Set 设置 k= oid||iid
// Set(oid||iid,map[string]any)
// Set(oid||iid,key string,val any)
func (this *Collection) Set(id any, v ...any) *operator.Operator {
	var data dataset.Update
	switch len(v) {
	case 1:
		if data = dataset.ParseUpdate(v[0]); data == nil {
			this.updater.Error = ErrArgsIllegal(id, v)
		}
	case 2:
		if field, ok := v[0].(string); ok {
			data = dataset.NewUpdate(field, v[1])
		} else {
			this.updater.Error = ErrArgsIllegal(id, v)
		}
	default:
		this.updater.Error = ErrArgsIllegal(id, v)
	}
	if this.updater.Error != nil {
		return nil
	}
	return this.operator(operator.TypesSet, id, "", 0, data)
}

// New 使用全新的模型插入
func (this *Collection) New(v dataset.Model) (err error) {
	n := int64(1)
	if getter, ok := v.(dataset.ModelGet); ok {
		field := this.Field()
		if i, _ := getter.Get(field); i != nil {
			n = dataset.ParseInt64(i)
		}
	}
	op := operator.New(operator.TypesNew, "", n, []any{v})
	op.OID = v.GetOID()
	op.IID = v.GetIID()
	if err = this.mayChange(op); err != nil {
		op.Release()
		return this.updater.Errorf(err)
	}
	this.statement.Insert(op)
	return
}

func (this *Collection) Len() int {
	return this.dataset.Len()
}

func (this *Collection) Has(id any) (r bool) {
	if oid, err := this.GetOID(id); err == nil {
		r = this.dataset.Has(oid)
	} else {
		logger.Debug(err)
	}
	return
}

func (this *Collection) Range(h func(id string, doc *dataset.Document) bool) {
	this.dataset.Range(h)
}

func (this *Collection) Cursor(key string) *dataset.Cursor {
	return this.dataset.Cursor(key)
}

// Remove 从内存中移除，用于清理不常用数据，不会改变数据库
func (this *Collection) Remove(id ...string) {
	this.remove = append(this.remove, id...)
}

// Receive 把**已经在手上的**文档直接塞进内存，不查库、不写库 —— Remove 的反向操作。
//
// 用在"业务别处已经查出这批数据、或本次请求刚亲手插入过"的场合：不塞的话
// Select + Data 会照着 oid 把同一条再查一遍。
//
// ⚠️ 塞进来的必须是**库里真实存在**（或本次请求正在插入）的那条，且 oid 对得上 ——
// 凭空造一条塞进来，后续对它的 Add/Set 会生成一条指向不存在记录的更新。框架不校验。
// ⚠️ 只进内存、**不记脏、不落库**；要落库仍然走 New/Add/Set。
// ⚠️ 塞进来之后 Select 会认为它已在内存而跳过，也就是说**后续不会再从库里刷新它**。
func (this *Collection) Receive(oid string, data any) {
	this.dataset.Receive(oid, data)
}

func (this *Collection) Field(field ...string) string {
	if len(field) > 0 {
		return field[0]
	}
	if f, ok := this.model.(CollectionModelValueJSName); ok {
		return f.GetValueJSName()
	}
	return dataset.Fields.VAL
}

func (this *Collection) Schema() *schema.Schema {
	if this.schema == nil {
		this.schema = this.model.Schema()
	}
	return this.schema
}

// Monitors 数据集变更观察者注册表，注册/注销走返回值上的方法：
//
//	coll.Monitors().Set("items", &itemsIndexesMonitor{...})
func (this *Collection) Monitors() *dataset.Monitors {
	return this.dataset.Monitors()
}

// ITypeCollection 返回 ITypeCollection 以访问 New/Stacked/ObjectId 等方法
func (this *Collection) ITypeCollection(iid int32) ITypeCollection {
	it := this.IType(iid)
	if it == nil {
		return nil
	}
	r, _ := it.(ITypeCollection)
	return r
}

func (this *Collection) GetOID(key any) (oid string, err error) {
	if v, ok := key.(string); ok {
		return v, nil
	}
	iid := dataset.ParseInt32(key)
	it := this.ITypeCollection(iid)
	if it == nil {
		return "", fmt.Errorf("IType unknown:%v", iid)
	}
	if !it.Stacked(iid) {
		return "", ErrObjectIdEmpty(iid)
	}
	if oid = it.GetOID(this.updater, iid); oid == "" {
		err = ErrUnableUseIIDOperation
	}
	return
}

func (this *Collection) Insert(op *operator.Operator, before ...bool) {
	this.format(op)
	this.statement.Insert(op, before...)
}

func (this *Collection) Dataset() *dataset.Collection {
	return this.dataset
}

func (this *Collection) Document(key any) (r *dataset.Document) {
	if oid, err := this.GetOID(key); err == nil {
		r = this.dataset.Val(oid)
	} else {
		logger.Debug(err)
	}
	return
}

// ===================== 类型特有私有方法 =====================

func (this *Collection) val(id string) (r int64, ok bool) {
	var i *dataset.Document
	if i, ok = this.dataset.Get(id); ok {
		k := this.Field()
		r = i.GetInt64(k)
	}
	return
}

// operator 封装 Operator，k oid||iid
func (this *Collection) operator(t operator.Types, id any, k string, v int64, r any) *operator.Operator {
	if err := this.updater.WriteAble(); err != nil {
		return nil
	}
	if v <= 0 && (t == operator.TypesAdd || t == operator.TypesSub) {
		return nil
	}

	op := operator.New(t, k, v, r)
	switch d := id.(type) {
	case string:
		op.OID = d
		op.IID, this.updater.Error = Config.ParseId(this.updater, op.OID)
	default:
		op.IID = dataset.ParseInt32(id)
	}

	if this.updater.Error != nil {
		op.Release()
		return nil
	}
	if this.updater.Error = this.mayChange(op); this.updater.Error != nil {
		op.Release()
		return nil
	}
	this.format(op)
	this.statement.Insert(op)
	return op
}

func (this *Collection) mayChange(op *operator.Operator) (err error) {
	it := this.ITypeCollection(op.IID)
	if it == nil {
		return ErrITypeNotExist(op.IID)
	}
	op.IType = it.ID()
	if listen, ok := it.(ITypeListener); ok {
		listen.Listener(this.updater, op)
	}
	if op.OType == operator.TypesDrop || op.OType == operator.TypesResolve {
		return nil
	}
	if op.OID == "" && it.Stacked(op.IID) {
		op.OID = it.GetOID(this.updater, op.IID)
	}
	if op.OID != "" {
		this.statement.Select(op.OID)
	}
	return
}

func (this *Collection) format(op *operator.Operator) {
	if op.OType != operator.TypesSet && op.OType != operator.TypesUnset {
		return
	}
	data := dataset.Update{}
	result, ok := op.Result.(dataset.Update)
	if !ok {
		this.updater.Error = fmt.Errorf("operator.set return error name:%s  result:%v", this.name, op.Result)
		return
	}
	sch := this.Schema()
	if sch == nil {
		this.updater.Error = fmt.Errorf("operator.set schema empty:%s", this.name)
		return
	}
	//统一成 json 名,与 Document.Field 同口径(理由见 Document.Name):op.Result 既是发
	//客户端的 payload,又经 dataset 进 dirty 落库;客户端那侧直通,落库那侧由 cosmo 的
	//Update.Transform 在边界换成 DBName。
	//
	//🔴 含 "." 的多级路径：JSName 自己会逐段走查(字段段换名、map 键与下标原样保留)。
	for k, v := range result {
		name, err := sch.JSName(k)
		if err != nil {
			this.updater.Error = fmt.Errorf("operator.set field error,name:%s,field:%s,error:%v", this.name, k, err)
			return
		}
		data[name] = v
	}
	op.Result = data
}

// ===================== CollectionBulkWrite =====================

// newCollectionBulkWrite 构造 dataset.CollectionWriter 适配器。
// hamster.NewCollectionBulkWrite 是共享实现（核心版句柄与扩展层共用），
// 这里把扩展层的 CollectionModel.Setter（首参 *Updater）适配成闭包传入。
// **Collection 与 Mount 共用这一个**。
//
// 为什么必须有这层适配、不能直接把 Updater.BulkWrite() 交给 dataset.Save：
//   - 两者不是一套接口 —— CollectionWriter 的 Delete/Insert 不带 model 参数（由适配器
//     绑定），还多一个 BulkWrite 没有的 Setter；
//   - Handle 自己也实现不了 CollectionWriter —— Collection.Delete(id any) 与接口要求的
//     Delete(where ...any) 重名不同签，一个类型上放不下。
func newCollectionBulkWrite(u *Updater, model CollectionModel, bulk ...BulkWrite) *hamster.CollectionBulkWrite {
	setter := func(bw BulkWrite, _id string, dirty dataset.Update, unset []string) error {
		return model.Setter(u, bw, _id, dirty, unset)
	}
	return hamster.NewCollectionBulkWrite(u.store, model, setter, bulk...)
}
