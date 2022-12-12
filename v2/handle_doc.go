package updater

import (
	"github.com/hwcer/cosgo/logger"
	"github.com/hwcer/cosmo/update"
	"github.com/hwcer/updater/bson"
)

type DocumentModel interface {
	Getter(adapter *Updater, keys []string) (any, error) //获取数据接口,返回 []byte(bson.raw) , bson.Document
	Setter(adapter *Updater, update update.Update) error //保存数据接口
}

// Document 文档存储
type Document struct {
	base
	bson.Document
	model  DocumentModel
	update update.Update
}

func NewDocument(adapter *Updater, model any) Handle {
	r := &Document{}
	r.base = *NewBase(adapter)
	r.model = model.(DocumentModel)
	return r
}

func (this *Document) reset() {
	this.base.reset()
	this.Document = bson.New()
}

func (this *Document) release() {
	this.update = nil
	this.base.release()
	this.Document = nil
}
func (this *Document) Del(k ikey) {
	this.act(ActTypeDel, k, nil)
}

func (this *Document) Add(k ikey, v ival) {
	this.act(ActTypeAdd, k, v)
}

func (this *Document) Sub(k ikey, v ival) {
	this.act(ActTypeSub, k, v)
}

func (this *Document) Max(k ikey, v ival) {
	this.act(ActTypeMax, k, v)
}

func (this *Document) Min(k ikey, v ival) {
	this.act(ActTypeMin, k, v)
}

// Set 设置字段值,  k：int32 ,string(可以使用 . 操作符操作子对象字段)
func (this *Document) Set(k ikey, v any) {
	this.act(ActTypeSet, k, v)
}

// Get 同Element仅仅包装接口
// k：int32 ,string(可以使用 . 操作符操作子对象字段)
func (this *Document) Get(k ikey) any {
	return this.Element(k)
}

func (this *Document) Val(k ikey) (r int64) {
	if ele := this.Element(k); ele != nil {
		r = ele.GetInt64()
	}
	return
}

// Bind 数据绑定,k为空时绑定当前对象，否则递归查找子对象并绑定
// k 可以使用.符合 递归子对象
// 绑定整个Document时 k="" 或者 k="."
func (this *Document) Bind(k ikey, i interface{}) (err error) {
	if ele := this.Element(k); ele != nil {
		err = ele.Unmarshal(i)
	}
	return
}

func (this *Document) Select(keys ...ikey) {
	for _, k := range keys {
		if field, err := this.Adapter.CreateId(k); err == nil {
			this.base.Select(field)
		} else {
			logger.Warn(err)
		}
	}
}

func (this *Document) Element(k ikey) (r *bson.Element) {
	field, err := this.Adapter.CreateId(k)
	if err == nil {
		r = this.Document.Get(field)
	} else {
		logger.Warn(err)
	}
	return
}

func (this *Document) act(t ActType, k ikey, v ival) {
	var err error
	defer func() {
		if err != nil {
			this.Error = err
		}
	}()

	cache := &Cache{AType: t, Val: v}
	cache.IID, cache.Key, err = this.ParseId(k)
	if err != nil {
		return
	}
	this.base.Select(cache.Key)
	it := cache.GetIType()
	if listener, ok := it.(ITypeListener); ok {
		if this.Error = listener.Listener(this.Adapter, cache); this.Error != nil {
			return
		}
	}
	this.base.Act(cache)
	if this.update != nil {
		_ = this.Verify()
	}
}

func (this *Document) Data() error {
	keys := this.base.Fields.keys
	if len(keys) == 0 {
		return nil
	}
	defer this.base.Fields.done()
	data, err := this.model.Getter(this.Adapter, keys)
	if err != nil {
		return err
	}
	doc, err := bson.Marshal(data)
	if err != nil {
		return err
	}
	this.Document.Merge(doc, false)
	return nil
}

func (this *Document) Verify() (err error) {
	if this.Error != nil {
		return this.Error
	}
	defer func() {
		if err == nil {
			this.cache = append(this.cache, this.acts...)
			this.acts = nil
		} else {
			this.update = nil
			this.base.Error = err
		}
	}()
	if this.update == nil {
		this.update = update.New()
	}
	if len(this.base.acts) == 0 {
		return
	}
	for _, act := range this.base.acts {
		if err = documentParse(this, act); err != nil {
			return
		}
	}
	return
}

func (this *Document) Save() (cache []*Cache, err error) {
	if this.Error != nil {
		return nil, this.Error
	}
	if this.update == nil || len(this.update) == 0 {
		return
	}
	if err = this.model.Setter(this.Adapter, this.update); err == nil {
		cache = this.base.cache
		this.base.cache = nil
	}
	return
}

func (this *Document) ParseId(k ikey) (iid int32, oid string, err error) {
	if IsOID(k) {
		oid = k.(string)
		iid, _ = this.Adapter.ParseId(k)
	} else {
		iid = bson.ParseInt32(k)
		oid, err = this.Adapter.CreateId(k)
	}
	return
}
