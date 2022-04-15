package updater

import (
	"github.com/hwcer/cosgo/library/logger"
	"github.com/hwcer/cosmo/update"
	"github.com/hwcer/updater/models"
)

type Hash struct {
	*base
	data   *Data
	update update.Update
}

func NewHash(model *models.Model, updater *Updater) *Hash {
	b := NewBase(model, updater)
	return &Hash{base: b}
}

func (h *Hash) release() {
	h.data = nil
	h.update = nil
	h.base.release()
}

func (h *Hash) Add(k int32, v int32) {
	if f := h.ParseID(k); f != "" {
		h.act(ActTypeAdd, f, v)
		it := Config.IType(k)
		if it.OnChange != nil {
			it.OnChange(h.updater, k, v)
		}
	}
}

func (h *Hash) Sub(k int32, v int32) {
	if f := h.ParseID(k); f != "" {
		h.act(ActTypeSub, f, v)
		it := Config.IType(k)
		if it.OnChange != nil {
			it.OnChange(h.updater, k, -v)
		}
	}
}

func (h *Hash) Set(k interface{}, v interface{}) {
	if f := h.ParseID(k); f != "" {
		h.act(ActTypeSet, f, v)
	}
}

func (h *Hash) Val(k interface{}) (r int64) {
	if v, ok := h.Get(k); ok {
		r, _ = ParseInt(v)
	}
	return
}

func (h *Hash) Get(k interface{}) (interface{}, bool) {
	if f := h.ParseID(k); f != "" {
		return h.data.Get(f)
	}
	return nil, false
}

func (h *Hash) Del(k interface{}) {
	logger.Warn("del is invalid:%v", h.model.Name)
	return
}

func (h *Hash) act(t ActType, k string, v interface{}) {
	h.Keys(k)
	oid, err := h.ObjectId()
	if err != nil {
		logger.Error(err)
		return
	}
	act := &Cache{OID: oid, AType: t, Key: k, Val: v}
	h.base.Act(act)
	if h.update != nil {
		h.Verify()
	}
}

func (h *Hash) Data() (err error) {
	data := h.New()
	keys := h.base.fields.String()
	var oid string
	if oid, err = h.ObjectId(); err != nil {
		return
	}
	tx := db.Select(keys...).Find(data, oid)
	if tx.RowsAffected == 0 {
		if _, ok := h.model.Model.(ModelSetOnInert); !ok {
			return ErrDataNotExist(oid)
		}
	} else if tx.Error != nil {
		return tx.Error
	}
	h.data = NewData(h.model.Schema, data)
	h.base.fields.reset()
	return nil
}

func (h *Hash) Verify() (err error) {
	defer func() {
		if err == nil {
			h.cache = append(h.cache, h.acts...)
			h.acts = nil
		} else {
			h.update = nil
			h.base.errMsg = err
		}
	}()
	if h.update == nil {
		h.update = update.New()
	}
	if len(h.base.acts) == 0 {
		return
	}
	for _, act := range h.base.acts {
		if err = parseHash(h, act); err != nil {
			return
		}
	}
	return
}

func (h *Hash) Save() (cache []*Cache, err error) {
	if h.base.errMsg != nil {
		return nil, h.base.errMsg
	}
	if h.update == nil || len(h.update) == 0 {
		return
	}
	var oid string
	if oid, err = h.ObjectId(); err != nil {
		return
	}
	if im, ok := h.model.Model.(ModelSetOnInert); ok {
		iv := im.SetOnInert(h.updater.uid, h.updater.Time())
		for k, v := range iv {
			h.update.SetOnInert(k, v)
		}
	}

	tx := db.Model(h.model.Model).Update(h.update, oid)
	if tx.Error == nil {
		cache = h.base.cache
		h.base.cache = nil
	} else {
		err = tx.Error
	}
	return
}

func (h *Hash) ObjectId() (oid string, err error) {
	return ObjectID.Create(h.updater, 0, false)
}

func (h *Hash) ParseID(id interface{}) (r string) {
	switch id.(type) {
	case string:
		return id.(string)
	default:
		iid, ok := ParseInt32(id)
		if !ok {
			return
		}
		return Config.Field(iid)
	}
}
