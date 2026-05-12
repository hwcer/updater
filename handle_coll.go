package updater

import (
	"fmt"
	"strings"

	"github.com/hwcer/cosgo/schema"
	"github.com/hwcer/logger"
	"github.com/hwcer/updater/dataset"
	"github.com/hwcer/updater/operator"
)

type collectionModel interface {
	IType(iid int32) int32
	Upsert(update *Updater, op *operator.Operator) bool
	Schema() *schema.Schema
	Getter(update *Updater, data *dataset.Collection, keys []string) error //keys==nil 初始化所有
	Setter(update *Updater, bulkWrite dataset.BulkWrite) error
	BulkWrite(update *Updater) dataset.BulkWrite
}

type collectionModelValueJSName interface {
	GetValueJSName() string //获取value值的jsname
}

type Collection struct {
	statement
	name      string
	model     collectionModel
	remove    []string //需要移除内存的数据,仅仅RAMMaybe有效
	dataset   *dataset.Collection
	monitor   dataset.CollectionMonitor //监控数据的insert 和 delete
	bulkWrite dataset.BulkWrite
}

func NewCollection(u *Updater, m *Model) Handle {
	r := &Collection{}
	r.name = m.name
	r.model = m.model.(collectionModel)
	r.statement = *newStatement(u, m, r.Has)
	return r
}

// ===================== Handle 接口公开方法 =====================

// Get 返回item,不可叠加道具只能使用oid获取
func (this *Collection) Get(key any) (r any) {
	if doc := this.Doc(key); doc != nil {
		r = doc.Any()
	}
	return
}

// Val 直接获取 item中的val值,不可叠加道具只能使用oid获取
func (this *Collection) Val(key any) (r int64) {
	if oid, err := this.GetOID(key); err == nil {
		r, _ = this.val(oid)
	}
	return
}

func (this *Collection) Data() (err error) {
	if this.Updater.Error != nil {
		return this.Updater.Error
	}
	if len(this.keys) == 0 {
		return nil
	}
	keys := this.keys.ToString()
	if err = this.model.Getter(this.Updater, this.dataset, keys); err == nil {
		this.statement.date()
	}
	return
}

