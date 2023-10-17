package dataset

import (
	"fmt"
	"github.com/hwcer/cosgo/schema"
	"github.com/hwcer/logger"
)

func NewDocument(i any) *Document {
	d := &Document{item: i}
	return d
}

type Document struct {
	sch  *schema.Schema
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

func (this *Document) Schema() (sch *schema.Schema, err error) {
	if this.sch == nil {
		if sch, err = schema.Parse(this.item); err == nil {
			this.sch = sch
		}
	}
	return this.sch, nil
}
func (this *Document) Get(key string) (r any) {
	if m, ok := this.item.(ModelGet); ok {
		if r, ok = m.Get(key); ok {
			return
		}
	}
	sch, err := this.Schema()
	if err != nil {
		logger.Error(err)
		return nil
	}
	logger.Debug("建议给%v.%v添加Get接口提升性能", sch.Name, key)
	return sch.GetValue(this.item, key)
}

func (this *Document) Set(key string, val any) error {
	if m, ok := this.item.(ModelSet); ok {
		if m.Set(key, val) {
			return nil
		}
	}
	sch, err := this.Schema()
	if err != nil {
		logger.Error(err)
		return nil
	}
	logger.Debug("建议给%v.%v添加Set接口提升性能", sch.Name, key)
	return sch.SetValue(this.item, key, val)
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

func (this *Document) Update(data Update) (err error) {
	for k, v := range data {
		if err = this.Set(k, v); err != nil {
			return
		}
	}
	return
}

func (this *Document) Interface() any {
	return this.item
}
