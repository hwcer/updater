package updater

import (
	"errors"
	"fmt"
	"github.com/hwcer/cosgo/schema"
	"github.com/hwcer/updater/dataset"
	"github.com/hwcer/updater/operator"
)

/*
建议使用 Document.Get(nil) 获取dataset.(struct) 直接读struct字段
切记不要直接修改dataset
建议使用dataset中实现以下接口提高性能

	Get(k string) any              //获取k的值
	Set(k string, v any) error    //设置k值的为v
*/
type documentModel interface {
	New(update *Updater) any                                      //初始化对象
	Field(update *Updater, iid int32) (string, error)             //使用IID映射字段名
	Getter(update *Updater, model any, keys []string) error       //获取数据接口,需要对data进行赋值,keys==nil 获取所有
	Setter(update *Updater, model any, data map[string]any) error //保存数据接口
}

//type documentKeys map[string]any

type documentDirty map[string]any

func (this documentDirty) Has(k string) bool {
	if _, ok := this[k]; ok {
		return true
	}
	return false
}

func (this documentDirty) Keys() (r []string) {
	for k, _ := range this {
		r = append(r, k)
	}
	return
}

func (this documentDirty) Merge(src documentDirty) {
	for k, v := range src {
		this[k] = v
	}
}

// Document 文档存储
type Document struct {
	*statement
	keys  documentDirty //当前执行过程需要查询的key
	dirty documentDirty //数据缓存
	model documentModel //handle model
	//schema  *schema.Schema
	dataset *dataset.Document
	history documentDirty // 仅按需获取模式(RAMTypeMaybe)下记录历史拉取记录
}

func NewDocument(u *Updater, model any, ram RAMType) Handle {
	r := &Document{}
	r.model = model.(documentModel)
	r.statement = NewStatement(u, ram, r.operator)
	return r
}
func (this *Document) Parser() Parser {
	return ParserTypeDocument
}

// Has 查询key(DBName)是否已经初始化
func (this *Document) has(key string) bool {
	if this.statement.ram == RAMTypeAlways {
		return true
	}
	if this.keys != nil && this.keys.Has(key) {
		return true
	}
	if this.history != nil && this.history.Has(key) {
		return true
	}
	if this.history.Has(key) {
		return true
	}
	return false
}

func (this *Document) set(k string, v any) (err error) {
	if this.dataset != nil {
		return this.dataset.Set(k, v)
	}
	return errors.New("dataset is nil")
}
func (this *Document) get(k string) (r any) {
	if this.dataset != nil {
		return this.dataset.Get(k)
	}
	return errors.New("dataset is nil")
}

func (this *Document) val(k string) (r int64) {
	if v, ok := this.values[k]; ok {
		return v
	}
	if v := this.get(k); v != nil {
		r = ParseInt64(v)
		this.values[k] = r
	}
	return
}

func (this *Document) save() (err error) {
	if len(this.dirty) == 0 {
		return
	}
	if err = this.model.Setter(this.statement.Updater, this.dataset.Interface(), this.dirty); err == nil {
		this.dirty = nil
	}
	return
}

// reset 运行时开始时
func (this *Document) reset() {
	this.statement.reset()
	if this.dirty == nil {
		this.dirty = documentDirty{}
	}
	if this.dataset == nil {
		i := this.model.New(this.Updater)
		this.dataset = dataset.NewDocument(i)
	}
	//if this.schema == nil {
	//	this.schema, this.Updater.Error = schema.Parse(this.dataset)
	//}
	if this.keys == nil && this.ram != RAMTypeAlways {
		this.keys = documentDirty{}
	}
	if this.history == nil && this.ram == RAMTypeMaybe {
		this.history = documentDirty{}
	}
}

// release 运行时释放
func (this *Document) release() {
	this.keys = nil
	this.statement.release()
	if this.statement.ram == RAMTypeNone {
		this.dirty = nil
		this.dataset = nil
	}
}
func (this *Document) init() (err error) {
	if this.statement.ram == RAMTypeAlways {
		i := this.model.New(this.Updater)
		this.dataset = dataset.NewDocument(i)
		err = this.model.Getter(this.Updater, this.dataset.Interface(), nil)
	}
	return
}

// 关闭时执行,玩家下线
func (this *Document) destroy() (err error) {
	return this.save()
}

// Get k==nil 获取的是整个struct
// 不建议使用GET获取特定字段值
func (this *Document) Get(k any) (r any) {
	if key, err := this.ObjectId(k); err == nil {
		r = this.get(key)
	}
	return
}

