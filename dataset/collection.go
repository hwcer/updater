package dataset

import (
	"fmt"
)

func New(rows ...any) *Collection {
	coll := &Collection{}
	coll.rows = map[string]*Document{}
	coll.Reset(rows)
	return coll
}

// Collection
//
//	insert ,remove 相互覆盖
type Collection struct {
	rows   map[string]*Document
	update map[string]struct{}  //更新
	insert map[string]*Document //插入
	remove map[string]struct{}  //标记删除
}

// Has 是否存在记录，包括已经标记为删除记录，主要用来判断是否已经拉取过数据
func (this *Collection) Has(id string) (ok bool) {
	if _, ok = this.rows[id]; ok {
		return true
	}
	if _, ok = this.insert[id]; ok {
		return true
	}
	return false
}

// Exist 是否存在有效数据,排除已经标记为删除的记录
func (this *Collection) Exist(id string) (ok bool) {
	if _, ok = this.remove[id]; ok {
		return false
	}
	return this.Has(id)
}

// Get 获取对象，已经标记为删除的对象被视为不存在
func (this *Collection) Get(id string) (r *Document, ok bool) {
	if _, ok = this.remove[id]; ok {
		return nil, false
	}
	if r, ok = this.insert[id]; ok {
		return
	}
	r, ok = this.rows[id]
	return
}

func (this *Collection) Val(id string) (r *Document) {
	r, _ = this.Get(id)
	return
}

func (this *Collection) Set(id string, field string, value any) error {
	data := Update{}
	data[field] = value
	return this.Update(id, data)
}

// Update 批量更新
func (this *Collection) Update(id string, data Update) error {
	doc, ok := this.Get(id)
	if !ok {
		return fmt.Errorf("item not exist:%v", id)
	}
	this.setter(id, doc, data)
	return nil
}

// Insert 如果已经存在转换成更新
func (this *Collection) Insert(i any) error {
	doc := NewDoc(i)
	id := doc.GetString(ItemNameOID)
	if id == "" {
		return fmt.Errorf("item id emtpy:%v", i)
	}

	if this.Has(id) {
		data, err := doc.Json()
		if err != nil {
			return err
		}
		this.setter(id, doc, data)
		delete(this.remove, id)
	} else {
		if this.insert == nil {
			this.insert = make(map[string]*Document)
		}
		this.insert[id] = doc
	}
	return nil
}

func (this *Collection) Remove(id string) {
	if !this.Exist(id) {
		return
	}
	if this.remove == nil {
		this.remove = map[string]struct{}{}
	}
	delete(this.insert, id)
	this.remove[id] = struct{}{}
}

func (this *Collection) Save(bulkWrite BulkWrite) (err error) {
	defer this.Release()
	for k, _ := range this.remove {
		bulkWrite.Delete(k)
	}
	for _, doc := range this.insert {
		bulkWrite.Insert(doc.Interface())
	}
	for k, _ := range this.update {
		if doc, ok := this.rows[k]; ok {
			if v, e := doc.Save(); e == nil {
				bulkWrite.Update(v, k)
			}
		}
	}
	return err
}

// Update 更新信息
//func (this Collection) Update(op *operator.Operator) (err error) {
//	switch op.Type {
//	case operator.TypesDel:
//		delete(this, op.OID)
//	case operator.TypesNew:
//		if values, ok := op.Result.([]any); ok {
//			for _, v := range values {
//				err = this.create(v)
//			}
//		} else {
//			err = fmt.Errorf("OperatorTypeNew Error:%v", op.Value)
//		}
//	case operator.TypesSet:
//		update, _ := op.Result.(Update)
//		err = this.update(op.OID, update)
//	case operator.TypesAdd, operator.TypesSub:
//		update := NewUpdate(ItemNameVAL, op.Result)
//		err = this.update(op.OID, update)
//	}
//	return
//}
//
//func (this Collection) update(id string, src Update) error {
//	data := this.Get(id)
//	if data == nil {
//		return fmt.Errorf("data not exist:%v", id)
//	}
//	return data.Update(src)
//}

func (this *Collection) create(i any) (err error) {
	doc := NewDoc(i)
	if id := doc.GetString(ItemNameOID); id != "" {
		this.rows[id] = doc
	} else {
		err = fmt.Errorf("item id empty:%+v", i)
	}
	return
}
func (this *Collection) setter(id string, doc *Document, data Update) {
	if this.update == nil {
		this.update = make(map[string]struct{})
	}
	doc.Update(data)
	this.update[id] = struct{}{}
}

func (this *Collection) Reset(rows ...any) {
	this.rows = make(map[string]*Document, len(rows))
	this.update = nil
	this.insert = nil
	this.remove = nil
	for _, i := range rows {
		_ = this.create(i)
	}
}

// Release 释放
func (this *Collection) Release() {
	this.update = nil
	this.insert = nil
	this.remove = nil
}
