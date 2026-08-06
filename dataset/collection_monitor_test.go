package dataset

import "testing"

type monTestItem struct {
	OID string `bson:"_id" json:"_id"`
	Val int64  `bson:"val" json:"val"`
}

// monTestSpy 记录 Monitor 收到的回调
type monTestSpy struct {
	inserted []*Document
	deleted  []*Document
}

func (s *monTestSpy) Insert(doc *Document) { s.inserted = append(s.inserted, doc) }
func (s *monTestSpy) Delete(doc *Document) { s.deleted = append(s.deleted, doc) }

// monTestWriter 满足 CollectionWriter，只记不写
type monTestWriter struct{}

func (monTestWriter) Delete(where ...any)                   {}
func (monTestWriter) Insert(documents ...any)               {}
func (monTestWriter) Setter(string, Update, []string) error { return nil }

// 🔴 本文件最重要的一条：Monitor 必须拿到**落进 dataset 的那个** Document。
//
// 同一 OID 在一次请求内既 Insert 又 Update 时，Save() 会 doc.Clone()（collection.go
// insert 分支），进 coll.dataset 的是 clone，Monitor 收到的也必须是同一个 clone。
//
// 这条钉死了 Monitor 的位置不可上移：任何更早的钩子（statement/operator 层都在
// save() 之前）只能拿到 clone 前的对象，据此建的索引会锁死在插入瞬间的快照上。
func TestSaveNotifiesMonitorWithFinalDoc(t *testing.T) {
	coll := NewColl()
	spy := &monTestSpy{}
	coll.Monitors().Set("spy", spy)

	if err := coll.Insert(&monTestItem{OID: "a", Val: 1}); err != nil {
		t.Fatalf("insert: %v", err)
	}
	origin := coll.Dirty().Get("a") //clone 前的对象
	if origin == nil {
		t.Fatal("dirty 里应有待插入的文档")
	}
	//同一 OID 再来一次更新 → 打上 update 标志 → Save 时触发 Clone
	if err := coll.Update("a", Update{"val": int64(2)}); err != nil {
		t.Fatalf("update: %v", err)
	}

	if err := coll.Save(monTestWriter{}); err != nil {
		t.Fatalf("save: %v", err)
	}

	if len(spy.inserted) != 1 {
		t.Fatalf("Monitor.Insert 应被调用 1 次，实际 %d", len(spy.inserted))
	}
	final, ok := coll.dataset.Get("a")
	if !ok {
		t.Fatal("文档应已落进 dataset")
	}
	if spy.inserted[0] != final {
		t.Fatalf("Monitor 收到的不是落进 dataset 的那个文档：got=%p dataset=%p", spy.inserted[0], final)
	}
	if spy.inserted[0] == origin {
		t.Fatal("insert+update 应触发 Clone，Monitor 不该收到 clone 前的对象")
	}
}

// 删除路径拿到的是 GetAndDel 返回的文档，非 nil
func TestSaveNotifiesMonitorOnDelete(t *testing.T) {
	coll := NewColl(&monTestItem{OID: "a", Val: 1})
	spy := &monTestSpy{}
	coll.Monitors().Set("spy", spy)

	coll.Delete("a")
	if err := coll.Save(monTestWriter{}); err != nil {
		t.Fatalf("save: %v", err)
	}

	if len(spy.deleted) != 1 {
		t.Fatalf("Monitor.Delete 应被调用 1 次，实际 %d", len(spy.deleted))
	}
	if spy.deleted[0] == nil {
		t.Fatal("Monitor.Delete 收到 nil 文档")
	}
	if _, ok := coll.dataset.Get("a"); ok {
		t.Fatal("文档应已从 dataset 移除")
	}
}

// 注册表在零值 Collection 上就能用：Set 要能就地把 nil map 初始化出来
// （这正是 Monitors 的 Get/Set/Remove 用指针接收者的原因）
func TestMonitorsSetOnNilMap(t *testing.T) {
	coll := &Collection{} //刻意不走 NewColl，monitors 为 nil
	spy := &monTestSpy{}

	coll.Monitors().Set("spy", spy)
	if got := coll.Monitors().Get("spy"); got != Monitor(spy) {
		t.Fatalf("Set 之后应能 Get 回来，实际 %v", got)
	}

	coll.Monitors().Remove("spy")
	if got := coll.Monitors().Get("spy"); got != nil {
		t.Fatalf("Remove 之后应取不到，实际 %v", got)
	}
}

// 没注册过任何观察者时 Get/Remove 不得 panic
func TestMonitorsGetOnNilMapReturnsNil(t *testing.T) {
	coll := &Collection{}
	if got := coll.Monitors().Get("nope"); got != nil {
		t.Fatalf("空注册表 Get 应返回 nil，实际 %v", got)
	}
	coll.Monitors().Remove("nope") //不 panic 即通过
}

// 多订阅能力必须保住：注册两个观察者，Save 时都要收到
func TestMonitorsFanOutToMultiple(t *testing.T) {
	coll := NewColl()
	a, b := &monTestSpy{}, &monTestSpy{}
	coll.Monitors().Set("a", a)
	coll.Monitors().Set("b", b)

	if err := coll.Insert(&monTestItem{OID: "x", Val: 1}); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if err := coll.Save(monTestWriter{}); err != nil {
		t.Fatalf("save: %v", err)
	}

	if len(a.inserted) != 1 || len(b.inserted) != 1 {
		t.Fatalf("两个观察者都应收到通知，实际 a=%d b=%d", len(a.inserted), len(b.inserted))
	}
}

// Cursor 改用新注册 API 后仍能收集新插入的文档；释放后能再取到一个可用的
func TestCursorStillCollectsInsertedDocs(t *testing.T) {
	coll := NewColl()
	cur := coll.Cursor("user1")
	if cur == nil {
		t.Fatal("Cursor 不该为 nil")
	}
	//框架自用的注册项应当存在，且业务看不到那个 key（不导出）
	if coll.Monitors().Get(collectionMonitorKey) == nil {
		t.Fatal("Cursor 应已注册进观察者注册表")
	}

	if err := coll.Insert(&monTestItem{OID: "x", Val: 1}); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if err := coll.Save(monTestWriter{}); err != nil {
		t.Fatalf("save: %v", err)
	}
	if len(cur.items) == 0 {
		t.Fatal("Cursor 应收集到新插入的文档")
	}

	coll.onCursorRelease()
	if coll.Monitors().Get(collectionMonitorKey) != nil {
		t.Fatal("释放后注册项应被摘除")
	}
	if next := coll.Cursor("user2"); next == nil {
		t.Fatal("释放后应能再取到一个可用的 Cursor")
	}
}
