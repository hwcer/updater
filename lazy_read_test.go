package updater

import (
	"strings"
	"testing"

	"github.com/hwcer/updater/dataset"
)

// 本文件钉懒加载的读契约：懒加载不预拉，**读之前业务自己 Select + Data** 拉要用的键，
// 未拉的键读到零值（这是契约，不是缺陷——「读出来判定再写绝对值」的业务漏了 Select 就会
// 用零值覆盖库）。扫描类接口靠 Select 拉不全，懒加载一律报错，不静默给半份。

type offDocRow struct {
	Uid   string `json:"uid" bson:"_id"`
	Gold  int64  `json:"gold" bson:"gold"`
	Score int64  `json:"score" bson:"score"`
}

// offDocModel 模仿项目 Role.Getter：按字段拉也 Reset 整个文档
type offDocModel struct {
	db offDocRow
}

func (m *offDocModel) TableName() string { return "off_doc" }
func (m *offDocModel) New(*Updater) any  { return &offDocRow{} }
func (m *offDocModel) IType(int32) int32 { return 9005 }
func (m *offDocModel) Field(_ *Updater, iid int32) (string, error) {
	if iid == 2001 {
		return "gold", nil
	}
	return "score", nil
}
func (m *offDocModel) Getter(_ *Updater, data *dataset.Document, keys []string) error {
	row := &offDocRow{Uid: m.db.Uid}
	if len(keys) == 0 {
		*row = m.db
	}
	for _, k := range keys {
		switch k {
		case "gold":
			row.Gold = m.db.Gold
		case "score":
			row.Score = m.db.Score
		}
	}
	data.Reset(row)
	return nil
}

type offReadBulk struct{ updates []any }

func (b *offReadBulk) Update(_ any, data any, _ ...any) { b.updates = append(b.updates, data) }
func (b *offReadBulk) Insert(any, ...any)               {}
func (b *offReadBulk) Delete(any, ...any)               {}
func (b *offReadBulk) Submit() error                    { return nil }
func (b *offReadBulk) String() string                   { return "off_read_bulk" }

// Document 先 Select+Data 再读 → 判定 → 写绝对值；不 Select 直接读是零值（契约）
func TestLazyDocumentSelectBeforeRead(t *testing.T) {
	m := &offDocModel{db: offDocRow{Uid: "off_doc_uid", Gold: 100, Score: 30}}
	bulk := &offReadBulk{}
	mg := New()
	mg.Config.IType = func(int32) int32 { return 9005 }
	mg.Config.BulkWrite = func(*Updater) BulkWrite { return bulk }
	if err := mg.Register(ParserTypeDocument, RAMTypeAlways, m, testIType(9005)); err != nil {
		t.Fatal(err)
	}
	u := mg.New(&managePlayer{uid: "off_doc_uid"})
	if _, err := u.Loading(true); err != nil {
		t.Fatal(err)
	}
	u.Reset()
	defer u.Release()

	doc := u.Document(9005)
	if v := doc.Val("score"); v != 0 {
		t.Fatalf("懒加载不预拉: 未 Select 读应为零值, 实际 %d", v)
	}
	doc.Select("score")
	if err := u.Data(); err != nil {
		t.Fatal(err)
	}
	score := doc.Val("score")
	if score != 30 {
		t.Fatalf("Select+Data 后应读到库值 30, 实际 %d", score)
	}
	doc.Set("score", score+1)
	if _, err := u.Submit(); err != nil {
		t.Fatal(err)
	}
	up, _ := bulk.updates[len(bulk.updates)-1].(map[string]any)
	if dataset.ParseInt64(up["score"]) != 31 {
		t.Fatalf("应写绝对值 31, 实际 %v", up)
	}
	if _, ok := up["gold"]; ok {
		t.Fatalf("未改动的 gold 不应进载荷(否则零值覆盖库), 实际 %v", up)
	}
}

