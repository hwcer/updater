package dataset

import (
	"fmt"

	"github.com/hwcer/logger"
)

// collectionMonitorKey Cursor 自用的注册键。
// 🔴 不导出：导出就意味着业务能用同一个 key 把框架的 Cursor 订阅覆盖掉。
const collectionMonitorKey = "_collection_cursor"

// Monitor 数据集变更观察者，由 Collection.Save 在文档真正落定时回调。
//
// 🔴 这个位置不可替代：Save 内当同一 OID 同时带 insert 与 update 标志时会 doc.Clone()，
// 进 Collection.dataset 的与这里收到的都是 clone。任何更早的钩子(如 operator 层)
// 拿到的都是 clone 前的对象，据此建的索引会锁死在插入瞬间的快照上。
type Monitor interface {
	Insert(doc *Document)
	Delete(doc *Document)
}

// Monitors 观察者注册表。注册/注销的方法挂在本类型上，Collection 只暴露 Monitors() 一个入口。
//
// Get/Set/Remove 用指针接收者：map 为 nil 时 Set 要能就地初始化，值接收者做不到。
// Insert/Delete 是扇出、不改注册表，保持值接收者。
type Monitors map[string]Monitor

func (m *Monitors) Get(key string) Monitor {
	if *m == nil {
		return nil
	}
	return (*m)[key]
}

func (m *Monitors) Set(key string, v Monitor) {
	if *m == nil {
		*m = Monitors{}
	}
	(*m)[key] = v
}

// Remove 注销观察者。
// 不叫 Delete —— 那个名字被下面的扇出方法占着，且语义完全相反
// (Delete 是「有文档被删了」，Remove 是「摘掉一个观察者」)。
func (m *Monitors) Remove(key string) {
	if *m != nil {
		delete(*m, key)
	}
}

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
	dirty      Dirty    //临时数据
	cursor     *Cursor  //游标
	dataset    Dataset  //数据集
	monitors   Monitors //监控数据的insert 和 delete
	saveFailed bool     //上次Save存在落库失败遗留:脏条目是下次同步的重试数据源,Release不得清空
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
//
// 🔴 带 update 脏标记的条目先强制 Save 该条(经 CollectionWriter 落库)再清理:
// 旧实现直接 delete,业务在 Update 之后、Save/Submit 之前调用 Remove 时,
// 未落库的改动被静默丢弃且无任何日志
func (coll *Collection) Remove(id ...string) {
	for _, k := range id {
		if v, ok := coll.dirty[k]; ok && v.op.Has(collOperatorUpdate) {
			if doc, exists := coll.dataset.Get(k); exists {
				//先落库该条,避免未持久化的改动被静默丢弃
				dirty, unsets, derr := doc.Save()
				if derr != nil || len(dirty) > 0 || len(unsets) > 0 {
					//此处拿不到 CollectionWriter(仅 Save 入参有),退化为
					//回填脏标记并告警——调用方下一次 Save 仍可重发。
					//🔴 derr != nil 时即便 dirty 为空同样拒绝:载荷生成失败的键
					//只在 doc 脏标记里,直接删除等于静默丢弃
					doc.Restore(dirty, unsets)
					logger.Alert("collection remove skip dirty entry, id:%s (persist before remove to discard intentionally)", k)
					continue
				}
			}
		}
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
	//只摘除已成功处理的条目:Setter失败时,失败条目(恢复doc级脏标记)与
	//尚未轮到的条目都必须留在dirty里,调用方"失败等待下次同步"的重试才有数据可重发;
	//旧实现无条件coll.dirty = nil,一次失败等于永久丢改动
	processed := make([]string, 0, len(coll.dirty))
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
				processed = append(processed, k)
				continue
			}
			dirty, unsets, derr := doc.Save() //载荷生成失败的键已在 Save 内部回填 doc 脏标记
			if len(dirty) > 0 || len(unsets) > 0 {
				if err = w.Setter(k, dirty, unsets); err != nil {
					//doc.Save已消费doc级脏标记,恢复之,下次Save重新生成载荷
					doc.Restore(dirty, unsets)
					coll.saveFailed = true
					break
				}
			}
			if derr != nil {
				//🔴 键级载荷生成失败:失败键已回填doc脏标记,但该条目不得从coll.dirty
				//摘除——摘了之后Collection级重试只遍历coll.dirty,失败键永远无人重发。
				//已成功的键照常下发,条目保留只等失败键
				coll.saveFailed = true
				continue
			}
		}
		processed = append(processed, k)
	}
	for _, k := range processed {
		delete(coll.dirty, k)
	}
	if err == nil && !coll.saveFailed {
		coll.saveFailed = false //本轮全部落库成功,解除Release保留
	}
	return
}

// Monitors 观察者注册表，注册/注销走返回值上的方法：coll.Monitors().Set(key, v)
//
// 返回 *Monitors 而非 Monitors：注册表要能从 nil 就地长出来，返回值拷贝做不到。
func (coll *Collection) Monitors() *Monitors {
	return &coll.monitors
}

func (coll *Collection) onCursorRelease() {
	coll.monitors.Remove(collectionMonitorKey)
}
func (coll *Collection) Cursor(key string) *Cursor {
	if coll.cursor == nil || coll.cursor.closed() {
		coll.cursor = NewCursor(coll.dataset, coll.onCursorRelease)
		coll.monitors.Set(collectionMonitorKey, &cursorMonitor{cursor: coll.cursor})
	}
	coll.cursor.users[key] = struct{}{}
	return coll.cursor
}

func (coll *Collection) Release() {
	if coll.saveFailed {
		//存在落库失败遗留:恢复过的doc级脏标记与未处理条目是"失败等待下次同步"的重试数据源,
		//清空等于把失败条目的改动永久丢弃
		return
	}
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

// Count 统计满足条件的记录数,O(n) 全量扫描
// 注意与 Range 的差异:Range 只遍历 dataset,看不到 dirty 中尚未 Save 的新增;
// Count 包含 dirty 待插入、排除 dirty 待删除,保证同一次请求内连续多次插入能被立即计入
// (上限检查依赖这一点,否则一个请求内连发 N 次插入会全部通过)
func (coll *Collection) Count(match func(doc *Document) bool) (r int64) {
	for k, doc := range coll.dataset {
		if ok, exist := coll.dirty.Has(k); exist && !ok {
			continue //已标记删除
		}
		if match(doc) {
			r++
		}
	}
	for k, v := range coll.dirty {
		if !v.op.Has(collOperatorInsert) || v.doc == nil {
			continue
		}
		if coll.dataset.Has(k) {
			continue //已在 dataset 中统计过
		}
		if match(v.doc) {
			r++
		}
	}
	return
}

func (coll *Collection) Reset(rows ...any) {
	coll.dataset = make(Dataset, len(rows))
	coll.dirty = nil
	coll.saveFailed = false
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
