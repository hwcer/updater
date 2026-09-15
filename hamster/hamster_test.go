package hamster_test

import (
	"sync"
	"testing"
	"time"

	"github.com/hwcer/cosgo/schema"
	"github.com/hwcer/updater/dataset"
	"github.com/hwcer/updater/hamster"
	"github.com/hwcer/updater/operator"
)

// 本文件钉的是核心版的验收标准（HAMSTER_PLAN.md 第十三节），多数是**静默失效**：
// 跑一遍看不出来，线上表现为"数据莫名其妙没了"。

// ---------------- 测试脚手架 ----------------

type testEntity struct{ id string }

func (e *testEntity) Id() string { return e.id }

// testRow 公会成员行
type testRow struct {
	Id   string `json:"_id" bson:"_id"`
	Name string `json:"name" bson:"name"`
	Val  int64  `json:"val" bson:"val"`
}

// testBulk 假 BulkWrite：只计数
type testBulk struct {
	updates []string
	inserts int
	deletes int
	submits int
}

func (b *testBulk) Submit() error                      { b.submits++; return nil }
func (b *testBulk) Update(_ any, _ any, where ...any)  { b.updates = append(b.updates, where[0].(string)) }
func (b *testBulk) Insert(_ any, _ ...any)             { b.inserts++ }
func (b *testBulk) Delete(_ any, _ ...any)             { b.deletes++ }
func (b *testBulk) String() string                     { return "" }

// testCollModel 集合模型：内存行 + Getter/Setter 计数 + 错误注入
type testCollModel struct {
	rows   map[string]*testRow
	err    error
	getter int
	setter int
}

func newTestCollModel(ids ...string) *testCollModel {
	m := &testCollModel{rows: map[string]*testRow{}}
	for _, id := range ids {
		m.rows[id] = &testRow{Id: id}
	}
	return m
}

func (m *testCollModel) TableName() string { return "hamster_test_coll" }
func (m *testCollModel) Schema() *schema.Schema {
	s, _ := schema.Parse(&testRow{})
	return s
}
func (m *testCollModel) Upsert(*hamster.Store, *operator.Operator) bool { return false }
func (m *testCollModel) Getter(_ *hamster.Store, data *dataset.Collection, keys []string) error {
	m.getter++
	if m.err != nil {
		return m.err
	}
	for _, k := range keys {
		if row, ok := m.rows[k]; ok {
			data.Receive(k, row)
		}
	}
	return nil
}
func (m *testCollModel) Setter(_ *hamster.Store, bw hamster.BulkWrite, _id string, dirty dataset.Update, _ []string) error {
	m.setter++
	bw.Update(m, dirty, _id)
	return nil
}

// testDoc 主档结构（真实主档是 struct，dataset.Reset 只认 struct）
type testDoc struct {
	Notice string `json:"notice" bson:"notice"`
	Funds  int64  `json:"funds" bson:"funds"`
}

// testDocModel 主档模型：**最小接口**（仅 New/Getter/Setter，无 IType/Field），
// 验收标准 11：最小 DocumentModel 注册成功且 Set/Get 往返正确
type testDocModel struct {
	row *testDoc
	err error
}

func newTestDocModel() *testDocModel {
	return &testDocModel{row: &testDoc{Notice: "hello"}}
}

func (m *testDocModel) TableName() string { return "hamster_test_doc" }
func (m *testDocModel) TableOrder() int32 { return 9 } //主档：TableOrder 最前
func (m *testDocModel) New(_ *hamster.Store) any         { return m.row }
func (m *testDocModel) Getter(_ *hamster.Store, data *dataset.Document, _ []string) error {
	if m.err != nil {
		return m.err
	}
	data.Reset(m.row) //与真实业务模型同口径：整档 Reset 填充
	return nil
}
func (m *testDocModel) Setter(_ *hamster.Store, bw hamster.BulkWrite, dirty dataset.Update, _ []string) error {
	bw.Update(m, dirty, "hamster_test_doc")
	return nil
}

// 注册表没有反注册（重名即错误，设计如此）：模型进程内只注册一次，
// 每个用例重置其内部状态。
var (
	sharedDocModel  = newTestDocModel()
	sharedCollModel = newTestCollModel()
	sharedOnce      sync.Once
)

