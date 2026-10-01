package updater

import (
	"testing"

	"github.com/hwcer/logger"
)

// 🔴 Add/Sub 路由失败与 0/负值一律静默跳过 + DEBUG 日志,不置 u.Error:
// 策划表数值列大量留空(bong 主线重打 0/0 奖励、道具未填满等)属正常态,
// 上抛会中断整次结算提交。排查靠 DEBUG 日志里的完整调用链(callerChain)。

func newMinimalUpdater(t *testing.T) *Updater {
	t.Helper()
	mg := New()
	//7777 映射到未注册的 IType 9999,构造真实的"模型未注册"路由失败
	mg.Config.IType = func(iid int32) int32 {
		if iid == 7777 {
			return 9999
		}
		return 9001
	}
	mg.Config.BulkWrite = func(*Updater) BulkWrite { return &bwRetryBulk{} }
	if err := mg.Register(ParserTypeValues, RAMTypeAlways, &bwValuesModel{data: map[int32]int64{100: 50}}, testIType(9001)); err != nil {
		t.Fatal(err)
	}
	u := mg.New(&managePlayer{uid: "addsub_route_uid"})
	if err := u.Loading(); err != nil {
		t.Fatal(err)
	}
	return u
}

// 0/负值与未注册模型(路由失败)一律跳过:不置错、不改数据、Submit 照常成功
func TestAddSubInvalidArgsSkipSilently(t *testing.T) {
	u := newMinimalUpdater(t)
	defer u.Release()
	u.Reset()

	// iid<=0
	u.Add(0, 10)
	u.Sub(0, 10)
	u.Add(-3, 5)
	// num<=0
	u.Add(100, 0)
	u.Sub(100, 0)
	u.Add(100, -5)
	u.Sub(100, -5)
	// 路由失败:未注册模型的正数 iid
	u.Add(7777, 10)
	u.Sub(7777, 10)

	if u.Error != nil {
		t.Fatalf("以上全部应静默跳过,不得置 u.Error:%v", u.Error)
	}
	if got := u.Val(100); got != 50 {
		t.Fatalf("数据应保持不变,期望 50 实际 %d", got)
	}
	if _, err := u.Submit(); err != nil {
		t.Fatalf("存在跳过操作时 Submit 不应失败:%v", err)
	}
}

// DEBUG 开启时跳过路径照常工作,日志路径(捕栈)不得 panic
func TestAddSubDebugSkipNoPanic(t *testing.T) {
	old := logger.GetLevel()
	logger.SetLevel(logger.LevelDebug)
	defer logger.SetLevel(old)

	u := newMinimalUpdater(t)
	defer u.Release()
	u.Reset()

	u.Add(0, 10)
	u.Sub(7777, 10)
	if u.Error != nil {
		t.Fatalf("DEBUG 级下跳过同样不得置错:%v", u.Error)
	}
	if got := u.Val(100); got != 50 {
		t.Fatalf("数据应保持不变,实际 %d", got)
	}
}

// 正常路径不受影响:已注册 iid 的 Add/Sub 照常工作
func TestAddSubRegisteredModelWorks(t *testing.T) {
	u := newMinimalUpdater(t)
	defer u.Release()
	u.Reset()

	u.Sub(100, 30)
	if err := u.Verify(); err != nil {
		t.Fatalf("verify:%v", err)
	}
	if got := u.Val(100); got != 20 {
		t.Fatalf("扣除后应为 20,实际 %d", got)
	}
	u.Add(100, 5)
	if err := u.Verify(); err != nil {
		t.Fatalf("verify:%v", err)
	}
	if got := u.Val(100); got != 25 {
		t.Fatalf("加回后应为 25,实际 %d", got)
	}
}
