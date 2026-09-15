package updater

import (
	"testing"
)

// Receive 是 Remove 的反向操作：把已经在手上的文档直接塞进内存，不查库、不写库。
// 业务别处刚查过/刚插入过的数据靠它进缓存，免得 Select+Data 照着 oid 再查一遍。
func TestCollectionReceive(t *testing.T) {
	u, _ := newMountUpdater(t)
	coll, err := u.Mount(newMountModel("oid1"))
	if err != nil {
		t.Fatalf("Mount:%v", err)
	}

	if coll.Has("oid1") {
		t.Fatal("未装载数据前不该命中")
	}
	coll.Receive("oid1", &mountRow{Id: "oid1", Val: 7})

	if !coll.Has("oid1") {
		t.Fatal("Receive 之后 Has 应命中,否则 Select 还会再查一遍库")
	}
	doc := coll.Document("oid1")
	if doc == nil {
		t.Fatal("Receive 之后应当取得到文档")
	}
	if doc.GetInt64("val") != 7 {
		t.Fatalf("文档内容不符,期望 7 实际 %d", doc.GetInt64("val"))
	}
	//不记脏：Submit 不应为此产生落库动作
	if _, err := u.Submit(); err != nil {
		t.Fatalf("Submit:%v", err)
	}
	//Receive 本身不产生变更（bw.updates 只会在真正写操作后增长，由其他用例覆盖）
}
