package updater

import (
	"github.com/hwcer/cosgo/library/logger"
	"github.com/hwcer/cosmo/update"
)

type HashModelObjectID interface {
	ObjectID(u *Updater) (string, error)
}

type Hash struct {
	*base
	data   *Data
	update update.Update
}

func NewHash(model *Model, updater *Updater) *Hash {
	b := NewBase(model, updater)
	return &Hash{base: b}
}

func (this *Hash) release() {
	this.data = nil
	this.update = nil
	this.base.release()
}

func (this *Hash) Add(k int32, v int32) {
	if f := this.ParseId(k); f != "" {
		this.act(ActTypeAdd, f, v)
		it := Config.IType(k)
		if onChange, ok := it.(ITypeOnChange); ok {
			onChange.OnChange(this.updater, k, v)
		}
	}
}

func (this *Hash) Sub(k int32, v int32) {
	if f := this.ParseId(k); f != "" {
		this.act(ActTypeSub, f, v)
		it := Config.IType(k)
		if onChange, ok := it.(ITypeOnChange); ok {
			onChange.OnChange(this.updater, k, -v)
		}
	}
}

func (this *Hash) Set(k interface{}, v interface{}) {
	if f := this.ParseId(k); f != "" {
		this.act(ActTypeSet, f, v)
	}
}

func (this *Hash) Val(k interface{}) (r int64) {
	if v, ok := this.Get(k); ok {
		r, _ = ParseInt(v)
	}
	return
}

func (this *Hash) Get(k interface{}) (interface{}, bool) {
	if f := this.ParseId(k); f != "" {
		return this.data.Get(f)
	}
	return nil, false
}

func (this *Hash) Del(k interface{}) {
	logger.Warn("del is invalid:%v", this.model.Name)
	return
}

func (this *Hash) act(t ActType, k string, v interface{}) {
	this.Keys(k)
	oid, err := this.ObjectID()
	if err != nil {
		logger.Error(err)
		return
	}
	act := &Cache{OID: oid, AType: t, Key: k, Val: v}
	this.base.Act(act)
	if this.update != nil {
		this.Verify()
	}
}

func (this *Hash) Data() (err error) {
	data := this.New()
	keys := this.base.fields.String()
	var oid string
	if oid, err = this.ObjectID(); err != nil {
		return
	}
	tx := db.Select(keys...).Find(data, oid)
	if tx.RowsAffected == 0 {
		if _, ok := this.model.Model.(ModelSetOnInert); !ok {
			return ErrDataNotExist(oid)
		}
	} else if tx.Error != nil {
		return tx.Error
	}
	this.data = NewData(this.model.Schema, data)
	this.base.fields.reset()
	return nil
}

func (this *Hash) Verify() (err error) {
	defer func() {
		if err == nil {
			this.cache = append(this.cache, this.acts...)
			this.acts = nil
		} else {
			this.update = nil
			this.base.errMsg = err
		}
	}()
	if this.update == nil {
		this.update = update.New()
	}
	if len(this.base.acts) == 0 {
		return
	}
	for _, act := range this.base.acts {
		if err = parseHash(this, act); err != nil {
			return
		}
	}
	return
}

func (this *Hash) Save() (cache []*Cache, err error) {
	if this.base.errMsg != nil {
		return nil, this.base.errMsg
	}
	if this.update == nil || len(this.update) == 0 {
		return
	}
	var oid string
	if oid, err = this.ObjectID(); err != nil {
		return
	}
	if im, ok := this.model.Model.(ModelSetOnInert); ok {
		iv := im.SetOnInert(this.updater.uid, this.updater.Time())
		for k, v := range iv {
			this.update.SetOnInert(k, v)
		}
	}

	tx := db.Model(this.model.Model).Update(this.update, oid)
	if tx.Error == nil {
		cache = this.base.cache
		this.base.cache = nil
	} else {
		err = tx.Error
	}
	return
}

func (this *Hash) ObjectID() (oid string, err error) {
	if m, ok := this.model.Model.(HashModelObjectID); ok {
		return m.ObjectID(this.updater)
	}
	return this.updater.Uid(), nil
}

func (this *Hash) ParseId(id interface{}) (r string) {
	switch id.(type) {
	case string:
		return id.(string)
	default:
		iid, ok := ParseInt32(id)
		if !ok {
			return
		}
		it := Config.IType(iid)
		if it != nil {
			r, _ = it.CreateId(this.updater, iid)
		}
	}
	return
}
