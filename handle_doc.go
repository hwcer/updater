package updater

import (
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
	keys    documentDirty //当前执行过程需要查询的key
	dirty   documentDirty //数据缓存
	model   documentModel //handle model
	schema  *schema.Schema
	dataset any
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
	if i, ok := this.dataset.(dataset.ModelSet); ok {
		return i.Set(k, v)
	}
	err = this.schema.SetValue(this.dataset, k, v)
	Logger.Debug("建议给%v添加Set接口提升性能", this.schema.Name)
	return
}
func (this *Document) get(k string) (r any) {
	if i, ok := this.dataset.(dataset.ModelGet); ok {
		return i.Get(k)
	}
	r = this.schema.GetValue(this.dataset, k)
	Logger.Debug("建议给%v添加Get接口提升性能", this.schema.Name)
	return
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
	if err = this.model.Setter(this.statement.Updater, this.dataset, this.dirty); err == nil {
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
		this.dataset = this.model.New(this.Updater)
	}
	if this.schema == nil {
		this.schema, this.Updater.Error = schema.Parse(this.dataset)
	}
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
		this.dataset = this.model.New(this.Updater)
		err = this.model.Getter(this.Updater, this.dataset, nil)
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
		this.operator(operator.Types_Set, k, 0, v[0])
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
	if err = this.model.Getter(this.Updater, this.dataset, keys); err == nil && this.history != nil {
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

func (this *Document) Submit() (r []*operator.Operator, err error) {
	if this.Updater.Error != nil {
		return nil, this.Updater.Error
	}
	//同步到内存
	for _, op := range this.statement.operator {
		if op.Type.IsValid() {
			if e := this.set(op.Key, op.Result); e != nil {
				Logger.Debug("数据保存失败可能是类型不匹配已经丢弃,table:%v,field:%v,result:%v", this.schema.Table, op.Key, op.Result)
			} else {
				this.dirty[op.Key] = op.Result
			}
		}
	}
	this.statement.done()
	if err = this.save(); err != nil && this.ram != RAMTypeNone {
		Logger.Alert("数据库[%v]同步数据错误,等待下次同步:%v", this.schema.Table, err)
		err = nil
	}
	r = this.statement.cache
	return
}
func (this *Document) Values() any {
	return this.dataset
}

func (this *Document) ObjectId(k any) (key string, err error) {
	switch v := k.(type) {
	case string:
		key = v
	default:
		iid := ParseInt32(k)
		key, err = this.model.Field(this.Updater, iid)
	}
	if err == nil {
		field := this.schema.LookUpField(key)
		if field != nil {
			key = field.DBName
		} else {
			err = fmt.Errorf("document field not exist")
		}
	}
	return
}

func (this *Document) operator(t operator.Types, k any, v int64, r any) {
	if t == operator.Types_Del {
		Logger.Debug("updater document del is disabled")
		return
	}
	op := operator.New(t, v, r)
	switch s := k.(type) {
	case string:
		op.Key = s
	default:
		if iid := ParseInt32(k); iid > 0 {
			op.Key, this.Updater.Error = this.model.Field(this.Updater, iid)
		}
	}
	if this.Updater.Error != nil {
		return
	}
	op.OID = this.schema.Table

	if this.Updater.Error != nil {
		return
	}
	if !this.has(op.Key) {
		this.keys[op.Key] = true
	}
	if listen, ok := this.model.(ModelListener); ok {
		listen.Listener(this.Updater, op)
	}
	this.statement.Operator(op)
	if this.verified {
		_ = this.Verify()
	}
}
