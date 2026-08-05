package updater

import (
	"strings"
	"testing"

	"github.com/hwcer/updater/dataset"
)

// fieldTestDoc 只为 Field() 的字段名解析服务：一个标量字段 + 一个 map 字段。
// 字段名故意用 PascalCase，落库名(DBName)是它的小写形式，用来验证子键 key 不被规范化。
type fieldTestDoc struct {
	BreakLv    int32
	SoulRelics map[int32]int64
}

// newFieldTestDocument 构造一个仅够跑 Field() 的 Document：
// Field 只用到 dataset(取 Schema) 与 Updater(出错时置 Error)，不需要完整的 Model 注册。
func newFieldTestDocument() *Document {
	doc := &Document{dataset: dataset.NewDoc(&fieldTestDoc{})}
	doc.statement.Updater = &Updater{}
	return doc
}

// 子键路径的根字段必须校验存在。
//
// 此前含 "." 的 key 在 Field() 里直接 return、不做任何校验，根字段名写错会一路放行到
// dataset.Document.Set，被那里的 `if !doc.Has(k) { return }` 静默丢弃——调用方拿不到
// 错误，还以为写成功了。
func TestDocumentFieldSubKeyValidatesRoot(t *testing.T) {
	doc := newFieldTestDocument()

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

// 🔴 子键 key 必须原样返回，不得做名称规范化。
//
// Name() 返回 JSName()(PascalCase)，若把它拼回子键会得到 SoulRelics.1；而落库时
// cosmo 的 update.Transform 对含 "." 的 key 原样下发、不查 schema，结果是往库里写一个
// 大小写不符的野字段，真正的字段纹丝不动 —— 静默丢数据，比它要修的问题更严重。
func TestDocumentFieldSubKeyNotNormalized(t *testing.T) {
	doc := newFieldTestDocument()

	//含 mongo 风格的多级路径：无论几级，整条 key 都必须原样透传
	for _, key := range []string{"soulrelics.1", "SoulRelics.1", "soulrelics.1.2", "soulrelics.1.a.b"} {
		got, err := doc.Field(key)
		if err != nil {
			t.Errorf("合法子键 %q 不该报错: %v", key, err)
			continue
		}
		if got != key {
			t.Errorf("子键 key 被改写了: %q -> %q（落库会写出错误字段名）", key, got)
		}
	}
}

// 整字段仍按 schema 规范化成 JSName，行为不变。
func TestDocumentFieldWholeFieldStillNormalized(t *testing.T) {
	doc := newFieldTestDocument()

	got, err := doc.Field("breaklv")
	if err != nil {
		t.Fatalf("整字段 breaklv 应能解析: %v", err)
	}
	if !strings.EqualFold(got, "BreakLv") {
		t.Fatalf("整字段应规范化为 schema 名，实际 %q", got)
	}
}
