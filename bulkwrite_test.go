package updater

import (
	"errors"
	"testing"
	"time"

	"github.com/hwcer/updater/dataset"
)

// 本文件钉的是 BulkWrite 的保留-重试-熔断语义:
// 提交失败队列跨请求保留(Release 不再丢弃),写库成功才清理;
// 连续失败达到 BulkWriteMaxFails 开启灾难保护,数据库恢复后自动解除。

// bwRetryBulk 前 failN 次 Submit 失败,之后成功;failN<0 永远失败
type bwRetryBulk struct {
	submits  int
	failN    int
	onSubmit func() //提交钩子,测试记录顺序用
}

func (b *bwRetryBulk) Submit() error {
	b.submits++
	if b.onSubmit != nil {
		b.onSubmit()
	}
	if b.failN < 0 || b.submits <= b.failN {
		return errors.New("db down")
	}
	return nil
}
func (b *bwRetryBulk) Update(model any, data any, where ...any) {}
func (b *bwRetryBulk) Insert(model any, documents ...any)       {}
func (b *bwRetryBulk) Delete(model any, where ...any)           {}
func (b *bwRetryBulk) String() string                           { return "bw_retry" }

// bwValuesModel 假数值模型:库在内存,Setter 只入队不失败(模拟真实 Mongo 集成形态)
type bwValuesModel struct {
	data     map[int32]int64
	setter   int
	onGetter func()
}

func (m *bwValuesModel) TableName() string { return "bw_values" }

func (m *bwValuesModel) Getter(_ *Updater, data *dataset.Values, keys []int32) error {
	if m.onGetter != nil {
		m.onGetter()
	}
	//Receive 直写持久层:Set 写 dirty,失败请求 Release 清 dirty 会把已加载值一起丢掉
	//(RAMTypeAlways 不再重查库,丢的就是永久丢)
	if len(keys) == 0 {
		for k, v := range m.data {
			data.Receive(k, v)
		}
		return nil
	}
	for _, k := range keys {
		if v, ok := m.data[k]; ok {
			data.Receive(k, v)
		}
	}
	return nil
}

func (m *bwValuesModel) Setter(_ *Updater, bw BulkWrite, dirty dataset.Data, _ []int32) error {
	m.setter++
	bw.Update(m, dirty)
	return nil
}

func newBWRetryUpdater(t *testing.T, bw BulkWrite, model *bwValuesModel) *Updater {
	t.Helper()
	mg := New()
	mg.Config.IType = func(int32) int32 { return 9001 }
	mg.Config.BulkWrite = func(*Updater) BulkWrite { return bw }
	if err := mg.Register(ParserTypeValues, RAMTypeAlways, model, testIType(9001)); err != nil {
		t.Fatal(err)
	}
	u := mg.New(&managePlayer{uid: "bw_retry_uid"})
	if err := u.Loading(); err != nil {
		t.Fatal(err)
	}
	return u
}

// 🔴 提交失败后队列跨请求保留:Release 不丢弃;下次请求 Reset 先重试;
// 数据库恢复后欠账与新增数据一起提交;成功后队列清理,不再重复提交。
func TestBulkWriteRetainedAcrossRequests(t *testing.T) {
	model := &bwValuesModel{data: map[int32]int64{}}
	bw := &bwRetryBulk{failN: 2} //前两次失败
	u := newBWRetryUpdater(t, bw, model)

	//请求1:提交失败,Release 后队列必须保留
	u.Reset()
	u.Add(100, 10)
	if _, err := u.Submit(); err == nil {
		t.Fatal("第一次 Submit 应失败")
	}
	u.Release()
	if u.bulkWrite == nil {
		t.Fatal("Release 不应丢弃提交失败的队列")
	}

	//请求2:Reset 重试欠账仍失败(DB未恢复),随后 DB 恢复,Submit 把欠账与新增一起提交
	u.Reset()
	if bw.submits != 2 {
		t.Fatalf("Reset 应重试遗留队列,Submit 次数=%d", bw.submits)
	}
	if u.bulkWrite == nil {
		t.Fatal("重试失败后队列应继续保留")
	}
	bw.failN = 0 //数据库恢复
	u.Add(100, 5)
	if _, err := u.Submit(); err != nil {
		t.Fatalf("恢复后 Submit 应成功:%v", err)
	}
	if bw.submits != 3 || model.setter != 2 {
		t.Fatalf("恢复后应重发同一队列(submits=%d setter=%d)", bw.submits, model.setter)
	}
	if u.bulkWrite != nil {
		t.Fatal("写库成功后应立即清理队列")
	}

	//请求3:队列已清,Reset 不应再产生提交
	u.Reset()
	if bw.submits != 3 {
		t.Fatalf("队列已清,Reset 不应再提交,Submit 次数=%d", bw.submits)
	}
	u.Release()
}

