package updater

import (
	"testing"

	"github.com/hwcer/updater/dataset"
)

type collReceiveRow struct {
	Id  string `json:"_id" bson:"_id"`
	Val int64  `json:"val" bson:"val"`
}

// Receive 是 Remove 的反向操作：把已经在手上的文档直接塞进内存，不查库、不写库。
// 业务别处刚查过/刚插入过的数据靠它进缓存，免得 Select+Data 照着 oid 再查一遍。
func TestCollectionReceive(t *testing.T) {
	//只装 dataset：Receive/Has/Document 都不碰 statement 与 Updater
	coll := &Collection{dataset: dataset.NewColl()}

	if coll.Has("oid1") {
		t.Fatal("空集合不该命中")
	}
	coll.Receive("oid1", &collReceiveRow{Id: "oid1", Val: 7})

	if !coll.Has("oid1") {
		t.Fatal("Receive 之后 Has 应命中,否则 Select 还会再查一遍库")
	}
	doc := coll.Document("oid1")
	if doc == nil {
		t.Fatal("Receive 之后应当取得到文档")
	}
	if got := doc.GetInt64("val"); got != 7 {
		t.Fatalf("val 期望 7 实际 %d", got)
	}

	//只进内存,不记脏
	if d := coll.dataset.Dirty(); len(d) != 0 {
		t.Fatalf("Receive 不该记脏,实际 %v", d)
	}
}
