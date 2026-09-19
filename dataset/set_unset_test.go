package dataset

import "testing"

// 🔴 同 key 先 Unset 后 Set:最终语义是 Set。
// Set 必须清掉 unset 标记,否则 Save 同时产出 $set + $unset,
// mongo 单条 update 不允许对同一字段二者并存(would create a conflict)。
func TestValuesSetClearsUnset(t *testing.T) {
	v := NewValues()
	v.Set(1, 5)
	v.Unset(1)
	if _, unsets := v.Save(); len(unsets) != 1 {
		t.Fatalf("仅 Unset 时应产出 1 项 unset,实际 %v", unsets)
	}

	v = NewValues()
	v.Set(1, 5)
	v.Unset(1)
	v.Set(1, 7) //改主意了:最终要的是 7
	dirty, unsets := v.Save()
	if len(unsets) != 0 {
		t.Fatalf("Unset 后又 Set 不该再产出 unset,实际 %v", unsets)
	}
	if got := dirty[1]; got != 7 {
		t.Fatalf("落库载荷应为 {1:7},实际 %v", dirty)
	}
	if v.Val(1) != 7 {
		t.Fatalf("内存读应为 7,实际 %d", v.Val(1))
	}
}

func TestDocumentSetClearsUnset(t *testing.T) {
	doc, _ := newUnsetTestDoc() //unsetTestDoc.BreakLv 是已注册字段

	doc.Unset("breaklv")
	doc.Set("breaklv", int32(9))
	dirty, unsets, _ := doc.Save()
	if len(unsets) != 0 {
		t.Fatalf("Unset 后又 Set 不该再产出 unset,实际 %v", unsets)
	}
	if v, ok := dirty["breaklv"]; !ok || v != int32(9) {
		t.Fatalf("落库载荷应含 breaklv=9,实际 %v", dirty)
	}
}
