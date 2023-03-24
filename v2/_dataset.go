package updater

import (
	"github.com/hwcer/cosgo/logger"
	"github.com/hwcer/cosgo/schema"
)

func NewData(i any) *Data {
	return &Data{item: i}
}

type Data struct {
	item any
}

type Dataset map[string]*Data

func (this *Data) OID() string {
	v := this.Get(ItemNameOID)
	r, _ := v.(string)
	return r
}
func (this *Data) IID() int32 {
	v := this.Get(ItemNameIID)
	return ParseInt32(v)
}
func (this *Data) VAL() int64 {
	v := this.Get(ItemNameVAL)
	return ParseInt64(v)
}

func (this *Data) Get(key string) any {
	if m, ok := this.item.(documentGet); ok {
		return m.Get(key)
	}
	sch, err := schema.Parse(this)
	if err != nil {
		logger.Error(err)
		return nil
	}
	return sch.GetValue(this, key)
}

func (this *Data) Set(key string, val any) error {
	if m, ok := this.item.(documentSet); ok {
		return m.Set(key, val)
	}
	sch, err := schema.Parse(this)
	if err != nil {
		logger.Error(err)
		return nil
	}
	return sch.SetValue(this, key, val)
}

func (this *Data) MSet(data map[string]any) (err error) {
	if m, ok := this.item.(documentSet); ok {
		for k, v := range data {
			if err = m.Set(k, v); err != nil {
				return
			}
		}
	}
	sch, err := schema.Parse(this)
	if err != nil {
		logger.Error(err)
		return nil
	}
	for k, v := range data {
		if err = sch.SetValue(this, k, v); err != nil {
			return
		}
	}
	return nil
}

func (this *Data) Add(key string, val int64) (r int64, err error) {
	i := this.Get(key)
	if i == nil {
		return 0, ErrObjNotExist(key)
	}
	r = ParseInt64(i) + val
	err = this.Set(key, r)
	return
}

func (this Dataset) Has(oid string) bool {
	_, ok := this[oid]
	return ok
}

func (this Dataset) Get(oid string) *Data {
	return this[oid]
}

func (this Dataset) Set(oid string, data any) {
	this[oid] = &Data{item: data}
}

func (this Dataset) Del(oid string) {
	delete(this, oid)
}
