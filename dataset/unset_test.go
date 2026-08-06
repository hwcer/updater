package dataset

import "testing"

// unsetTestDoc 故意**不实现** ModelUnset，走 unsetMemory 的通用回落分支。
// 业务模型（如 Role）实现了 ModelUnset 会走前一个分支，这里守的是没实现的那批。
type unsetTestDoc struct {
	BreakLv    int32
	Goods      map[int32]int64
	SoulRelics map[int32]*unsetTestItem
}

type unsetTestItem struct {
	Lv   int32
	Name string
}

func newUnsetTestDoc() (*Document, *unsetTestDoc) {
	m := &unsetTestDoc{
		BreakLv:    5,
		Goods:      map[int32]int64{10001: 99, 10002: 7},
		SoulRelics: map[int32]*unsetTestItem{1: {Lv: 7, Name: "a"}, 2: {Lv: 8}},
	}
	return NewDoc(m), m
}

// 🔴 Unset 子键必须同步清掉内存。
//
// 改之前 unsetMemory 走 schema.SetValue，对含 "." 的 key 必然报 "field not exist"、
// 只打一条 Alert 就过去了：落库的 unset 列表照常带上这些 key（库里删掉了），
// 内存却纹丝不动。Role 是 RAMTypeAlways 常驻内存，这种分叉会一直错到重启。
func TestUnsetSubKeyClearsMemory(t *testing.T) {
	doc, m := newUnsetTestDoc()

	doc.Unset("goods.10001")
	if _, ok := m.Goods[10001]; ok {
		t.Errorf("goods.10001 应从内存删除，实际 %v", m.Goods)
	}
	if m.Goods[10002] != 7 {
		t.Errorf("同 map 内其它键不该受影响，实际 %v", m.Goods)
	}

	doc.Unset("soulrelics.1")
	if _, ok := m.SoulRelics[1]; ok {
		t.Errorf("soulrelics.1 应从内存删除，实际 %v", m.SoulRelics)
	}
	if m.SoulRelics[2] == nil {
		t.Error("soulrelics.2 不该受影响")
	}
}

// 多级路径：只清到目标字段，不误删整个 map 元素
func TestUnsetDeepSubKeyClearsMemory(t *testing.T) {
	doc, m := newUnsetTestDoc()

	doc.Unset("soulrelics.1.lv")
	if m.SoulRelics[1] == nil {
		t.Fatal("只清字段，不该把整个 map 元素删掉")
	}
	if m.SoulRelics[1].Lv != 0 {
		t.Errorf("soulrelics.1.lv 应置零，实际 %d", m.SoulRelics[1].Lv)
	}
	if m.SoulRelics[1].Name != "a" {
		t.Errorf("同元素内其它字段不该受影响，实际 %q", m.SoulRelics[1].Name)
	}
}

// 整字段清除（原本就能工作）+ 落库列表仍然完整：内存与库两边动作必须一致
func TestUnsetWholeFieldAndSaveList(t *testing.T) {
	doc, m := newUnsetTestDoc()

	doc.Unset("breaklv")
	if m.BreakLv != 0 {
		t.Errorf("breaklv 应置零，实际 %d", m.BreakLv)
	}

	doc.Unset("goods.10001")
	_, unsets := doc.Save()
	if len(unsets) != 2 {
		t.Fatalf("落库 unset 列表应有 2 项，实际 %v", unsets)
	}
	got := map[string]bool{}
	for _, k := range unsets {
		got[k] = true
	}
	if !got["breaklv"] || !got["goods.10001"] {
		t.Fatalf("落库 unset 列表内容不对: %v", unsets)
	}
}
