package dataset

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/hwcer/cosgo/schema"
	"github.com/hwcer/logger"
)

func NewDoc(i any) *Document {
	if r, ok := i.(*Document); ok {
		return r
	}
	return &Document{data: i}
}

type Document struct {
	data  any
	dirty Update
	unset map[string]struct{}
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

func (doc *Document) Unset(k string) {
	if !doc.Has(k) {
		return
	}
	if doc.unset == nil {
		doc.unset = make(map[string]struct{})
	}
	doc.unset[k] = struct{}{}
	if doc.dirty != nil {
		delete(doc.dirty, k)
	}
	doc.unsetMemory(k)
}

func (doc *Document) unsetMemory(k string) {
	if m, ok := doc.data.(ModelUnset); ok {
		m.Unset(k)
		return
	}
	sch, err := doc.Schema()
	if err != nil {
		return
	}
	var v any
	if strings.Contains(k, ".") {
		v = reflect.Value{}
	}
	if err = sch.SetValue(doc.data, v, k); err != nil {
		logger.Alert("Unset: %s.%s error: %v", sch.Name, k, err)
	}
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

func (doc *Document) Save() (dirty Update, unsets []string) {
	if len(doc.dirty) > 0 {
		dirty = Update{}
		for k, v := range doc.dirty {
			if r, err := doc.setter(k, v); err != nil {
				logger.Alert("Document Save error,key:%v,Error:%v", k, err)
			} else {
				switch vv := r.(type) {
				case Update:
					dirty.Merge(vv)
				default:
					dirty[k] = r
				}
			}
		}
		doc.dirty = nil
	}
	if len(doc.unset) > 0 {
		unsets = make([]string, 0, len(doc.unset))
		for k := range doc.unset {
			unsets = append(unsets, k)
		}
		doc.unset = nil
	}
	return
}
func (doc *Document) Release() {
	doc.dirty = nil
	doc.unset = nil
}

func (doc *Document) setter(k string, v any) (r any, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("%v", e)
		}
	}()
	if m, ok := doc.data.(ModelSet); ok {
		if r, ok = m.Set(k, v); ok {
			//业务声称处理了(ok=true)，却对**非 nil 输入**产出 nil —— 视为没真正处理。
			//
			//典型场景：mongo 风格的多级路径(a.b.c)被交给只认单段键的业务 handle，
			//它解析键失败后写成 `return nil, true`。若在此放行，nil 会进 dirty，
			//而 cosmo 的 update 对 map 输入不过滤零值、含 "." 的 key 又原样下发，
			//最终发出 $set:{"a.b.c":null} —— 把库里的真实字段清成 null，
			//内存却毫不知情，重新加载后数据就没了。
			//
			//这里只拦「输入非 nil 却产出 nil」这一种组合：显式写 nil(清字段)时 v 本身
			//就是 nil，不受影响。报错后由 Save 打 Alert 并跳过该键，
			//从"静默清库"变成"不写 + 报警"。
			if r == nil && v != nil {
				return nil, fmt.Errorf("model set returned nil for key:%s, treated as unhandled", k)
			}
			return
		}
	}
	sch, err := doc.Schema()
	if err != nil {
		return nil, err
	}
	logger.Debug("建议给%v.%v添加Set接口提升性能", sch.Name, k)
	return v, sch.SetValue(doc.data, v, k)
}

func (doc *Document) Schema() (sch *schema.Schema, err error) {
	if doc.data == nil {
		err = errors.New("document not loader")
		return
	}
	return schema.Parse(doc.data)
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
		if k := field.DBName(); k != Fields.OID {
			r[k] = sch.GetValue(doc.data, k)
		}
	}
	return r, nil
}

func (doc *Document) Reset(v any) {
	doc.data = v
	doc.dirty = nil
	doc.unset = nil
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
func (doc *Document) IsNil() bool {
	return doc.data == nil
}
