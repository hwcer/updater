package updater

import (
	"strings"
	"testing"
	"time"
)

type okResetModel struct{}

func (*okResetModel) Reset(*Updater, time.Time) bool { return false }

// badResetModel 用旧签名(int64),模拟"接口改了签名但实现方没跟进"
type badResetModel struct{}

func (*badResetModel) Reset(*Updater, int64) bool { return false }

type noResetModel struct{}

// TestVerifyOptionalCatchesStaleSignature 签名写错必须被检出
//
// ModelReset 靠类型断言识别,签名不符时断言静默失败:编译能过、Reset 永不被调用。
// 这正是接口改签名时最危险的失效方式,故注册期必须拦下。
func TestVerifyOptionalCatchesStaleSignature(t *testing.T) {
	if err := verifyOptional(&okResetModel{}); err != nil {
		t.Errorf("正确签名不应报错: %v", err)
	}
	if err := verifyOptional(&noResetModel{}); err != nil {
		t.Errorf("未实现该接口(无同名方法)不应报错: %v", err)
	}
	err := verifyOptional(&badResetModel{})
	if err == nil {
		t.Fatal("旧签名(int64)必须被检出,否则 ModelReset 会静默失效")
	}
	if !strings.Contains(err.Error(), "Reset") {
		t.Errorf("错误信息应指明是哪个方法: %v", err)
	}
	t.Logf("检出: %v", err)
}
