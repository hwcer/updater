package updater

import (
	"encoding/json"
	"testing"

	"github.com/hwcer/cosgo/values"
)

// TestErrItemNotEnough_Args 🔴 道具不足必须把「哪个道具、差多少」以结构化参数下发。
//
// 从前这些参数只被 Sprintf 拍进 Data 的英文文案里,客户端要提示「XX不足」就只能去解字符串,
// 而文案随时会改。现在参数一律只进 Args,文案退化成固定标识。
//
// 判据落在 Args 的**值**上,不能只断言 len==3：这些 helper 收的是变参,一旦谁把
// WithArgs(args...) 写成 WithArgs(args) 就嵌套成 [[1001 5 2]],客户端取不到道具ID,
// 而错误本身照样有 Args、照样非空,不看值就发现不了。
func TestErrItemNotEnough_Args(t *testing.T) {
	err := ErrItemNotEnough(1001, 5, 2)

	msg, ok := err.(*values.Message)
	if !ok {
		t.Fatalf("应为 *values.Message,实得 %T", err)
	}
	//文案是固定标识,不再带参数 —— 参数一律走 Args
	if msg.Error() != "Item Not Enough" {
		t.Fatalf("Error() = %q, want %q —— 参数不该出现在文案里", msg.Error(), "Item Not Enough")
	}
	if len(msg.Args) != 3 {
		t.Fatalf("Args = %v, want [1001 5 2] —— 长度不对多半是嵌套了一层", msg.Args)
	}
	want := []int32{1001, 5, 2}
	for i, w := range want {
		if got := values.ParseInt32(msg.Args[i]); got != w {
			t.Fatalf("Args[%d] = %v(%T), want %d", i, msg.Args[i], msg.Args[i], w)
		}
	}

	//走一趟客户端拿到的报文
	b, e := json.Marshal(msg)
	if e != nil {
		t.Fatal(e)
	}
	t.Logf("客户端收到: %s", b)
}

// TestErrItemNotExist_Args 单实参 helper 也走 Args,文案里不留占位符。
func TestErrItemNotExist_Args(t *testing.T) {
	msg, ok := ErrItemNotExist("oid-123").(*values.Message)
	if !ok {
		t.Fatal("应为 *values.Message")
	}
	if len(msg.Args) != 1 || msg.Args[0] != "oid-123" {
		t.Fatalf("Args = %v, want [oid-123]", msg.Args)
	}
}
