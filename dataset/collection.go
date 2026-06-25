package dataset

import (
	"fmt"
)

const CollectionMonitorKey = "_collection_cursor"

type Monitor interface {
	Insert(doc *Document)
	Delete(doc *Document)
}

type Monitors map[string]Monitor

func (m Monitors) Insert(doc *Document) {
	for _, v := range m {
		v.Insert(doc)
	}
}

func (m Monitors) Delete(doc *Document) {
	for _, v := range m {
		v.Delete(doc)
	}
}

func NewColl(rows ...any) *Collection {
	coll := &Collection{}
	coll.dataset = Dataset{}
	coll.Reset(rows...)
	return coll
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

func (d Dataset) GetAndDel(k string) (doc *Document) {
	if doc = d[k]; doc != nil {
		delete(d, k)
	}
	return
}

type Collection struct {
	dirty    Dirty    //临时数据
	cursor   *Cursor  //游标
	dataset  Dataset  //数据集
	monitors Monitors //监控数据的insert 和 delete
}

func (coll *Collection) Len() int {
	return len(coll.dataset)
}

// Has 是否存在记录，包括已经标记为删除记录，主要用来判断是否已经拉取过数据
func (coll *Collection) Has(id string) bool {
	if ok, exist := coll.dirty.Has(id); exist {
		return ok
	} else if coll.dataset.Has(id) {
		return true
	}
	return false
}

// Get 获取对象，已经标记为删除的对象被视为不存在
func (coll *Collection) Get(id string) (*Document, bool) {
	if r := coll.dirty.Get(id); r != nil {
		return r, true
	}
	return coll.dataset.Get(id)
}

func (coll *Collection) Val(id string) (r *Document) {
	r, _ = coll.Get(id)
	return
}

func (coll *Collection) Set(id string, field string, value any) error {
	data := make(map[string]any)
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
	doc.Update(data)
	dirty := coll.Dirty()
	dirty.Update(id)
	return nil
}

// Insert 插入新对象，已存在则返回错误
func (coll *Collection) Insert(i any) (err error) {
	doc := NewDoc(i)
	id := doc.GetString(Fields.OID)
	if id == "" {
		return fmt.Errorf("item id emtpy:%v", i)
	}
	if coll.Has(id) {
		return fmt.Errorf("item already exist:%v", id)
	}
	dirty := coll.Dirty()
	dirty.Insert(id, doc)
	return
}

func (coll *Collection) Delete(id string) {
	dirty := coll.Dirty()
	dirty.Delete(id)
}

// Remove 从内存中清理，不会触发持久化操作
func (coll *Collection) Remove(id ...string) {
	for _, k := range id {
		delete(coll.dirty, k)
		delete(coll.dataset, k)
	}
}

// CollectionWriter Collection 持久化所需的操作接口
type CollectionWriter interface {
	Delete(where ...any)
	Insert(documents ...any)
	Setter(_id string, dirty Update, unset []string) error
}

func (coll *Collection) Save(w CollectionWriter) (err error) {
	for k, v := range coll.dirty {
		if v.op.Has(collOperatorDelete) {
			doc := coll.dataset.GetAndDel(k)
			w.Delete(k)
			if coll.monitors != nil && doc != nil {
				coll.monitors.Delete(doc)
			}
		}
		if v.op.Has(collOperatorInsert) {
			doc := v.doc
			if v.op.Has(collOperatorUpdate) {
				doc = doc.Clone()
			}
			doc.Save()
			coll.dataset.Set(k, doc)
			w.Insert(doc.Any())
			if coll.monitors != nil {
				coll.monitors.Insert(doc)
			}
		} else if v.op.Has(collOperatorUpdate) {
			doc, _ := coll.dataset.Get(k)
			if doc == nil {
				continue
			}
			dirty, unsets := doc.Save()
			if len(dirty) > 0 || len(unsets) > 0 {
				if err = w.Setter(k, dirty, unsets); err != nil {
					break
				}
			}
		}
	}
	coll.dirty = nil
	return
}

func (coll *Collection) GetMonitor() Monitors {
	if coll.monitors == nil {
		coll.monitors = make(Monitors)
	}
	return coll.monitors
}

func (coll *Collection) SetMonitor(key string, v Monitor) {
	if coll.monitors == nil {
		coll.monitors = make(Monitors)
	}
	coll.monitors[key] = v
}

func (coll *Collection) RemoveMonitor(key string) {
	delete(coll.monitors, key)
}

func (coll *Collection) onCursorRelease() {
	coll.RemoveMonitor(CollectionMonitorKey)
}
func (coll *Collection) Cursor(key string) *Cursor {
	if coll.cursor == nil || coll.cursor.closed() {
		coll.cursor = NewCursor(coll.dataset, coll.onCursorRelease)
		coll.SetMonitor(CollectionMonitorKey, &cursorMonitor{cursor: coll.cursor})
	}
	coll.cursor.users[key] = struct{}{}
	return coll.cursor
}

func (coll *Collection) Release() {
	if coll.dirty == nil {
		return
	}
	for k, v := range coll.dirty {
		if v.op.Has(collOperatorUpdate) {
			if doc, ok := coll.dataset.Get(k); ok {
				doc.Release()
			}
		}
	}
	coll.dirty = nil
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
	for _, i := range rows {
		_ = coll.create(i)
	}
}

// Receive 接收器，接收外部对象放入列表，不进行任何操作，一般用于初始化
func (coll *Collection) Receive(id string, data any) {
	coll.dataset.Set(id, NewDoc(data))
}
func (coll *Collection) create(i any) (err error) {
	doc := NewDoc(i)
	if id := doc.GetString(Fields.OID); id != "" {
		coll.dataset.Set(id, doc)
	} else {
		err = fmt.Errorf("item id empty:%+v", i)
	}
	return
}

func (coll *Collection) Dirty() Dirty {
	if coll.dirty == nil {
		coll.dirty = Dirty{}
	}
	return coll.dirty
}
