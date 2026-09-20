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

// failSetModel 模拟业务 ModelSet:对 badKey 走"声称处理却产出 nil"的错误路径
// (document.setter 拦截转错误),其余键放行。平铺结构,避免嵌入干扰 schema 解析
type failSetModel struct {
	Oid string `bson:"_id"`
	Val int64  `bson:"val"`
}

var failSetBadKey string

func (m *failSetModel) Set(k string, v any) (r any, ok bool) {
	if k == failSetBadKey {
		return nil, true //非 nil 输入产出 nil:setter 判定为未真正处理,转错误
	}
	return v, true
}

// Document.Save 的载荷生成失败键必须回填脏标记,成功键正常消费。
// 🔴 回归:旧版本此测试名不副实——从未让任何键失败,"失败键回填"分支零覆盖
func TestDocumentSaveKeyFailureRestoresFailedKeys(t *testing.T) {
	failSetBadKey = "val"
	defer func() { failSetBadKey = "" }()
	doc := NewDoc(&failSetModel{Oid: "a", Val: 1})
	if _, err := doc.Schema(); err != nil {
		t.Fatalf("schema: %v", err)
	}
	if !doc.Has("val") {
		t.Fatal("前置:schema 应含 val 字段")
	}
	doc.Set("val", int64(5)) //val 走失败路径
	dirty, unsets, err := doc.Save()
	if err == nil {
		t.Fatal("含失败键的 Save 应返回错误")
	}
	if len(dirty) != 0 {
		t.Fatalf("失败键不得进载荷: %+v", dirty)
	}
	if len(unsets) != 0 {
		t.Fatalf("不应有 unset: %v", unsets)
	}
	if len(doc.dirty) != 1 {
		t.Fatalf("失败键应回填脏标记等待重试,剩余: %v", doc.dirty)
	}
	if !doc.saveFailed {
		t.Fatal("键级失败应置 saveFailed,Release 不得清空重试数据源")
	}
	//🔴 Release 不得清空:失败键滞留 doc.dirty,靠它驱动下次重试
	doc.Release()
	if len(doc.dirty) != 1 {
		t.Fatalf("saveFailed 后 Release 不应清空脏标记: %v", doc.dirty)
	}
	//修复 badKey 后重试:载荷正常产出
	failSetBadKey = ""
	dirty2, _, err := doc.Save()
	if err != nil {
		t.Fatalf("重试 save: %v", err)
	}
	if dirty2["val"] != int64(5) {
		t.Fatalf("重试应重新产出载荷: %+v", dirty2)
	}
	if doc.saveFailed {
		t.Fatal("重试 Save 后 saveFailed 应解除")
	}
}

// 🔴 P1 回归:Values/Document 落库失败后,Restore 回填的脏标记必须跨 Release 存活——
// 旧实现 Release 无条件清空,"失败等待下次同步"在同一请求的 release 里就没了数据,
// 改动静默永久丢失(内存新、库旧、无重试数据)
func TestValuesSaveFailureRetainsDirtyAcrossRelease(t *testing.T) {
	val := NewValues()
	val.Set(100, 5)
	dirty, unsets := val.Save()
	if len(dirty) != 1 || len(unsets) != 0 {
		t.Fatalf("载荷不符: %+v / %v", dirty, unsets)
	}
	//模拟落库失败:Restore 回填(与 handle_val.save 失败分支同调)
	val.Restore(dirty, unsets)
	//请求结束 release:不得清空
	val.Release()
	if len(val.dirty) != 1 {
		t.Fatalf("落库失败后 Release 不应清空 Values 脏标记,实际 %v", val.dirty)
	}
	//恢复后重新 Save:改动可重发
	dirty2, _ := val.Save()
	if dirty2[100] != 5 {
		t.Fatalf("重试应重新产出载荷: %+v", dirty2)
	}
	if val.saveFailed {
		t.Fatal("重试 Save 后 saveFailed 应解除")
	}
	//正常路径 Release 照常清空
	val.Release()
	if len(val.dirty) != 0 {
		t.Fatal("正常 Release 应清空脏标记")
	}
}

// Document 整批落库失败:Restore 置位后 Release 保留,重试 Save 重新产出载荷
func TestDocumentSaveFailureRetainsDirtyAcrossRelease(t *testing.T) {
	doc := NewDoc(&monTestItem{OID: "a", Val: 1})
	if _, err := doc.Schema(); err != nil {
		t.Fatalf("schema: %v", err)
	}
	doc.Set("val", int64(9))
	dirty, unsets, err := doc.Save()
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	doc.Restore(dirty, unsets)
	doc.Release()
	if len(doc.dirty) != 1 {
		t.Fatalf("落库失败后 Release 不应清空 Document 脏标记: %v", doc.dirty)
	}
	dirty2, _, err := doc.Save()
	if err != nil {
		t.Fatalf("重试 save: %v", err)
	}
	if dirty2["val"] != int64(9) {
		t.Fatalf("重试应重新产出载荷: %+v", dirty2)
	}
}