// Document.Range 遍历全部字段 = 扫描：懒加载只拉过 Select 的字段，报错不迭代
func TestLazyDocumentRangeRejected(t *testing.T) {
	m := &offDocModel{db: offDocRow{Uid: "off_doc_uid", Gold: 100, Score: 30}}
	mg := New()
	mg.Config.IType = func(int32) int32 { return 9005 }
	mg.Config.BulkWrite = func(*Updater) BulkWrite { return &offReadBulk{} }
	if err := mg.Register(ParserTypeDocument, RAMTypeAlways, m, testIType(9005)); err != nil {
		t.Fatal(err)
	}
	u := mg.New(&managePlayer{uid: "off_doc_uid"})
	if _, err := u.Loading(true); err != nil {
		t.Fatal(err)
	}
	u.Reset()
	defer u.Release()
	u.Document(9005).Range(func(string, any) bool { t.Fatal("懒加载 Document.Range 不得迭代"); return true })
	if u.Error == nil || !strings.Contains(u.Error.Error(), "懒加载禁止") {
		t.Fatalf("懒加载 Document.Range 应置错, 实际 %v", u.Error)
	}
}

// Collection / Values 先 Select+Data 再读
func TestLazyPointSelectBeforeRead(t *testing.T) {
	coll, vals := newPromoModels()
	vals.data[5] = 7
	u := newPromoManage(t, coll, vals).New(&managePlayer{uid: "point_uid"})
	if _, err := u.Loading(true); err != nil {
		t.Fatal(err)
	}
	u.Reset()
	defer u.Release()

	c, v := u.Collection(9003), u.Values(9004)
	if c.Val("c1") != 0 || v.Val(5) != 0 {
		t.Fatal("懒加载不预拉: 未 Select 读应为零值")
	}
	c.Select("c1")
	v.Select(5)
	if err := u.Data(); err != nil {
		t.Fatal(err)
	}
	if got := c.Val("c1"); got != 50 {
		t.Fatalf("Collection Select+Data 后应读到 50, 实际 %d", got)
	}
	if got := v.Val(5); got != 7 {
		t.Fatalf("Values Select+Data 后应读到 7, 实际 %d", got)
	}
	if coll.full != 0 || vals.full != 0 {
		t.Fatalf("懒加载不得整表拉, 实际 coll.full=%d vals.full=%d", coll.full, vals.full)
	}
}

// 扫描类接口懒加载报错：不迭代 / 返回空，并置 u.Error，不静默给半份
func TestLazyScanRejected(t *testing.T) {
	cases := []struct {
		name string
		scan func(u *Updater)
	}{
		{"Collection.Range", func(u *Updater) {
			u.Collection(9003).Range(func(string, *dataset.Document) bool { t.Fatal("懒加载 Range 不得迭代"); return true })
		}},
		{"Collection.Len", func(u *Updater) {
			if u.Collection(9003).Len() != 0 {
				t.Fatal("懒加载 Len 应返回 0")
			}
		}},
		{"Values.All", func(u *Updater) {
			if u.Values(9004).All() != nil {
				t.Fatal("懒加载 All 应返回 nil")
			}
		}},
		{"Values.Range", func(u *Updater) {
			u.Values(9004).Range(func(int32, int64) bool { t.Fatal("懒加载 Range 不得迭代"); return true })
		}},
		{"Collection.Cursor", func(u *Updater) {
			if c := u.Collection(9003).Cursor("val"); c == nil {
				t.Fatal("懒加载 Cursor 应返回非 nil 的空游标")
			}
			if u.Collection(9003).Dataset().Len() == 0 {
				t.Fatal("前置条件: 数据集里应已拉进一行(空游标不能是因为本来就空)")
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			coll, vals := newPromoModels()
			u := newPromoManage(t, coll, vals).New(&managePlayer{uid: "scan_uid"})
			if _, err := u.Loading(true); err != nil {
				t.Fatal(err)
			}
			u.Reset()
			defer u.Release()
			u.Collection(9003).Select("c1") //先拉进一行:扫描若放行就会返回这「半份」
			if err := u.Data(); err != nil {
				t.Fatal(err)
			}
			tc.scan(u)
			if u.Error == nil || !strings.Contains(u.Error.Error(), "懒加载禁止") {
				t.Fatalf("懒加载扫描应置错, 实际 %v", u.Error)
			}
		})
	}
}
