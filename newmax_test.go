package updater

import (
	"fmt"
	"math"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hwcer/cosgo/schema"
	"github.com/hwcer/updater/dataset"
	"github.com/hwcer/updater/operator"
)

// 本文件钉住:不可叠加道具批量创建上限(Options.NewMax)、
// IType/ParseId 缺配置时的降级(不 panic)、三类句柄 Add 的 int64 溢出拦截。

// ---------- 测试模型 ----------

type capRow struct {
	Id  string `json:"_id" bson:"_id"`
	IID int32  `json:"iid" bson:"iid"`
	Val int64  `json:"val" bson:"val"`
}

// capIType stacked=false 走 NewEquip 逐个创建;stacked=true 走单文档累加
type capIType struct {
	id      int32
	stacked bool
}

func (c capIType) ID() int32                           { return c.id }
func (c capIType) Stacked(int32) bool                  { return c.stacked }
func (c capIType) GetOID(*Updater, int32) (oid string) { return "c1" }

var capRowSeq atomic.Int64

func (c capIType) New(u *Updater, op *operator.Operator) (any, error) {
	//_id 必须唯一:集合 Insert 对重复 id 报 already exist
	return &capRow{Id: fmt.Sprintf("c_%d", capRowSeq.Add(1)), IID: op.IID}, nil
}

// capCollModel 库在内存;Getter 全量加载(nil keys)或按 keys 加载
type capCollModel struct {
	rows map[string]*capRow
}

func (m *capCollModel) TableName() string { return "cap_coll" }
func (m *capCollModel) Schema() *schema.Schema {
	s, _ := schema.Parse(&capRow{})
	return s
}
func (m *capCollModel) Upsert(*Updater, *operator.Operator) bool { return false }
func (m *capCollModel) Getter(_ *Updater, data *dataset.Collection, keys []string) error {
	if len(keys) == 0 {
		for k, r := range m.rows {
			data.Receive(k, r)
		}
		return nil
	}
	for _, k := range keys {
		if r, ok := m.rows[k]; ok {
			data.Receive(k, r)
		}
	}
	return nil
}
func (m *capCollModel) Setter(_ *Updater, bw BulkWrite, _id string, dirty dataset.Update, unset []string) error {
	bw.Update(m, dirty, _id)
	return nil
}

func newCapUpdater(t *testing.T, model *capCollModel, its ...capIType) *Updater {
	t.Helper()
	mg := New()
	mg.Config.IType = func(int32) int32 { return 9001 }
	mg.Config.BulkWrite = func(*Updater) BulkWrite { return &bwRetryBulk{} }
	args := make([]IType, len(its))
	for i, it := range its {
		args[i] = it
	}
	if err := mg.Register(ParserTypeCollection, RAMTypeAlways, model, args...); err != nil {
		t.Fatal(err)
	}
	u := mg.New(&managePlayer{uid: "cap_uid"})
	if err := u.Loading(); err != nil {
		t.Fatal(err)
	}
	return u
}

// ---------- NewMax 上限 ----------

// 🔴 不可叠加道具批量创建受 Options.NewMax 限制:默认 999,超限整个操作被拒绝;
// 可叠加道具不受限。
func TestNewMaxCapNonStackable(t *testing.T) {
	model := &capCollModel{rows: map[string]*capRow{}}
	u := newCapUpdater(t, model, capIType{id: 9001, stacked: false})

	//默认上限 999:申请 1000 件被拒
	u.Reset()
	u.Add(700, 1000)
	if _, err := u.Submit(); err == nil {
		t.Fatal("默认 NewMax=999 下批量创建 1000 件应被拒绝")
	} else if !strings.Contains(err.Error(), "new max exceeded") {
		t.Fatalf("应为 NewMax 超限错误,实际 %v", err)
	}
	u.Release()

	//边界:恰好 999 件放行(数据集常驻,文档跨请求累积)
	u.Reset()
	u.Add(700, 999)
	if _, err := u.Submit(); err != nil {
		t.Fatalf("999 件应放行:%v", err)
	}
	if got := u.Collection("cap_coll").Count(700); got != 999 {
		t.Fatalf("边界请求后应有 999 个文档,实际 %d", got)
	}
	u.Release()

	//配置收窄为 5:6 件拒绝,5 件放行并真实创建 5 个文档
	u.Manage().Config.NewMax = 5
	u.Reset()
	u.Add(700, 6)
	if _, err := u.Submit(); err == nil {
		t.Fatal("NewMax=5 下批量创建 6 件应被拒绝")
	}
	u.Release()

	u.Reset()
	u.Add(700, 5)
	if _, err := u.Submit(); err != nil {
		t.Fatalf("NewMax=5 下创建 5 件应放行:%v", err)
	}
	if got := u.Collection("cap_coll").Count(700); got != 1004 {
		t.Fatalf("累计应为 999+5=1004 个文档,实际 %d", got)
	}
	u.Release()

	//可叠加道具不受 NewMax 限制:单次 Add 一万个也放行(单文档数值)
	model2 := &capCollModel{rows: map[string]*capRow{"c1": {Id: "c1", IID: 800, Val: 1}}}
	u2 := newCapUpdater(t, model2, capIType{id: 9001, stacked: true})
	u2.Reset()
	u2.Add(800, 10000)
	if _, err := u2.Submit(); err != nil {
		t.Fatalf("可叠加道具不应受 NewMax 限制:%v", err)
	}
	u2.Release()
}

