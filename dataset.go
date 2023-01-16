package updater

import (
	"errors"
	"fmt"
	"github.com/hwcer/cosgo/schema"
	"reflect"
)

func NewData(schema *schema.Schema, item interface{}) *Data {
	return &Data{item: item, schema: schema}
}

func NewDataset(schema *schema.Schema) *Dataset {
	dt := &Dataset{schema: schema}
	dt.release()
	return dt
}

type Data struct {
	item         interface{}
	schema       *schema.Schema
	reflectValue reflect.Value
}

type Dataset struct {
	schema  *schema.Schema
	dataset map[string]*Data
	indexes map[int32][]string
}

func (this *Data) Reflect() reflect.Value {
	if this.reflectValue.IsValid() {
		return this.reflectValue
	}
	reflectValue := reflect.ValueOf(this.item)
	for reflectValue.Kind() == reflect.Ptr || reflectValue.Kind() == reflect.Interface {
		reflectValue = reflectValue.Elem()
	}
	this.reflectValue = reflectValue
	return this.reflectValue
}

func (this *Data) OID() string {
	v, _ := this.Get(ItemNameOID)
	if r, ok := v.(string); ok {
		return r
	} else {
		return ""
	}
}
func (this *Data) IID() int32 {
	v, _ := this.Get(ItemNameIID)
	r, _ := ParseInt32(v)
	return r
}

func (this *Data) Get(key string) (interface{}, bool) {
	if m, ok := this.item.(ModelGetVal); ok {
		return m.GetVal(key)
	}
	reflectValue := this.Reflect()
	if !reflectValue.IsValid() {
		return nil, false
	}
	field := this.schema.LookUpField(key)
	if field == nil {
		return nil, false
	}
	v := reflectValue.FieldByIndex(field.StructField.Index).Interface()
	return v, true
}

func (this *Data) GetInt(key string) (int64, bool) {
	v, ok := this.Get(key)
	if !ok {
		return 0, ok
	}
	return ParseInt(v)
}

func (this *Data) Set(key string, val interface{}) (interface{}, error) {
	if m, ok := this.item.(ModelSetVal); ok {
		return m.SetVal(key, val)
	}
	reflectValue := this.Reflect()
	if !reflectValue.IsValid() {
		return val, nil //TODO
	}
	field := this.schema.LookUpField(key)
	if field == nil {
		return nil, fmt.Errorf("item field not exist:%v", key)
	}
	return val, field.Set(reflectValue, val)
}

func (this *Data) Add(key string, val int64) (r int64, err error) {
	i, _ := this.Get(key)
	if i == nil {
		return 0, ErrObjNotExist(key)
	}
	v, ok := ParseInt(i)
	if !ok {
		return 0, errors.New("item field not number")
	}
	r = v + val
	_, err = this.Set(key, r)
	return
}
func (this *Data) MSet(vs map[string]interface{}) (err error) {
	for k, v := range vs {
		if _, err = this.Set(k, v); err != nil {
			return
		}
	}
	return
}

func (this *Dataset) Get(oid string) (*Data, bool) {
	r, ok := this.dataset[oid]
	return r, ok
}

func (this *Dataset) Set(i interface{}) (r bool) {
	item := NewData(this.schema, i)
	oid := item.OID()
	if oid == "" {
		return
	}
	iid := item.IID()
	if iid == 0 {
		return
	}
	if _, ok := this.dataset[oid]; !ok {
		this.indexes[iid] = append(this.indexes[iid], oid)
	}
	this.dataset[oid] = item
	return true
}

func (this *Dataset) Del(oid string) bool {
	item, ok := this.dataset[oid]
	if !ok {
		return true
	}
	iid := item.IID()
	delete(this.dataset, oid)
	indexes := this.indexes[iid]
	newIndexes := make([]string, 0, len(indexes)-1)
	for _, v := range indexes {
		if v != oid {
			newIndexes = append(newIndexes, v)
		}
	}
	this.indexes[iid] = newIndexes
	return true
}
func (this *Dataset) Val(oid string) (r int64) {
	item, ok := this.dataset[oid]
	if !ok {
		return
	}
	v, _ := item.Get(ItemNameVAL)
	r, _ = ParseInt(v)
	return
}

func (this *Dataset) Data(oid string) (interface{}, bool) {
	if r, ok := this.dataset[oid]; ok {
		return r.item, true
	} else {
		return nil, false
	}
}

// Count 统计道具数量,如果道具不可叠加 则统计所有
// 叠加道具效果捅Val
func (this *Dataset) Count(iid int32) (r int64) {
	for _, oid := range this.Indexes(iid) {
		r += this.Val(oid)
	}
	return
}

// Indexes 配置ID为id的道具oid集合
func (this *Dataset) Indexes(iid int32) (r []string) {
	if v, ok := this.indexes[iid]; ok {
		r = append(r, v...)
	}
	return
}

// release 重置清空数据
func (this *Dataset) release() {
	this.dataset = make(map[string]*Data)
	this.indexes = make(map[int32][]string)
}
