package updater

import (
	"sync"
	"testing"

	"github.com/hwcer/updater/dataset"
	"github.com/hwcer/updater/hamster"
)

// fieldTestDoc 只为 Field() 的字段名解析服务。字段名故意用 PascalCase、不带 bson 标签，
// 落库名(DBName)是它的小写形式，用来验证各段是否被正确换名。
// 三种容器都要有：标量、map 到结构体(可继续下钻)、map 到标量(下钻到此为止)。
type fieldTestDoc struct {
	BreakLv    int32
	SoulRelics map[int32]*fieldTestRelic
	Goods      map[int32]int64
}

type fieldTestRelic struct {
	Lv int32
}

type fieldTestDocModel struct{}

func (m *fieldTestDocModel) TableName() string      { return "doc_field_test" }
func (m *fieldTestDocModel) New(*hamster.Store) any { return &fieldTestDoc{} }
func (m *fieldTestDocModel) Getter(_ *hamster.Store, d *dataset.Document, _ []string) error {
	d.Reset(&fieldTestDoc{})
	return nil
}
func (m *fieldTestDocModel) Setter(*hamster.Store, hamster.BulkWrite, dataset.Update, []string) error {
	return nil
}

var (
	fieldDocOnce     sync.Once
	fieldDocNoneOnce sync.Once
)

// newFieldTestStore 注册一次（注册表无反注册）并返回载好主档的 Store
func newFieldTestStore(t *testing.T) *hamster.Store {
	t.Helper()
	fieldDocOnce.Do(func() {
		if err := hamster.RegisterDocument("doc_field_test", hamster.RAMTypeAlways, &fieldTestDocModel{}); err != nil {
			t.Fatalf("RegisterDocument:%v", err)
		}
	})
	s := hamster.New(&mountPlayer{uid: "doc_field_test"})
	if err := s.Loading(); err != nil {
		t.Fatalf("Loading:%v", err)
	}
	s.Reset()
	return s
}

// RAMTypeNone 版：Release 后 dataset/schema 被置空（schema-unavailable 场景用）
func newFieldTestStoreNone(t *testing.T) *hamster.Store {
	t.Helper()
	fieldDocNoneOnce.Do(func() {
		if err := hamster.RegisterDocument("doc_field_test_none", hamster.RAMTypeNone, &fieldTestDocModel{}); err != nil {
			t.Fatalf("RegisterDocument(none):%v", err)
		}
	})
	s := hamster.New(&mountPlayer{uid: "doc_field_test_none"})
	if err := s.Loading(); err != nil {
		t.Fatalf("Loading:%v", err)
	}
	s.Reset()
	return s
}

// 子键路径的根字段必须校验存在。
//
// 此前含 "." 的 key 在 Field() 里直接 return、不做任何校验，根字段名写错会一路放行到
// dataset.Document.Set，被那里的 `if !doc.Has(k) { return }` 静默丢弃——调用方拿不到
// 错误，还以为写成功了。
func TestDocumentFieldSubKeyValidatesRoot(t *testing.T) {
	s := newFieldTestStore(t)
	doc := s.Document("doc_field_test")

	//多级路径(mongo 风格 a.b.c)同样按第一个点取根字段
	for _, key := range []string{"nosuchfield.1", "nosuchfield.1.2", "nosuchfield.a.b.c"} {
		if _, err := doc.Field(key); err == nil {
			t.Errorf("根字段不存在的路径 %q 应报错，否则会被 dataset 层静默丢弃", key)
		}
	}
	if _, err := doc.Field("nosuchfield"); err == nil {
		t.Error("不存在的整字段应报错(原有行为，回归)")
	}
}

// 🔴 多级路径逐段规范化成 json 名，map 键原样保留。
// 本测试的模型不带任何 tag，故 json 名 = Go 字段名（PascalCase）。
func TestDocumentFieldPathNormalizedPerSegment(t *testing.T) {
	s := newFieldTestStore(t)
	doc := s.Document("doc_field_test")

	cases := map[string]string{
		"soulrelics.1":    "SoulRelics.1", //根字段换名
		"SoulRelics.1":    "SoulRelics.1",
		"soulrelics.1.lv": "SoulRelics.1.Lv", //穿过 map 键继续下钻到值类型的字段
		"SoulRelics.1.Lv": "SoulRelics.1.Lv",
		"goods.10001":     "Goods.10001",
	}
	for key, want := range cases {
		got, err := doc.Field(key)
		if err != nil {
			t.Errorf("合法路径 %q 不该报错: %v", key, err)
			continue
		}
		if got != want {
			t.Errorf("路径 %q 期望 %q，实际 %q（落库会写出错误字段名）", key, want, got)
		}
	}
}

// 路径越界必须报错：以前只校验根字段，中间段和末段写错这一层查不出来，
// 会一路放行到 dataset.Document.Set 被静默丢弃。
func TestDocumentFieldRejectsBadPath(t *testing.T) {
	s := newFieldTestStore(t)
	doc := s.Document("doc_field_test")

	bad := map[string]string{
		"SoulRelics.1.Nope": "map 值结构体里没有这个字段",
		"Goods.1.Lv":        "map 值是标量，不能再往下钻",
		"BreakLv.1":         "标量字段不能再往下钻",
	}
	for key, why := range bad {
		if got, err := doc.Field(key); err == nil {
			t.Errorf("路径 %q 应报错(%s)，实际返回 %q", key, why, got)
		}
	}
}

// schema 取不到时必须报错：dataset 不可用时 Field 不能把空字段名当成解析成功。
// RAMTypeNone 的句柄在 Release 后 dataset 被置空 —— 旧实现此刻 Field 返回 ("", nil)。
func TestDocumentFieldSchemaUnavailable(t *testing.T) {
	s := newFieldTestStoreNone(t)
	s.Release() //RAMTypeNone → dataset = nil, schema = nil

	doc := s.Document("doc_field_test_none")
	if _, err := doc.Field("breaklv"); err == nil {
		t.Fatal("dataset 不可用时 Field 应报错")
	}
}

// 🔴 写接口不得清掉已挂起的 Store.Error。
//
// 旧代码无条件赋值：字段解析成功时把 nil 写回，抹掉之前的错误，紧接着的
// WriteAble 便误判为可写，本该被整体拦下的写入照样落库。
func TestDocumentWriteKeepsPendingError(t *testing.T) {
	s := newFieldTestStore(t)
	doc := s.Document("doc_field_test")
	pending := ErrArgsIllegal(1, 1)
	s.Error = pending

	if op := doc.Set("breaklv", 1); op != nil {
		t.Error("已处于错误状态，Set 不应产出操作")
	}
	if s.Error != pending {
		t.Fatalf("挂起的错误被覆盖了: %v", s.Error)
	}
}

// 整字段按 schema 规范化成 json 名（不是 DBName）。
func TestDocumentFieldWholeFieldNormalizedToJSName(t *testing.T) {
	s := newFieldTestStore(t)
	doc := s.Document("doc_field_test")

	for _, key := range []string{"breaklv", "BreakLv"} {
		got, err := doc.Field(key)
		if err != nil {
			t.Fatalf("整字段 %q 应能解析: %v", key, err)
		}
		if got != "BreakLv" {
			t.Fatalf("整字段 %q 应规范化为 json 名 BreakLv，实际 %q", key, got)
		}
	}
}