// 写库成功的常规路径:Submit 成功即清理,后续请求从空队列开始
func TestBulkWriteClearedOnSuccess(t *testing.T) {
	model := &bwValuesModel{data: map[int32]int64{}}
	bw := &bwRetryBulk{}
	u := newBWRetryUpdater(t, bw, model)

	u.Reset()
	u.Add(100, 1)
	if _, err := u.Submit(); err != nil {
		t.Fatalf("Submit:%v", err)
	}
	if u.bulkWrite != nil {
		t.Fatal("提交成功后应立即清理 bulkWrite")
	}
	u.Release()

	u.Reset()
	if bw.submits != 1 {
		t.Fatalf("队列已清,下次 Reset 不应重复提交,实际 %d", bw.submits)
	}
	u.Release()
}

// 🔴 Reload 必须先清欠账再重载:提交失败时不重载、不清内存、保留队列
func TestReloadFlushesPendingBeforeReloading(t *testing.T) {
	model := &bwValuesModel{data: map[int32]int64{}}
	bw := &bwRetryBulk{failN: -1} //数据库持续不可写
	u := newBWRetryUpdater(t, bw, model)

	u.Reset()
	u.Add(100, 10)
	if _, err := u.Submit(); err == nil {
		t.Fatal("Submit 应失败")
	}
	u.Release()

	//DB不可写:Reload 直接失败,不重载、保留队列
	if err := u.Reload(); err == nil {
		t.Fatal("数据库不可写时 Reload 应失败")
	}
	if u.bulkWrite == nil {
		t.Fatal("Reload 失败时应保留队列")
	}

	//DB恢复:先 submit 清欠账,后 getter 重载,顺序不能反
	var order []string
	bw.failN = 0
	bw.onSubmit = func() { order = append(order, "submit") }
	pre := model.onGetter
	model.onGetter = func() {
		order = append(order, "getter")
		if pre != nil {
			pre()
		}
	}
	if err := u.Reload(); err != nil {
		t.Fatalf("恢复后 Reload 应成功:%v", err)
	}
	if len(order) != 2 || order[0] != "submit" || order[1] != "getter" {
		t.Fatalf("Reload 应先清欠账(submit)再重载(getter),实际 %v", order)
	}
	if u.bulkWrite != nil {
		t.Fatal("Reload 成功提交后应清理队列")
	}
}

// 🔴 Destroy 失败保留队列与实体,重试 Destroy 重发同一批;成功才完成销毁
func TestDestroyRetriesAfterFailure(t *testing.T) {
	model := &bwValuesModel{data: map[int32]int64{}}
	bw := &bwRetryBulk{failN: -1}
	u := newBWRetryUpdater(t, bw, model)

	u.Reset()
	u.Add(100, 10)
	if _, err := u.Submit(); err == nil {
		t.Fatal("Submit 应失败")
	}
	u.Release()

	if err := u.Destroy(); err == nil {
		t.Fatal("数据库不可写时 Destroy 应失败")
	}
	if u.bulkWrite == nil {
		t.Fatal("Destroy 失败应保留队列供重试")
	}
	if u.entity == nil || u.handles == nil {
		t.Fatal("Destroy 失败不应提前拆实体")
	}

	bw.failN = 0
	if err := u.Destroy(); err != nil {
		t.Fatalf("重试 Destroy 应成功:%v", err)
	}
	if u.bulkWrite != nil {
		t.Fatal("销毁成功后应清理队列")
	}
	if u.entity != nil || u.handles != nil {
		t.Fatal("销毁成功后应完成清理")
	}
}

// 🔴 连续提交失败达到 BulkWriteMaxFails 开启灾难保护(Reset 拒绝服务),
// 数据库恢复(DatabaseMonitoring 探测为健康)后自动解除
func TestConsecutiveSubmitFailsTripDisaster(t *testing.T) {
	oldMax := BulkWriteMaxFails
	oldDM := DatabaseMonitoring
	BulkWriteMaxFails = 2
	monCalled := false
	DatabaseMonitoring = func() bool {
		monCalled = true
		return true
	}
	t.Cleanup(func() {
		BulkWriteMaxFails = oldMax
		DatabaseMonitoring = oldDM
		disaster.Store(0)
		bulkWriteFails.Store(0)
		monitoring.Store(false)
	})

	model := &bwValuesModel{data: map[int32]int64{}}
	bw := &bwRetryBulk{failN: -1}
	u := newBWRetryUpdater(t, bw, model)

	//两个失败请求累计达到阈值(请求2的 Reset 重试也计一次)
	for i := 0; i < 2; i++ {
		u.Reset()
		u.Add(100, 1)
		u.Submit()
		u.Release()
	}
	if disaster.Load() != 1 {
		t.Fatalf("连续失败 %d 次(阈值 %d)应开启灾难保护", bulkWriteFails.Load(), BulkWriteMaxFails)
	}

	//灾难保护下 Reset 拒绝服务
	u.Reset()
	if u.Error == nil {
		t.Fatal("灾难保护下 Reset 应置 ErrServerDeniedService")
	}
	u.Release()

	//监控协程探测数据库健康后自动解除
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && disaster.Load() != 0 {
		time.Sleep(50 * time.Millisecond)
	}
	if disaster.Load() != 0 {
		t.Fatal("数据库恢复后灾难保护应自动解除")
	}
	if !monCalled {
		t.Fatal("应启动数据库监控探测")
	}
}
