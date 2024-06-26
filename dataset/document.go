package dataset

import (
	"errors"
	"github.com/hwcer/logger"
	"github.com/hwcer/schema"
	"reflect"
	"strings"
)

func NewDoc(i any) *Document {
	if r, ok := i.(*Document); ok {
		return r
	}
	return &Document{data: i}
}

type Document struct {
	sch   *schema.Schema
	data  any
	dirty Update
}

// Has 是否存在字段
func (doc *Document) Has(k string) bool {
	sch, err := doc.Schema()
	if err != nil {
		return false
	}
	if i := strings.Index(k, "."); i > 0 {
		k = k[0:i]
	}
	if field := sch.LookUpField(k); field != nil {
		return true
	} else {
		logger.Alert("document[%v] does not have field:%v ", sch.Name, k)
	}
	return false
}

func (doc *Document) Val(k string) (r any) {
	r, _ = doc.Get(k)
	return
}
func (doc *Document) Get(k string) (r any, ok bool) {
	if r, ok = doc.dirty.Get(k); ok {
		return
	}
	if m, exist := doc.data.(ModelGet); exist {
		if r, ok = m.Get(k); ok {
			return
		}
	}
	sch, err := doc.Schema()
	if err != nil {
		return
	}
	logger.Debug("建议给%v.%v添加Get接口提升性能", sch.Name, k)
	r = sch.GetValue(doc.data, k)
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
	if !doc.Has(k) {
		return
	}
	if doc.dirty == nil {
		doc.dirty = Update{}
	}
	doc.dirty.Set(k, v)
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

func (doc *Document) Save(dirty Update) error {
	if len(doc.dirty) == 0 {
		return nil
	}
	for k, v := range doc.dirty {
		if err := doc.write(k, v); err != nil {
			logger.Alert("Document Save:%v", err)
		} else if dirty != nil {
			dirty.Set(k, v)
		}
	}
	if dirty != nil {
		if m, ok := doc.data.(ModelSaving); ok {
			m.Saving(dirty)
		}
	}
	return nil
}
func (doc *Document) Loader() bool {
	return doc.data != nil
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
	return sch.SetValue(doc.data, v, k)
}

func (doc *Document) Schema() (sch *schema.Schema, err error) {
	if doc.data == nil {
		err = errors.New("document not loader")
		return
	}
	if doc.sch == nil {
		if sch, err = schema.Parse(doc.data); err == nil {
			doc.sch = sch
		} else {
			logger.Error(err)
		}
	} else {
		sch = doc.sch
	}
	return
}
func (doc *Document) Clone() *Document {
	if i, ok := doc.data.(ModelClone); ok {
		return &Document{dirty: doc.dirty, data: i.Clone()}
	}

	//使用反射获取复制体
	srcValue := reflect.ValueOf(doc.data)
	logger.Debug("建议添加Clone()方法提升性能:%v", srcValue.String())
	// 源对象必须是指针
	if srcValue.Kind() != reflect.Ptr {
		logger.Debug("CopyObject needs a pointer as input:%v", srcValue.String())
		return doc
	}
	// 获取源对象的元素（实际的值）
	srcElement := srcValue.Elem()
	// 根据源对象的类型创建一个新的对象
	copiedValue := reflect.New(srcElement.Type()).Elem()
	// 将源对象的字段复制到新对象中
	copiedValue.Set(srcElement)
	// 返回新对象的地址
	return &Document{dirty: doc.dirty, data: copiedValue.Addr().Interface()}
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

func (doc *Document) Range(handle func(string, any) bool) {
	sch, err := doc.Schema()
	if err != nil {
		return
	}
	for _, field := range sch.Fields {
		k := field.Name
		v := sch.GetValue(doc.data, k)
		if !handle(k, v) {
			return
		}
	}
}

func (doc *Document) Any() any {
	return doc.data
}
