package dataset

import "testing"

// 🔴 Cursor 必须感知删除:分页期间被删除的文档要从快照里摘除,
// 否则翻页会翻出库里已不存在的数据。
func TestCursorRemovesDeletedDocs(t *testing.T) {
	coll := NewColl(
		&monTestItem{OID: "a", Val: 1},
		&monTestItem{OID: "b", Val: 2},
		&monTestItem{OID: "c", Val: 3},
	)
	cur := coll.Cursor("pager")
	if cur.Len() != 3 {
		t.Fatalf("初始快照应 3 条,实际 %d", cur.Len())
	}

	coll.Delete("b")
	if err := coll.Save(monTestWriter{}); err != nil {
		t.Fatalf("save: %v", err)
	}

	if cur.Len() != 2 {
		t.Fatalf("删除后快照应剩 2 条,实际 %d", cur.Len())
	}
	seen := map[string]bool{}
	cur.Range(0, 10, func(doc *Document) bool {
		seen[doc.GetString("_id")] = true
		return true
	})
	if seen["b"] {
		t.Fatal("已删除的文档不应再被分页翻出")
	}
	if !seen["a"] || !seen["c"] {
		t.Fatalf("其余文档应保留在快照中,实际 %v", seen)
	}
}

// 🔴 游标关闭后,迟到的插入不得复活快照(items 必须保持 nil)
func TestCursorClosedNotRevivedByInsert(t *testing.T) {
	coll := NewColl(&monTestItem{OID: "a", Val: 1})
	cur := coll.Cursor("user1")
	cur.Close("user1")
	if !cur.closed() {
		t.Fatal("Close 后游标应处于关闭状态")
	}

	if err := coll.Insert(&monTestItem{OID: "x", Val: 9}); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if err := coll.Save(monTestWriter{}); err != nil {
		t.Fatalf("save: %v", err)
	}
	if len(cur.items) != 0 {
		t.Fatalf("关闭后的游标不应被插入复活,items=%d", len(cur.items))
	}
}
