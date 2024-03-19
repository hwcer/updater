package dataset

import (
	"fmt"
)

func NewColl(rows ...any) *Collection {
	coll := &Collection{}
	coll.dataset = Dataset{}
	coll.Reset(rows...)
	return coll
}

type Dirty map[string]struct{}

func (d Dirty) Set(k string) {
	d[k] = struct{}{}
}
func (d Dirty) Has(k string) (ok bool) {
	_, ok = d[k]
	return
}
func (d Dirty) Del(k string) {
	delete(d, k)
}

type Dataset map[string]*Document

func (d Dataset) Set(k string, doc *Document) {
	d[k] = doc
}
func (d Dataset) Has(k string) (ok bool) {
	_, ok = d[k]
	return
}
func (d Dataset) Del(k string) {
	delete(d, k)
}

func (d Dataset) Get(k string) (doc *Document, ok bool) {
	doc, ok = d[k]
	return
}

type Collection struct {
	dirty   Dirty   //更新标记
	remove  Dirty   //删除标记
	insert  Dataset //插入标记
	dataset Dataset //数据集
}

// Has 是否存在记录，包括已经标记为删除记录，主要用来判断是否已经拉取过数据
func (coll *Collection) Has(id string) bool {
	if coll.remove.Has(id) {
		return false
	} else if coll.insert.Has(id) {
		return true
	} else if coll.dataset.Has(id) {
		return true
	}
	return false
}

// Get 获取对象，已经标记为删除的对象被视为不存在
func (coll *Collection) Get(id string) (*Document, bool) {
	if coll.remove.Has(id) {
		return nil, false
	}
	return coll.getter(id)
}

func (coll *Collection) Val(id string) (r *Document) {
	r, _ = coll.Get(id)
	return
}

func (coll *Collection) Set(id string, field string, value any) error {
	data := Update{}
	data[field] = value
	return coll.Update(id, data)
}
func (coll *Collection) New(i ...any) (err error) {
	for _, v := range i {
		if err = coll.Insert(v); err != nil {
			return
		}
	}
	return
}

// Update 批量更新,对象必须已经存在
func (coll *Collection) Update(id string, data Update) error {
	doc, ok := coll.Get(id)
	if !ok {
		return fmt.Errorf("item not exist:%v", id)
	}
	coll.setter(id, doc, data)
	return nil
}

// Insert 如果已经存在转换成覆盖
func (coll *Collection) Insert(i any) (err error) {
	doc := NewDoc(i)
	id := doc.GetString(ItemNameOID)
	if id == "" {
		return fmt.Errorf("item id emtpy:%v", i)
	}
	defer func() {
		if err == nil {
			delete(coll.remove, id)
		}
	}()

	if v, ok := coll.insert.Get(id); ok {
		v.Reset(i)
	} else if v, ok = coll.dataset.Get(id); ok {
		var data Update
		if data, err = doc.Json(); err == nil {
			coll.setter(id, v, data)
		}
	} else {
		if coll.insert == nil {
			coll.insert = Dataset{}
		}
		coll.insert.Set(id, doc)
	}
	return
}

func (coll *Collection) Delete(id string) {
	coll.dirty.Del(id)
	coll.insert.Del(id)
	if coll.remove == nil {
		coll.remove = Dirty{}
	}
	coll.remove.Set(id)
}

// Remove 从内存中清理，不会触发持久化操作
func (coll *Collection) Remove(id ...string) {
	for _, k := range id {
		delete(coll.dirty, k)
		delete(coll.insert, k)
		delete(coll.dataset, k)
	}
}

// Dirty 外部直接修改doc后用来标记修改
func (coll *Collection) Dirty(id ...string) {
	for _, k := range id {
		coll.setter(k, nil, nil)
	}
}

func (coll *Collection) Save(bulkWrite BulkWrite) error {
	for k, _ := range coll.remove {
		coll.dataset.Del(k)
		if bulkWrite != nil {
			bulkWrite.Delete(k)
		}
	}
	for k, doc := range coll.insert {
		if err := doc.Save(nil); err == nil {
			coll.dataset.Set(k, doc)
			if bulkWrite != nil {
				bulkWrite.Insert(doc.Any())
			}
		}
	}
	for k, _ := range coll.dirty {
		if doc, ok := coll.dataset.Get(k); ok {
			v := Update{}
			if err := doc.Save(v); err == nil && len(v) > 0 && bulkWrite != nil {
				bulkWrite.Update(v, k)
			}
		}
	}
	return nil
}

func (coll *Collection) Range(handle func(string, *Document) bool) {
	for k, v := range coll.dataset {
		if !handle(k, v) {
			return
		}
	}
}

func (coll *Collection) Reset(rows ...any) {
	coll.dataset = make(Dataset, len(rows))
	coll.dirty = nil
	coll.insert = nil
	coll.remove = nil
	for _, i := range rows {
		_ = coll.create(i)
	}
}

// Release 释放执行过程
func (coll *Collection) Release() {
	for k, _ := range coll.dirty {
		if doc, ok := coll.dataset[k]; ok {
			doc.Release()
		}
	}
	for _, doc := range coll.insert {
		doc.Release()
	}

	coll.dirty = nil
	coll.insert = nil
	coll.remove = nil
}

func (coll *Collection) Length() int {
	return len(coll.dataset)
}

// Receive 接收器，接收外部对象放入列表，不进行任何操作，一般用于初始化
func (coll *Collection) Receive(id string, data any) {
	coll.dataset.Set(id, NewDoc(data))
}
func (coll *Collection) create(i any) (err error) {
	doc := NewDoc(i)
	if id := doc.GetString(ItemNameOID); id != "" {
		coll.dataset.Set(id, doc)
	} else {
		err = fmt.Errorf("item id empty:%+v", i)
	}
	return
}
func (coll *Collection) setter(id string, doc *Document, data Update) {
	if coll.dirty == nil {
		coll.dirty = Dirty{}
	}
	if doc != nil && data != nil {
		doc.Update(data)
	}
	coll.dirty.Set(id)
}
func (coll *Collection) getter(id string) (r *Document, ok bool) {
	if r, ok = coll.insert.Get(id); ok {
		return
	}
	return coll.dataset.Get(id)
}
