package dataset

import (
	"github.com/hwcer/cosgo/schema"
	"github.com/hwcer/logger"
)

func NewDoc(i any) *Document {
	d := &Document{data: i}
	return d
}

type Document struct {
	sch   *schema.Schema
	data  any
	dirty map[string]any
}

// Has 是否存在字段
func (doc *Document) Has(k string) bool {
	sch, err := doc.Schema()
	if err != nil {
		return false
	}
	if field := sch.LookUpField(k); field != nil {
		return true
	}
	return false
}

func (doc *Document) Val(k string) (r any) {
	r, _ = doc.Get(k)
	return
}
func (doc *Document) Get(key string) (r any, ok bool) {
	if doc.dirty != nil {
		if r = doc.dirty[key]; r != nil {
			return r, true
		}
	}
	if m, exist := doc.data.(ModelGet); exist {
		if r, ok = m.Get(key); ok {
			return
		}
	}
	sch, err := doc.Schema()
	if err != nil {
		return
	}
	logger.Debug("建议给%v.%v添加Get接口提升性能", sch.Name, key)
	r = sch.GetValue(doc.data, key)
	ok = r != nil
	return
}

func (doc *Document) GetInt32(key string) int32 {
	v := doc.Val(key)
	return ParseInt32(v)
}
func (doc *Document) GetInt64(key string) int64 {
	v := doc.Val(key)
	return ParseInt64(v)
}
func (doc *Document) GetString(key string) string {
	v := doc.Val(key)
	r, _ := v.(string)
	return r
}

func (doc *Document) Set(k string, v any) {
	if exist := doc.Has(k); !exist {
		if sch, err := doc.Schema(); err != nil {
			logger.Alert("Document[%v] field not exist:%v", sch.Name, k)
		}
		return
	}
	if doc.dirty == nil {
		doc.dirty = make(map[string]any)
	}
	doc.dirty[k] = v
}

func (doc *Document) Add(k string, v int64) (r int64) {
	r = doc.GetInt64(k) + v
	doc.Set(k, r)
	return
}

func (doc *Document) Sub(k string, v int64) (r int64) {
	r = doc.GetInt64(k) - v
	doc.Set(k, r)
	return
}

// Update 批量更新
func (doc *Document) Update(data Update) {
	for k, v := range data {
		doc.Set(k, v)
	}
}

func (doc *Document) Save() (map[string]any, error) {
	if len(doc.dirty) == 0 {
		return nil, nil
	}
	defer doc.Release()
	dirty := make(map[string]any, len(doc.dirty))
	for k, v := range doc.dirty {
		if err := doc.write(k, v); err == nil {
			dirty[k] = v
		} else {
			logger.Alert("Document Save:%", err)
		}
	}
	return dirty, nil
}

// write 跳过缓存直接修改数据
func (doc *Document) write(k string, v any) error {
	if m, ok := doc.data.(ModelSet); ok {
		if m.Set(k, v) {
			return nil
		}
	}
	sch, err := doc.Schema()
	if err != nil {
		return err
	}
	logger.Debug("建议给%v.%v添加Set接口提升性能", sch.Name, k)
	return sch.SetValue(doc.data, k, v)
}

func (doc *Document) Schema() (sch *schema.Schema, err error) {
	if doc.sch == nil {
		if sch, err = schema.Parse(doc.data); err == nil {
			doc.sch = sch
		} else {
			logger.Error(err)
		}
	}
	return doc.sch, nil
}

// Json 转换成json 不包含主键
func (doc *Document) Json() (Update, error) {
	sch, err := doc.Schema()
	if err != nil {
		return nil, err
	}
	r := Update{}
	for _, field := range sch.Fields {
		if k := field.DBName; k != ItemNameOID {
			r[k] = sch.GetValue(doc.data, k)
		}
	}
	return r, nil
}

func (doc *Document) Reset(v any) {
	doc.sch = nil
	doc.data = v
	doc.dirty = nil
}

func (doc *Document) Release() {
	doc.dirty = nil
}
func (doc *Document) Interface() any {
	return doc.data
}