func (this *Collection) IType(iid int32) IType {
	it := this.model.IType(iid)
	if it == 0 {
		return nil
	}
	return itypesDict[it]
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

// ===================== Handle 接口私有方法 =====================

func (this *Collection) increase(id int32, v int64) {
	field := this.Field()
	this.operator(operator.TypesAdd, id, field, v, nil)
}
func (this *Collection) decrease(id int32, v int64) {
	field := this.Field()
	this.operator(operator.TypesSub, id, field, v, nil)
}

func (this *Collection) save() (err error) {
	bulkWrite := this.BulkWrite()
	if bulkWrite == nil {
		return
	}
	if err = this.dataset.Save(bulkWrite, this.monitor); err != nil {
		return
	}
	if err = this.model.Setter(this.statement.Updater, bulkWrite); err == nil {
		this.bulkWrite = nil
	} else {
		logger.Alert("database save error,uid:%s,Collection:%s\nOperation:%s\nerror:%s", this.Updater.Uid(), this.name, this.bulkWrite.String(), err.Error())
		var s bool
		if s, err = onSaveErrorHandle(this.Updater, err); !s {
			this.bulkWrite = nil
		}
	}
	return
}

func (this *Collection) reset() {
	this.statement.reset()
	if this.dataset == nil {
		this.dataset = dataset.NewColl()
	}
	if reset, ok := this.model.(ModelReset); ok {
		if reset.Reset(this.Updater, this.Updater.last) {
			this.Updater.Error = this.reload()
		}
	}
}

func (this *Collection) reload() error {
	this.dataset = nil
	this.statement.reload()
	return this.loading()
}

func (this *Collection) loading() error {
	if this.dataset == nil {
		this.dataset = dataset.NewColl()
	}
	if this.statement.loading() {
		if this.Updater.Error = this.model.Getter(this.Updater, this.dataset, nil); this.Updater.Error == nil {
			this.statement.loader = true
		}
	}
	return this.Updater.Error
}

func (this *Collection) release() {
	this.statement.release()
	this.remove = nil
	if this.statement.ram == RAMTypeNone {
		this.dataset = nil
	} else {
		this.dataset.Release()
	}
}

func (this *Collection) destroy() (err error) {
	return this.save()
}

func (this *Collection) submit() (err error) {
	if err = this.Updater.WriteAble(); err != nil {
		return
	}
	this.statement.submit()
	if err = this.save(); err != nil && this.ram != RAMTypeNone {
		logger.Alert("同步数据失败,等待下次同步:%v", err)
		err = nil
	}
	if len(this.remove) > 0 {
		this.dataset.Remove(this.remove...)
		this.remove = nil
	}
	return
}

func (this *Collection) verify() (err error) {
	if err = this.Updater.WriteAble(); err != nil {
		return
	}
	for _, act := range this.statement.operator {
		if err = this.Parse(act); err != nil {
			return
		}
	}
	this.statement.verify()
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

// Set 设置 k= oid||iid
// Set(oid||iid,map[string]any)
// Set(oid||iid,key string,val any)
func (this *Collection) Set(id any, v ...any) *operator.Operator {
	var data dataset.Update
	switch len(v) {
	case 1:
		if data = dataset.ParseUpdate(v[0]); data == nil {
			this.Updater.Error = ErrArgsIllegal(id, v)
		}
	case 2:
		if field, ok := v[0].(string); ok {
			data = dataset.NewUpdate(field, v[1])
		} else {
			this.Updater.Error = ErrArgsIllegal(id, v)
		}
	default:
		this.Updater.Error = ErrArgsIllegal(id, v)
	}
	if this.Updater.Error != nil {
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
		return this.Updater.Errorf(err)
	}
	this.statement.insert(op)
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

func (this *Collection) Doc(key any) (r *dataset.Document) {
	if oid, err := this.GetOID(key); err == nil {
		r = this.dataset.Val(oid)
	} else {
		logger.Debug(err)
	}
	return
}

func (this *Collection) Range(h func(id string, doc *dataset.Document) bool) {
	this.dataset.Range(h)
}

// Remove 从内存中移除，用于清理不常用数据，不会改变数据库
func (this *Collection) Remove(id ...string) {
	this.remove = append(this.remove, id...)
}

func (this *Collection) Field(field ...string) string {
	if len(field) > 0 {
		return field[0]
	}
	if f, ok := this.model.(collectionModelValueJSName); ok {
		return f.GetValueJSName()
	}
	return dataset.Fields.VAL
}

func (this *Collection) Schema() *schema.Schema {
	return this.model.Schema()
}

func (this *Collection) SetMonitor(v dataset.CollectionMonitor) {
	this.monitor = v
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

func (this *Collection) BulkWrite() dataset.BulkWrite {
	if this.bulkWrite == nil {
		this.bulkWrite = this.model.BulkWrite(this.Updater)
	}
	return this.bulkWrite
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
	if oid = it.GetOID(this.Updater, iid); oid == "" {
		err = ErrUnableUseIIDOperation
	}
	return
}

func (this *Collection) Insert(op *operator.Operator, before ...bool) {
	this.format(op)
	this.statement.insert(op, before...)
}

func (this *Collection) Dataset() *dataset.Collection {
	return this.dataset
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
	if err := this.Updater.WriteAble(); err != nil {
		return nil
	}
	op := operator.New(t, k, v, r)
	switch d := id.(type) {
	case string:
		op.OID = d
		op.IID, this.Updater.Error = Config.ParseId(this.Updater, op.OID)
	default:
		op.IID = dataset.ParseInt32(id)
	}

	if this.Updater.Error != nil {
		return nil
	}
	if this.Updater.Error = this.mayChange(op); this.Updater.Error != nil {
		return nil
	}
	this.format(op)
	this.statement.insert(op)
	return op
}

func (this *Collection) mayChange(op *operator.Operator) (err error) {
	it := this.ITypeCollection(op.IID)
	if it == nil {
		return ErrITypeNotExist(op.IID)
	}
	op.IType = it.ID()
	if listen, ok := it.(ITypeListener); ok {
		listen.Listener(this.Updater, op)
	}
	if op.OType == operator.TypesDrop || op.OType == operator.TypesResolve {
		return nil
	}
	if op.OID == "" && it.Stacked(op.IID) {
		op.OID = it.GetOID(this.Updater, op.IID)
	}
	if op.OID != "" {
		this.statement.Select(op.OID)
	}
	return
}

func (this *Collection) format(op *operator.Operator) {
	if op.OType != operator.TypesSet {
		return
	}
	data := dataset.Update{}
	result, ok := op.Result.(dataset.Update)
	if !ok {
		this.Updater.Error = fmt.Errorf("Operator.set return error name:%s  result:%v", this.name, op.Result)
		return
	}
	sch := this.Schema()
	if sch == nil {
		this.Updater.Error = fmt.Errorf("operator.set schema empty:%s", this.name)
		return
	}
	for k, v := range result {
		if strings.Contains(k, ".") {
			data[k] = v
		} else if name := sch.JSName(k); name != "" {
			data[name] = v
		} else {
			this.Updater.Error = fmt.Errorf("operator.set field not exist,name:%s field:%s", this.name, k)
			return
		}
	}
	op.Result = data
}