// 🔴 Options.IType 未配置(手工构造 &Options{} 绕过 New 默认值)时,
// u.Add / u.Val 走告警+忽略路径,不得 panic
func TestNilITypeDegradesInsteadOfPanic(t *testing.T) {
	mg := New()
	mg.Config = &Options{BulkWrite: func(*Updater) BulkWrite { return &bwRetryBulk{} }} //手工构造,IType/ParseId 均为 nil
	if err := mg.Register(ParserTypeValues, RAMTypeAlways, &bwValuesModel{data: map[int32]int64{}}, testIType(9001)); err != nil {
		t.Fatal(err)
	}
	u := mg.New(&managePlayer{uid: "nil_itype_uid"})
	if err := u.Loading(); err != nil {
		t.Fatal(err)
	}
	u.Reset()
	u.Add(100, 1) //不得 panic;操作被忽略
	if _, err := u.Submit(); err != nil {
		t.Fatalf("Submit 不应失败:%v", err)
	}
	if got := u.Val(100); got != 0 {
		t.Fatalf("操作应被忽略,Val 应为 0,实际 %d", got)
	}
	u.Release()
}

// ---------- int64 溢出拦截 ----------

// 🔴 Values:Add 到接近 MaxInt64 时拒绝操作,不再包装成负数静默腐蚀余额
func TestValuesAddOverflowRejects(t *testing.T) {
	model := &bwValuesModel{data: map[int32]int64{100: math.MaxInt64 - 5}}
	u := newBWRetryUpdater(t, &bwRetryBulk{}, model)
	if got := u.Val(100); got != math.MaxInt64-5 {
		t.Fatalf("Loading 后 Val 应为 MaxInt64-5,实际 %d", got)
	}

	u.Reset()
	u.Add(100, 10)
	if _, err := u.Submit(); err == nil {
		t.Fatal("Add 溢出应拒绝操作")
	} else if !strings.Contains(err.Error(), "args illegal") {
		t.Fatalf("应为参数非法错误,实际 %v", err)
	}
	u.Release()

	//边界内正常累加不受影响
	u.Reset()
	u.Add(100, 5)
	if _, err := u.Submit(); err != nil {
		t.Fatalf("边界内 Add 应成功:%v", err)
	}
	if got := u.Val(100); got != math.MaxInt64 {
		t.Fatalf("应累加到 MaxInt64,实际 %d", got)
	}
	u.Release()
}

// 🔴 Collection(可叠加):同样的溢出拦截
func TestCollectionAddOverflowRejects(t *testing.T) {
	model := &capCollModel{rows: map[string]*capRow{"c1": {Id: "c1", IID: 800, Val: math.MaxInt64 - 5}}}
	u := newCapUpdater(t, model, capIType{id: 9001, stacked: true})

	u.Reset()
	u.Add(800, 10)
	if _, err := u.Submit(); err == nil {
		t.Fatal("Collection Add 溢出应拒绝操作")
	}
	u.Release()
}

// 🔴 Document:同样的溢出拦截
func TestDocumentAddOverflowRejects(t *testing.T) {
	model := &capDocModel{doc: &capDocRow{Gold: math.MaxInt64 - 5}}
	mg := New()
	mg.Config.IType = func(int32) int32 { return 9001 }
	mg.Config.BulkWrite = func(*Updater) BulkWrite { return &bwRetryBulk{} }
	if err := mg.Register(ParserTypeDocument, RAMTypeAlways, model, testIType(9001)); err != nil {
		t.Fatal(err)
	}
	u := mg.New(&managePlayer{uid: "doc_overflow_uid"})
	if err := u.Loading(); err != nil {
		t.Fatal(err)
	}
	u.Reset()
	u.Document("capDocModel").Add("gold", 10)
	if _, err := u.Submit(); err == nil {
		t.Fatal("Document Add 溢出应拒绝操作")
	}
	u.Release()
}

type capDocModel struct {
	doc *capDocRow
}

type capDocRow struct {
	Gold int64 `json:"gold" bson:"gold"`
}

func (m *capDocModel) New(*Updater) any                      { return &capDocRow{} }
func (m *capDocModel) IType(int32) int32                     { return 9001 }
func (m *capDocModel) Field(*Updater, int32) (string, error) { return "gold", nil }
func (m *capDocModel) Getter(_ *Updater, data *dataset.Document, keys []string) error {
	if data.IsNil() {
		data.Reset(m.doc)
	}
	return nil
}
func (m *capDocModel) Setter(_ *Updater, bw BulkWrite, dirty dataset.Update, unset []string) error {
	bw.Update(m, dirty)
	return nil
}