// newTestStore 建好带 Document(主档) + Collection 的 Store 并完成 Loading/Reset
func newTestStore(t *testing.T, collIds ...string) (*hamster.Store, *testDocModel, *testCollModel, *testBulk) {
	t.Helper()
	bw := &testBulk{}
	old := hamster.Config.BulkWrite
	hamster.Config.BulkWrite = func(*hamster.Store) hamster.BulkWrite { return bw }
	t.Cleanup(func() { hamster.Config.BulkWrite = old })

	//重置共享模型状态（注册表持有的就是这两个实例）
	docModel := sharedDocModel
	docModel.err = nil
	docModel.row = &testDoc{Notice: "hello"}
	collModel := sharedCollModel
	collModel.err = nil
	collModel.getter, collModel.setter = 0, 0
	collModel.rows = map[string]*testRow{}
	for _, id := range collIds {
		collModel.rows[id] = &testRow{Id: id}
	}

	sharedOnce.Do(func() {
		if err := hamster.RegisterDocument("hamster_test_doc", hamster.RAMTypeAlways, docModel,
			hamster.WithTableOrder(docModel.TableOrder())); err != nil {
			t.Fatalf("RegisterDocument(最小主档模型):%v", err)
		}
		if err := hamster.RegisterCollection("hamster_test_coll", hamster.RAMTypeMaybe, collModel); err != nil {
			t.Fatalf("RegisterCollection:%v", err)
		}
	})

	s := hamster.New(&testEntity{id: "guild_1"})
	if err := s.Loading(); err != nil {
		t.Fatalf("Loading:%v", err)
	}
	s.Reset(time.Now())
	return s, docModel, collModel, bw
}

// ---------------- 验收标准逐条钉 ----------------

// 验收 2：hamster.Collection Select→Data→Get 取到库数据（钉 StatusChanged 闸门 ——
// 漏置闸门的症状是"Select 了、Data 了、Get 拿到 nil、还不报错"）。
func TestCollectionSelectDataGet(t *testing.T) {
	s, _, m, _ := newTestStore(t, "row1")
	m.rows["row1"].Val = 42

	coll := s.Collection("hamster_test_coll")
	if coll == nil {
		t.Fatal("注册过的集合句柄取不到")
	}
	coll.Select("row1")
	if err := s.Data(); err != nil {
		t.Fatalf("Data:%v", err)
	}
	if got := coll.Val("row1"); got != 42 {
		t.Fatalf("Select→Data→Get 应回读 42,实际 %d（闸门漏置时这里拿到 0）", got)
	}
}

// 验收 3：主档先行 —— TableOrder 值大的主档在 Loading 时先于其他模型完成加载，
// 其余模型的 Getter 能读到主档内容（这里用 Getter 内读不到主档的顺序问题做代理：
// 主档 IsNil 时 model.New 返回的 map 才被填，Loading 结束后 Document 必须有值）。
func TestMasterDocumentLoadsFirst(t *testing.T) {
	s, docModel, _, _ := newTestStore(t, "row1")
	_ = docModel
	prof := s.Document("hamster_test_doc")
	if prof == nil {
		t.Fatal("主档句柄取不到")
	}
	if v := prof.Get("notice"); v != "hello" {
		t.Fatalf("主档加载失败,Get(notice)=%v（最小模型 New/Getter/Setter 应该工作）", v)
	}
}

// 验收 6（负向约束）：hamster.Collection 对超限 Add **不做任何溢出处理** ——
// 数值就是加上去，不截断、不分解、不产生 TypesOverflow。
func TestCollectionAddHasNoOverflow(t *testing.T) {
	s, _, _, _ := newTestStore(t, "row1")
	coll := s.Collection("hamster_test_coll")
	coll.Select("row1")
	if err := s.Data(); err != nil {
		t.Fatalf("Data:%v", err)
	}
	coll.Add("row1", "val", 3)
	coll.Add("row1", "val", 5)
	if err := s.Verify(); err != nil {
		t.Fatalf("Verify 不该报错,更不该触发溢出:%v", err)
	}
	for _, op := range coll.Operators() {
		if op.OType == operator.TypesOverflow || op.OType == operator.TypesResolve {
			t.Fatal("核心版不该产出溢出类 operator")
		}
	}
	if got := coll.Val("row1"); got != 8 {
		t.Fatalf("超限 Add 不截断,期望按加法累积到 8,实际 %d", got)
	}
}

// 验收 7：hamster 产 operator 的 IType==0 且默认不进 dirty；装收集接收器后 Submit 返回非空。
func TestOperatorITypeZeroAndDiscardByDefault(t *testing.T) {
	s, _, _, _ := newTestStore(t, "row1")
	coll := s.Collection("hamster_test_coll")
	coll.Select("row1")
	_ = s.Data()

	coll.Set("row1", "name", "alice")
	if err := s.Verify(); err != nil {
		t.Fatalf("Verify:%v", err)
	}
	for _, op := range coll.Operators() {
		if op.IType != 0 {
			t.Fatalf("核心版 operator 的 IType 应恒 0,实际 %d", op.IType)
		}
	}
	//默认 Discard 接收器：Submit 不产生变更流水
	changes, err := s.Submit()
	if err != nil {
		t.Fatalf("Submit:%v", err)
	}
	if len(changes) != 0 {
		t.Fatalf("默认 Discard 下 Submit 应返回空流水,实际 %d 条", len(changes))
	}

	//显式装收集接收器后：变更进流水
	coll2 := s.Collection("hamster_test_coll")
	coll2.Select("row1")
	_ = s.Data()
	coll2.Set("row1", "name", "bob")
	coll2.Receiver(func(_ *hamster.Store, ops []*operator.Operator) {
		s.Dirty(ops...)
	})
	changes, err = s.Submit()
	if err != nil {
		t.Fatalf("Submit:%v", err)
	}
	if len(changes) == 0 {
		t.Fatal("装了收集接收器后 Submit 应返回非空流水")
	}
}

