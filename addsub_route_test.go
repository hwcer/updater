package updater

import (
	"strings"
	"testing"
)

// 🔴 T4 回归:Add/Sub 路由失败不得静默丢弃——旧实现 ParseId 失败仅 Alert、
// 模型未注册仅 Debug,void API 调用方对"什么都没发生"零感知:
// u.Sub(金币, price) 静默不执行继续发货 = 刷道具;Add 静默丢失 = 付费未到账。

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

// 模型未注册的 iid:Add/Sub 必须置 u.Error,SubErr/AddErr 必须返回 error
func TestAddSubUnregisteredModelSurfacesError(t *testing.T) {
	u := newMinimalUpdater(t)
	defer u.Release()
	u.Reset()

	if err := u.SubErr(7777, 10); err == nil {
		t.Fatal("未注册模型的 SubErr 应返回错误(旧实现静默忽略,等于凭空发货)")
	}
	if err := u.AddErr(7777, 10); err == nil {
		t.Fatal("未注册模型的 AddErr 应返回错误")
	}

	//void 版本:路由失败落 u.Error,Submit 感知失败
	u.Reset()
	u.Sub(7777, 10)
	if u.Error == nil {
		t.Fatal("void Sub 路由失败应置 u.Error")
	}
	if _, err := u.Submit(); err == nil {
		t.Fatal("存在路由失败错误时 Submit 应失败,请求不得假装成功")
	}
}

// 正常路径不受影响:已注册 iid 的 Add/Sub 照常工作
func TestAddSubRegisteredModelWorks(t *testing.T) {
	u := newMinimalUpdater(t)
	defer u.Release()
	u.Reset()

	if err := u.SubErr(100, 30); err != nil {
		t.Fatalf("已注册模型 SubErr 不应报错:%v", err)
	}
	if err := u.Verify(); err != nil {
		t.Fatalf("verify:%v", err)
	}
	if got := u.Val(100); got != 20 {
		t.Fatalf("扣除后应为 20,实际 %d", got)
	}
	if err := u.AddErr(100, 5); err != nil {
		t.Fatalf("已注册模型 AddErr 不应报错:%v", err)
	}
	if err := u.Verify(); err != nil {
		t.Fatalf("verify:%v", err)
	}
	if got := u.Val(100); got != 25 {
		t.Fatalf("加回后应为 25,实际 %d", got)
	}
}

// 解析失败语义自查:合法 iid 正常工作
func TestAddErrParseSanity(t *testing.T) {
	u := newMinimalUpdater(t)
	defer u.Release()
	u.Reset()

	if err := u.AddErr(100, 1); err != nil {
		t.Fatalf("合法 iid 不应报错:%v", err)
	}
	_ = strings.Contains("sanity", "check")
}
