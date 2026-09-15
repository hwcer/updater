package hamster

import (
	"github.com/hwcer/updater/dataset"
)

// CollectionBulkWrite 实现 dataset.CollectionWriter，把 dataset 的持久化动作转发到
// 共享 BulkWrite 与模型的 Setter。**hamster.Collection 与扩展层句柄共用这一个**
// （扩展层经自己的适配函数接入，见 updater 侧 newCollectionBulkWrite）。
//
// 为什么必须有这层适配、不能直接把 Store.BulkWrite() 交给 dataset.Save：
//   - 两者不是一套接口 —— CollectionWriter 的 Delete/Insert 不带 model 参数（由适配器
//     绑定），还多一个 BulkWrite 没有的 Setter；
//   - 句柄自己实现不了 CollectionWriter —— 句柄的 Delete(id) 与接口要求的
//     Delete(where ...any) 重名不同签，一个类型上放不下。
type CollectionBulkWrite struct {
	model any
	// setter 转发到模型落库。核心版句柄传 CollectionModel.Setter 的适配闭包；
	// 扩展层传自己的（签名里是 *Updater）。
	setter func(bw BulkWrite, _id string, dirty dataset.Update, unset []string) error
	// bulk 指定往哪个 BulkWrite 写。nil 时用 Store 那份**共享**实例（常规路径）；
	// Collection.Submit（单表独立落库）会传一份独立的进来，不捎带提交整个 Store。
	bulk BulkWrite
	store *Store
}

// NewCollectionBulkWrite 不传 bulk 就用 Store 的共享实例。
// model 作为 BulkWrite.Update/Insert/Delete 的首参透传（通常就是业务模型/表）。
func NewCollectionBulkWrite(s *Store, model any, setter func(bw BulkWrite, _id string, dirty dataset.Update, unset []string) error, bulk ...BulkWrite) *CollectionBulkWrite {
	r := &CollectionBulkWrite{store: s, model: model, setter: setter}
	if len(bulk) > 0 {
		r.bulk = bulk[0]
	}
	return r
}

func (w *CollectionBulkWrite) write() BulkWrite {
	if w.bulk != nil {
		return w.bulk
	}
	return w.store.BulkWrite()
}

func (w *CollectionBulkWrite) Delete(where ...any) {
	w.write().Delete(w.model, where...)
}

func (w *CollectionBulkWrite) Insert(documents ...any) {
	w.write().Insert(w.model, documents...)
}

func (w *CollectionBulkWrite) Setter(_id string, dirty dataset.Update, unset []string) error {
	return w.setter(w.write(), _id, dirty, unset)
}
