package updater

import (
	"errors"
	"testing"
)

// 🔴 Config.BulkWrite 没配的话所有句柄的落库都会**静默失效** ——
// save 报出的 ErrBulkWriteNotInit 会被 submit 吞成一行 Alert，玩家一路正常玩、
// 一行数据都没落库，重启才发现。所以在玩家数据第一次加载时就拦下来。
func TestLoadingRequiresBulkWrite(t *testing.T) {
	old := Config.BulkWrite
	Config.BulkWrite = nil
	t.Cleanup(func() { Config.BulkWrite = old })

	u := New(&mountPlayer{uid: "uid1"})
	err := u.Loading()
	if err == nil {
		t.Fatal("Config.BulkWrite 没配时 Loading 应当报错,否则落库会静默失效")
	}
	if !errors.Is(err, ErrBulkWriteNotInit) && err.Error() != ErrBulkWriteNotInit.Error() {
		t.Fatalf("应当是 ErrBulkWriteNotInit,实际 %v", err)
	}
	if u.Loader() {
		t.Fatal("检查没过就不该置 StatusInit —— 否则下次 Loading 会直接返回、当成已加载")
	}
}
