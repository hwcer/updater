package dataset

import (
	"errors"
	"github.com/hwcer/cosmo/update"
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
	dirty update.Update
	//unset map[string]struct{} //仅仅针对BSON MAP 或者子对象(a.b)为这些结构
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
	if doc.dirty.Has(update.UpdateTypeUnset, k) {
		return nil, true
	}
	if r, ok = doc.dirty.Get(update.UpdateTypeSet, k); ok {
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
		doc.dirty = update.New()
	}
	doc.dirty.Set(k, v)
	doc.dirty.Remove(update.UpdateTypeUnset, k)
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

func (doc *Document) Unset(k string) {
	if !doc.Has(k) {
		return
	}
	if doc.dirty == nil {
		doc.dirty = update.New()
	}
	doc.dirty.Unset(k)
	doc.dirty.Remove(update.UpdateTypeSet, k)
}

// Update 批量更新
func (doc *Document) Update(data map[string]any) {
	for k, v := range data {
		doc.Set(k, v)
	}
}

func (doc *Document) Save() (dirty update.Update, err error) {
	if len(doc.dirty) == 0 {
		return
	}
	dirty, doc.dirty = doc.dirty, nil
	if m, ok := doc.data.(ModelSaving); ok {
		m.Saving(dirty)
	}
	for k, v := range dirty[update.UpdateTypeSet] {
		if err = doc.setter(k, v); err != nil {
			logger.Alert("Document Save Update:%v,Error:%v,", dirty, err)
		}
	}
	return
}
func (doc *Document) Loader() bool {
	return doc.data != nil
}

// write 跳过缓存直接修改数据
func (doc *Document) setter(k string, v any) error {
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
func (doc *Document) unset(k string) bool {
	if m, ok := doc.data.(ModelUnset); ok {
		return m.Unset(k)
	}
	logger.Debug("缺少Unset接口,操作无法完成，已经丢弃：%+v", doc.Any())
	return false
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
func (doc *Document) Json() (map[string]any, error) {
	sch, err := doc.Schema()
	if err != nil {
		return nil, err
	}
	r := map[string]any{}
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