// Val 不建议使用Val获取特定字段值的int64值
func (this *Document) Val(k any) (r int64) {
	if key, err := this.ObjectId(k); err == nil {
		r = this.val(key)
	}
	return
}

// Set 设置
// Set(k string,v any)
func (this *Document) Set(k any, v ...any) {
	switch len(v) {
	case 1:
		this.operator(operator.TypesSet, k, 0, v[0])
	default:
		this.Updater.Error = ErrArgsIllegal(k, v)
	}
}

func (this *Document) Select(keys ...any) {
	if this.ram == RAMTypeAlways {
		return
	}
	for _, k := range keys {
		if key, err := this.ObjectId(k); err == nil && !this.has(key) {
			this.keys[key] = true
		} else {
			Logger.Alert(err)
		}
	}
}

func (this *Document) Data() (err error) {
	if this.Updater.Error != nil {
		return this.Updater.Error
	}
	if len(this.keys) == 0 {
		return nil
	}
	keys := this.keys.Keys()
	if err = this.model.Getter(this.Updater, this.dataset.Interface(), keys); err == nil && this.history != nil {
		this.history.Merge(this.keys)
	}
	this.keys = nil
	return
}

func (this *Document) Verify() (err error) {
	if this.Updater.Error != nil {
		return this.Updater.Error
	}
	for _, act := range this.statement.operator {
		if err = this.Parse(act); err != nil {
			return
		}
	}
	this.statement.verified = true
	return
}

// Table  表名
func (this *Document) Table() (r string) {
	if sch := this.Schema(); sch != nil {
		r = sch.Table
	}
	return
}

func (this *Document) Schema() *schema.Schema {
	sch, err := this.dataset.Schema()
	if err != nil {
		this.Updater.Error = err
	}
	return sch
}

func (this *Document) Submit() (r []*operator.Operator, err error) {
	defer this.statement.done()
	if this.Updater.Error != nil {
		return nil, this.Updater.Error
	}
	//同步到内存
	r = this.statement.operator
	for _, op := range r {
		if op.Type.IsValid() {
			if e := this.set(op.Key, op.Result); e != nil {
				Logger.Debug("数据保存失败可能是类型不匹配已经丢弃,table:%v,field:%v,result:%v", this.Table(), op.Key, op.Result)
			} else {
				this.dirty[op.Key] = op.Result
			}
		}
	}

	if err = this.save(); err != nil && this.ram != RAMTypeNone {
		Logger.Alert("数据库[%v]同步数据错误,等待下次同步:%v", this.Table(), err)
		err = nil
	}
	return
}

// Dirty 设置脏数据,手动修改内存后置脏同步到数据库
func (this *Document) Dirty(k string, v any) {
	this.dirty[k] = v
}
func (this *Document) Values() any {
	return this.dataset.Interface()
}

func (this *Document) ObjectId(k any) (key string, err error) {
	switch v := k.(type) {
	case string:
		key = v
	default:
		iid := ParseInt32(k)
		key, err = this.model.Field(this.Updater, iid)
	}
	if err != nil {
		return
	}
	if sch := this.Schema(); sch != nil {
		if field := sch.LookUpField(key); field != nil {
			key = field.DBName
		} else {
			err = fmt.Errorf("document field not exist")
		}
	}
	return
}
func (this *Document) IType(iid int32) IType {
	if h, ok := this.model.(modelIType); ok {
		v := h.IType(iid)
		return itypesDict[v]
	} else {
		return this.Updater.IType(iid)
	}
}
func (this *Document) operator(t operator.Types, k any, v int64, r any) {
	if t == operator.TypesDel {
		Logger.Debug("updater document del is disabled")
		return
	}
	op := operator.New(t, v, r)
	switch s := k.(type) {
	case string:
		op.Key = s
	default:
		if op.IID = ParseInt32(k); op.IID > 0 {
			op.Key, this.Updater.Error = this.model.Field(this.Updater, op.IID)
		}
	}
	if this.Updater.Error != nil {
		return
	}
	sch, err := this.dataset.Schema()
	if err != nil {
		this.Updater.Error = err
		return
	}
	op.OID = sch.Table

	if this.Updater.Error != nil {
		return
	}
	if !this.has(op.Key) {
		this.keys[op.Key] = true
		this.Updater.changed = true
	}
	it := this.IType(op.IID)
	if it != nil {
		op.Bag = it.Id()
		if listen, ok := it.(ITypeListener); ok {
			listen.Listener(this.Updater, op)
		}
	}
	this.statement.Operator(op)
	if this.verified {
		this.Updater.Error = this.Parse(op)
	}
}