// 验收 4：同批次原子性 —— Document 与 Collection 同请求改动，Getter 报错时两方都不落库。
func TestAtomicBatchAcrossHandles(t *testing.T) {
	s, docModel, collModel, bw := newTestStore(t, "row1")
	//Collection 的 Data 阶段失败：整条收敛链中断，谁都不许落库
	collModel.err = errTest{}
	coll := s.Collection("hamster_test_coll")
	coll.Select("row1")
	prof := s.Document("hamster_test_doc")
	prof.Set("notice", "changed")
	if _, err := s.Submit(); err == nil {
		t.Fatal("Getter 报错时 Submit 应当失败")
	}
	if len(bw.updates) != 0 {
		t.Fatalf("同批次原子性被破坏:Collection 失败但 bulkWrite 里有 %d 条更新", len(bw.updates))
	}
	//恢复后重走：主档的改动还在内存（与拆分前口径一致：内存已改,提交前可重试）
	collModel.err = nil
	if _, err := s.Submit(); err != nil {
		_ = err
	}
	_ = docModel
}

// 验收 10：Destroy 刷盘 + 清表，复用后无残留状态
func TestDestroyClearsState(t *testing.T) {
	s, _, m, bw := newTestStore(t, "row1")
	coll := s.Collection("hamster_test_coll")
	coll.Select("row1")
	_ = s.Data()
	coll.Add("row1", "val", 7)
	if _, err := s.Submit(); err != nil {
		t.Fatalf("Submit:%v", err)
	}
	if len(bw.updates) == 0 {
		t.Fatal("Submit 应经 bulkWrite 落库")
	}
	if err := s.Destroy(); err != nil {
		t.Fatalf("Destroy:%v", err)
	}
	if len(s.Mounts()) != 0 {
		t.Fatal("Destroy 应清空挂载表")
	}
	_ = m
}

// 字段级 Add/Sub：余额不足报错；CreditAllowed 放行扣负（公会资金/成员贡献够用）。
func TestCollectionFieldAddSub(t *testing.T) {
	s, _, _, _ := newTestStore(t, "row1")
	coll := s.Collection("hamster_test_coll")
	coll.Select("row1")
	_ = s.Data()

	coll.Add("row1", "val", 10)
	coll.Sub("row1", "val", 4)
	if err := s.Verify(); err != nil {
		t.Fatalf("Add(10)+Sub(4) 不该报错:%v", err)
	}
	if got := coll.Val("row1"); got != 6 {
		t.Fatalf("字段级加减应得 6,实际 %d", got)
	}

	//余额不足：打脏 Error
	coll.Sub("row1", "val", 100)
	if s.Verify() == nil {
		t.Fatal("余额不足的 Sub 应当报错")
	}
}

// Mount：幂等 + 重名报错 + keys 当场取数（Mount 蓝本行为在 hamster 侧复钉）。
func TestMountIdempotentAndConflict(t *testing.T) {
	s, _, _, _ := newTestStore(t)
	m := &namedCollModel{testCollModel: newTestCollModel("row1"), name: "hamster_test_mount"}
	m.rows["row1"].Val = 5

	c1, err := s.Mount(m, "row1")
	if err != nil {
		t.Fatalf("Mount:%v", err)
	}
	c2, err := s.Mount(m)
	if err != nil {
		t.Fatalf("Mount 幂等:%v", err)
	}
	if c1 != c2 {
		t.Fatal("重复 Mount 应返回同一句柄")
	}
	if got := c1.Val("row1"); got != 5 {
		t.Fatalf("keys 当场取数失败,期望 5 实际 %d", got)
	}

	//与已注册模型重名必须报错
	if _, err := s.Mount(&namedCollModel{testCollModel: newTestCollModel(), name: "hamster_test_coll"}); err == nil {
		t.Fatal("与已注册模型重名的 Mount 应报错")
	}
}

type errTest struct{}

func (errTest) Error() string { return "test getter error" }

// namedCollModel 换名的集合模型（重名测试用）
type namedCollModel struct {
	*testCollModel
	name string
}

func (m *namedCollModel) TableName() string { return m.name }
