package dataset

import (
	"errors"
	"testing"
)

// failWriter Setter 始终失败的 CollectionWriter
type failWriter struct{ err error }

func (failWriter) Delete(where ...any)                     {}
func (failWriter) Insert(documents ...any)                 {}
func (w failWriter) Setter(string, Update, []string) error { return w.err }

// 🔴 P0 回归:落库失败后,失败条目的脏标记必须保留,Release 不得清空——
// 否则"失败等待下次同步"没有任何数据可重发,玩家的改动被静默永久丢弃。
func TestSaveFailureRetainsDirtyAcrossRelease(t *testing.T) {
	coll := NewColl()
	if err := coll.Insert(&monTestItem{OID: "a", Val: 1}); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if err := coll.Save(monTestWriter{}); err != nil {
		t.Fatalf("初始落库: %v", err)
	}

	//制造 update 脏标记
	if err := coll.Update("a", Update{"val": int64(2)}); err != nil {
		t.Fatalf("update: %v", err)
	}

	//落库失败 → restore 留下重试数据源
	if err := coll.Save(failWriter{err: errors.New("db down")}); err == nil {
		t.Fatal("Setter 失败应返回错误")
	}

	//失败后 Release 不得清空脏标记
	coll.Release()
	if len(coll.Dirty()) == 0 {
		t.Fatal("落库失败后 Release 不应清空脏标记,否则重试数据源丢失")
	}

	//恢复后重新 Save:失败条目应被重发并成功
	if err := coll.Save(monTestWriter{}); err != nil {
		t.Fatalf("恢复后 save: %v", err)
	}
	if len(coll.Dirty()) != 0 {
		t.Fatal("重试成功后不应再有脏数据")
	}
	final, ok := coll.dataset.Get("a")
	if !ok || final.GetInt64("val") != 2 {
		t.Fatalf("重试后 val 应为 2,实际 %+v", final)
	}

	//成功后 Release 恢复常规行为(无脏数据,保持为空)
	coll.Release()
	if len(coll.Dirty()) != 0 {
		t.Fatal("成功 Save 后 Release 不应产生脏数据")
	}
}

// Document.Save 的载荷生成失败键必须回填脏标记,成功键正常消费
func TestDocumentSaveKeyFailureRestoresFailedKeys(t *testing.T) {
	doc := NewDoc(&monTestItem{OID: "a", Val: 1})
	if _, err := doc.Schema(); err != nil {
		t.Fatalf("schema: %v", err)
	}
	doc.Set("val", int64(5))
	dirty, unsets, err := doc.Save()
	if err != nil {
		t.Fatalf("正常 Save 不应出错: %v", err)
	}
	if dirty["val"] != int64(5) || len(unsets) != 0 {
		t.Fatalf("载荷不符: %+v / %v", dirty, unsets)
	}
	if len(doc.dirty) != 0 {
		t.Fatalf("成功键应被消费,剩余脏标记: %v", doc.dirty)
	}
}
