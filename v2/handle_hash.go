package updater

import (
	"errors"
	"github.com/hwcer/adapter/bson"
	"github.com/hwcer/cosmo/update"
	"github.com/hwcer/logger"
	"strconv"
)

type HashModel interface {
	Getter(adapter *Updater, keys []string) (map[string]int64, error) //获取数据接口,返回 []byte(bson.raw) , bson.Document
	Setter(adapter *Updater, update update.Update) error              //保存数据接口
}

// Hash MAP储存简单数量
type Hash struct {
	base
	Dataset
	model  HashModel
	update update.Update
}

func NewHash(adapter *Updater, model any) Handle {
	r := &Hash{}
	r.base = *NewBase(adapter)
	r.model = model.(HashModel)
	return r
}

func (this *Hash) reset() {
	this.base.reset()
	this.Dataset = Dataset{}
}

func (this *Hash) release() {
	this.update = nil
	this.base.release()
	this.Dataset = nil
}
func (this *Hash) Del(k ikey) {
	this.Operator(ActTypeDel, k, nil)
}

func (this *Hash) Add(k ikey, v ival) {
	this.Operator(ActTypeAdd, k, v)
}

func (this *Hash) Sub(k ikey, v ival) {
	this.Operator(ActTypeSub, k, v)
}

func (this *Hash) Max(k ikey, v ival) {
	this.Operator(ActTypeMax, k, v)
}

func (this *Hash) Min(k ikey, v ival) {
	this.Operator(ActTypeMin, k, v)
}

// Set 设置字段值,  k：int32 ,string(可以使用 . 操作符操作子对象字段)
func (this *Hash) Set(k ikey, v any) {
	this.Operator(ActTypeSet, k, v)
}

func (this *Hash) Get(k ikey) (r any) {
	return this.Val(k)
}

func (this *Hash) Val(k ikey) (r int64) {
	if id, err := this.ParseOID(k); err == nil {
		r = this.Dataset[id]
	}
	return
}

// Bind 数据绑定,仅仅支持int int32 int64
func (this *Hash) Bind(k ikey, i any) (err error) {
	v := this.Val(k)
	switch r := i.(type) {
	case *int:
		*r = int(v)
	case *int32:
		*r = int32(v)
	case *int64:
		*r = v
	default:
		err = errors.New("bind args(i) type unknown")
	}
	return
}

func (this *Hash) Select(keys ...ikey) {
	for _, k := range keys {
		if field, err := this.ParseOID(k); err == nil {
			this.base.Select(field)
		} else {
			logger.Warn(err)
		}
	}
}

func (this *Hash) Data() error {
	if this.Error != nil {
		return this.Error
	}
	keys := this.base.Fields.keys
	if len(keys) == 0 {
		return nil
	}
	defer this.base.Fields.done()
	data, err := this.model.Getter(this.Adapter, keys)
	if err != nil {
		return err
	}
	var ok bool
	for k, v := range data {
		if _, ok = this.Dataset[k]; !ok {
			this.Dataset[k] = v
		}
	}
	return nil
}

func (this *Hash) Verify() (err error) {
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
		if err = hashParse(this, act); err != nil {
			return
		}
	}
	return
}

func (this *Hash) Save() (cache []*Cache, err error) {
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

func (this *Hash) ParseId(k ikey) (iid int32, oid string, err error) {
	if IsOID(k) {
		oid = k.(string)
		iid, err = this.ParseInt32(oid)
	} else {
		iid = bson.ParseInt32(k)
		oid = strconv.Itoa(int(iid))
	}
	return
}

func (this *Hash) ParseIID(k ikey) (iid int32, err error) {
	if IsOID(k) {
		return this.ParseInt32(k.(string))
	} else {
		return bson.ParseInt32(k), nil
	}
}
func (this *Hash) ParseOID(k ikey) (oid string, err error) {
	if IsOID(k) {
		return k.(string), nil
	} else {
		return strconv.Itoa(int(bson.ParseInt64(k))), nil
	}
}

func (this *Hash) ParseInt32(k string) (iid int32, err error) {
	var v int
	v, err = strconv.Atoi(k)
	return int32(v), err
}

func (this *Hash) Operator(t ActType, k ikey, v ival) {
	var err error
	defer func() {
		if err != nil {
			this.Error = err
		}
	}()
	//必须是正整数
	if t.MustNumber() && (!bson.IsNumber(v) || bson.ParseInt64(v) < 0) {
		err = errors.New("val must uint64")
		return
	}

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
