package updater

import (
	"fmt"
	"github.com/hwcer/cosgo/logger"
	"github.com/hwcer/cosgo/schema"
	"github.com/hwcer/updater/v2/dataset"
	"github.com/hwcer/updater/v2/operator"
)

/*
建议使用 Document.Get(nil) 获取dataset.(struct) 直接读struct字段
切记不要直接修改dataset
建议使用dataset中实现以下接口提高性能

	Get(k string) any              //获取k的值
	Set(k string, v any) error    //设置k值的为v
*/
type documentModel interface {
	Init(update *Updater, init bool) (any, error)                 //获取对象,init==true时需要初始化对象(从数据库中获取值)
	Getter(update *Updater, model any, keys []string) error       //获取数据接口,需要对data进行赋值
	Setter(update *Updater, model any, data map[string]any) error //保存数据接口,需要从data中取值
}

type documentKeys map[string]bool
type documentDirty map[string]any

func (this documentKeys) Keys() (r []string) {
	for k, _ := range this {
		r = append(r, k)
	}
	return
}
func (this documentKeys) Merge(src documentKeys) {
	for k, v := range src {
		this[k] = v
	}
}

// Document 文档存储
type Document struct {
	*statement
	keys   documentKeys
	dirty  documentDirty
	model  documentModel //handle model
	fields documentKeys  //非内存模式已经初始化的字段
	schema *schema.Schema
	//update  map[string]any //更新器
	dataset any
}

func NewDocument(u *Updater, model any, ram RAMType) Handle {
	r := &Document{}
	r.model = model.(documentModel)
	r.statement = NewStatement(u, ram, r.Operator)
	return r
}

// Has 查询key(DBName)是否已经初始化
func (this *Document) has(key string) (r bool) {
	if this.statement.ram == RAMTypeAlways || (this.keys != nil && this.keys[key]) || (this.fields != nil && this.fields[key]) {
		return true
	}
	if this.dirty != nil {
		_, r = this.dirty[key]
	}
	return false
}

func (this *Document) set(k string, v any) (err error) {
	if i, ok := this.dataset.(dataset.ModelSet); ok {
		return i.Set(k, v)
	}
	err = this.schema.SetValue(this.dataset, k, v)
	logger.Debug("建议给%v添加Set接口提升性能", this.schema.Name)
	return
}
func (this *Document) get(k string) (r any) {
	if i, ok := this.dataset.(dataset.ModelGet); ok {
		return i.Get(k)
	}
	r = this.schema.GetValue(this.dataset, k)
	logger.Debug("建议给%v添加Get接口提升性能", this.schema.Name)
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
	this.keys = documentKeys{}
	this.statement.reset()
	if this.dirty == nil {
		this.dirty = documentDirty{}
	}
	if this.fields == nil && this.statement.ram != RAMTypeAlways {
		this.fields = documentKeys{}
	}
	if this.dataset == nil {
		if this.statement.ram == RAMTypeAlways {
			this.dataset, this.Updater.Error = this.model.Init(this.statement.Updater, true)
		} else {
			this.dataset, this.Updater.Error = this.model.Init(this.statement.Updater, false)
		}
	}
	if this.schema == nil {
		this.schema, this.Updater.Error = schema.Parse(this.dataset)
	}
}

// release 运行时释放
func (this *Document) release() {
	this.keys = nil
	this.statement.release()
	if this.statement.ram == RAMTypeNone {
		this.dirty = nil
		this.fields = nil
		this.dataset = nil
	}
}

// 关闭时执行,玩家下线
func (this *Document) destruct() (err error) {
	return this.save()
}

// Get k==nil 获取的是整个struct
// 不建议使用GET获取特定字段值
func (this *Document) Get(k any) (r any) {
	if key, _, err := this.ObjectId(k); err == nil {
		r = this.get(key)
	}
	return
}

// Val 不建议使用Val获取特定字段值的int64值
func (this *Document) Val(k any) (r int64) {
	if key, _, err := this.ObjectId(k); err == nil {
		r = this.val(key)
	}
	return
}

// Set 设置
// Set(k string,v any)
func (this *Document) Set(k any, v ...any) {
	switch len(v) {
	case 1:
		this.Operator(operator.TypeSet, k, v[0])
	default:
		this.Updater.Error = ErrArgsIllegal(k, v)
	}
}

func (this *Document) Select(keys ...any) {
	if this.ram == RAMTypeAlways {
		return
	}
	for _, k := range keys {
		if key, _, err := this.ObjectId(k); err == nil && !this.has(key) {
			this.keys[key] = true
		} else {
			logger.Warn(err)
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
	if err = this.model.Getter(this.Updater, this.dataset, keys); err == nil {
		this.fields.Merge(this.keys)
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

func (this *Document) Save() (err error) {
	if this.Updater.Error != nil {
		return this.Updater.Error
	}
	//同步到内存
	for _, act := range this.statement.operator {
		if act.TYP.IsValid() {
			if e := this.set(act.Key, act.Value); e != nil {
				logger.Debug("数据保存失败可能是类型不匹配已经丢弃,table:%v,value:%v", this.schema.Table, act.Value)
			} else {
				this.dirty[act.Key] = act.Value
			}
		}
	}
	this.statement.done()
	if err = this.save(); err != nil && this.ram != RAMTypeNone {
		logger.Warn("数据库[%v]同步数据错误,等待下次同步:%v", this.schema.Table, err)
		err = nil
	}
	return
}

func (this *Document) ObjectId(k any) (key string, iid int32, err error) {
	switch v := k.(type) {
	case string:
		key = v
	default:
		iid = ParseInt32(k)
		key, err = this.Updater.CreateId(v)
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

func (this *Document) Operator(t operator.Types, k any, v any) {
	if t == operator.TypeDel {
		logger.Debug("updater document del is disabled")
		return
	}
	cache := operator.New(t, v)
	cache.OID = this.schema.Table
	cache.Key, cache.IID, this.Updater.Error = this.ObjectId(k)
	if this.Updater.Error != nil {
		return
	}
	if !this.has(cache.Key) {
		this.keys[cache.Key] = true
	}
	this.statement.Operator(cache)
	if this.verified {
		_ = this.Verify()
	}
}

// Interface 用来导出dataset
func (this *Document) Interface() any {
	return this.dataset
}
