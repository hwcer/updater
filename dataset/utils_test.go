package dataset

import "testing"

// TestParseIntNamedType 命名整数类型（proto 枚举同构）必须可解析——曾静默返回 0，
// 造成「落库对、内存写零」的分叉（2026-09-29 dev21 joinMode 7008 根因）。
type namedEnum int32

func TestParseIntNamedType(t *testing.T) {
	if v := ParseInt32(namedEnum(3)); v != 3 {
		t.Fatalf("ParseInt32(namedEnum(3)) = %v, want 3", v)
	}
	if v, ok := TryParseInt64(namedEnum(7)); !ok || v != 7 {
		t.Fatalf("TryParseInt64(namedEnum(7)) = (%v,%v), want (7,true)", v, ok)
	}
	if v := ParseInt64(typedUint32(9)); v != 9 {
		t.Fatalf("ParseInt64(typedUint32) = %v, want 9", v)
	}
	if _, ok := TryParseInt64("x"); ok {
		t.Fatal("string 不可解析应 ok=false")
	}
}

type typedUint32 uint32
