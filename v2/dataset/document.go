package dataset

import (
	"fmt"
	"github.com/hwcer/cosgo/logger"
	"github.com/hwcer/cosgo/schema"
)

func NewDocument(i any) *Document {
	return &Document{item: i}
}

type Document struct {
	item any
}

func (this *Document) OID() string {
	v := this.Get(ItemNameOID)
	r, _ := v.(string)
	return r
}
func (this *Document) IID() int32 {
	v := this.Get(ItemNameIID)
	return ParseInt32(v)
}
func (this *Document) VAL() int64 {
	v := this.Get(ItemNameVAL)
	return ParseInt64(v)
}

func (this *Document) Get(key string) any {
	if m, ok := this.item.(ModelGet); ok {
		return m.Get(key)
	}
	sch, err := schema.Parse(this)
	if err != nil {
		logger.Error(err)
		return nil
	}
	return sch.GetValue(this, key)
}

func (this *Document) Set(key string, val any) error {
	if m, ok := this.item.(ModelSet); ok {
		return m.Set(key, val)
	}
	sch, err := schema.Parse(this)
	if err != nil {
		logger.Error(err)
		return nil
	}
	return sch.SetValue(this, key, val)
}

func (this *Document) Add(key string, val int64) (r int64, err error) {
	i := this.Get(key)
	if i == nil {
		return 0, fmt.Errorf("data not exist:%v", key)
	}
	r = ParseInt64(i) + val
	err = this.Set(key, r)
	return
}

func (this *Document) Update(data map[string]any) (err error) {
	if m, ok := this.item.(ModelSet); ok {
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

func (this *Document) Interface() any {
	return this.item
}
