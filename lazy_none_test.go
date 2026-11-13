package updater

import (
	"testing"

	"github.com/hwcer/updater/dataset"
)

// 懒加载 = 全部 handle 按 RAMTypeNone 运行（newStatement 一处改写）：
// 不预拉；写照常 Select 记账 → Submit 收敛的 Data 按需拉触碰的键（写会触发读——
// 已放弃「写绝不读库」）→ Parse 内存读改写（绝对值，校验/溢出/Result/幂等重试全保留）
// → save $set；release 丢 dataset 不驻留。handle 零改动，$inc 非幂等问题不存在。

type offNoneBulk struct {
	updates []dataset.Update
	wheres  [][]any
}

func (b *offNoneBulk) Update(model any, data any, where ...any) {
	b.updates = append(b.updates, data.(dataset.Update))
	b.wheres = append(b.wheres, where)
}
func (b *offNoneBulk) Insert(model any, documents ...any) {}
func (b *offNoneBulk) Delete(model any, where ...any)     {}
func (b *offNoneBulk) Submit() error                      { return nil }
func (b *offNoneBulk) String() string                     { return "off_none_bulk" }

func TestLazyRAMTypeNone(t *testing.T) {
	mg := New()
	model := &capCollModel{rows: map[string]*capRow{"c1": {Id: "c1", IID: 1001, Val: 50}}}
	bulk := &offNoneBulk{}
	mg.Config.IType = func(int32) int32 { return 9003 }
	mg.Config.ParseId = func(_ *Updater, _ string) (int32, error) { return 1001, nil }
	mg.Config.BulkWrite = func(*Updater) BulkWrite { return bulk }
	if err := mg.Register(ParserTypeCollection, RAMTypeAlways, model, capIType{id: 9003, stacked: true}); err != nil {
		t.Fatal(err)
	}
	u := mg.New(&managePlayer{uid: "off_none_uid"})
	if _, err := u.Loading(true); err != nil {
		t.Fatal(err)
	}
	defer u.Release()
	u.Reset()

	coll := u.Collection(9003)
	coll.Sub("c1", 5) //Select(c1) 记账——随 Submit 收敛按需拉取（写触发读，按需不整表）
	if u.Error != nil {
		t.Fatalf("懒加载写不得置错: %v", u.Error)
	}
	if _, err := u.Submit(); err != nil {
		t.Fatal(err)
	}

	//绝对值语义：拉到库值 50 → Parse 读改写 → $set 45（可安全重发，无 $inc 双记问题）
	if len(bulk.updates) != 1 {
		t.Fatalf("应有一条 $set 载荷, 实际 %d: %+v", len(bulk.updates), bulk.updates)
	}
	if v := bulk.updates[0]["val"]; dataset.ParseInt64(v) != 45 {
		t.Fatalf("应为绝对值 45, 实际 %v", v)
	}
	if w := bulk.wheres[0]; len(w) != 1 || w[0] != "c1" {
		t.Fatalf("where 应为 [c1], 实际 %v", w)
	}

	//RAMTypeNone：release 丢 dataset 不驻留
	u.Release()
	if coll.dataset != nil {
		t.Fatal("懒加载 release 后 dataset 应丢弃（RAMTypeNone 语义, 不驻留）")
	}

	//真上线（无参 Loading）：清 lazy 恢复原始形态——GetRAMType 回注册值、幂等分支
	//tryLoad 重新全量预拉并置 StatusLoaded、release 回常驻语义
	u.Reset()
	if _, err := u.Loading(); err != nil {
		t.Fatal(err)
	}
	if !u.Loaded() {
		t.Fatal("上线应重新预拉并置 StatusLoaded")
	}
	if coll.GetRAMType() != RAMTypeAlways {
		t.Fatalf("GetRAMType 应回原始注册值 Always, 实际 %v", coll.GetRAMType())
	}
	u.Release()
	if coll.dataset == nil {
		t.Fatal("在线档 release 后 dataset 应常驻（原始 RAMTypeAlways 语义, ram 字段从未被改写）")
	}
}
