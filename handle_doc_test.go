package updater

import (
	"testing"

	"github.com/hwcer/updater/dataset"
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

// 🔴 多级路径逐段规范化成 json 名，map 键原样保留。
//
// updater 内部统一用 json 名：op.Field / op.Result 直接就是发给客户端的 key，
// 落库名由 cosmo 在边界换（Update.Transform / Selector.Projection 都走 schema.DBName）。
// 本测试的模型不带任何 tag，故 json 名 = Go 字段名（PascalCase）。
//
// 反过来 map 键的大小写有业务含义，动了就是写错地方。逐段判定由 schema 负责。
func TestDocumentFieldPathNormalizedPerSegment(t *testing.T) {
	doc := newFieldTestDocument()

	cases := map[string]string{
		"soulrelics.1":    "SoulRelics.1",    //根字段换名
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
	doc := newFieldTestDocument()

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

// schema 取不到时必须报错。
//
// 旧实现 Name() 在 Schema() 返回 nil 时走完 if 直接 return，得到 ("", nil)——
// Field 把空字段名当成解析成功交给调用方，写入落到一个空 key 上。
func TestDocumentFieldSchemaUnavailable(t *testing.T) {
	doc := &Document{name: "unittest"}
	doc.statement.Updater = &Updater{}

	if key, err := doc.Field("breaklv"); err == nil {
		t.Fatalf("dataset 未初始化时 Field 应报错，实际返回 key=%q", key)
	}
}

// 🔴 写接口不得清掉已挂起的 Updater.Error。
//
// 旧代码 `field, this.Updater.Error = this.Field(k)` 无条件赋值：字段解析成功时把 nil
// 写回 Updater.Error，抹掉之前的错误，紧接着的 WriteAble 便误判为可写，
// 本该被整体拦下的写入照样落库。
func TestDocumentWriteKeepsPendingError(t *testing.T) {
	doc := newFieldTestDocument()
	pending := ErrArgsIllegal(1, 1)
	doc.Updater.Error = pending

	if op := doc.Set("breaklv", 1); op != nil {
		t.Error("Updater 已处于错误状态，Set 不应产出操作")
	}
	if doc.Updater.Error != pending {
		t.Fatalf("挂起的错误被覆盖了: %v", doc.Updater.Error)
	}
}

// 整字段按 schema 规范化成 json 名。
//
// 🔴 不是 DBName：op.Field / op.Result 同时喂客户端 payload 与落库两条路，
// 带 json 名则客户端直通，落库那侧由 cosmo 在边界统一换成 DBName。
// 反过来带 DBName 就要求客户端认库名，且 Collection 的 op.Result 还得再转一次。
func TestDocumentFieldWholeFieldNormalizedToJSName(t *testing.T) {
	doc := newFieldTestDocument()

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
