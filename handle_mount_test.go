package updater

import (
	"errors"
	"testing"

	"github.com/hwcer/cosgo/schema"
	"github.com/hwcer/updater/dataset"
	"github.com/hwcer/updater/operator"
)

// 本文件钉的多数是**静默失效**：跑一遍看不出来，线上表现为"数据莫名其妙没了"。

type mountPlayer struct{ uid string }

func (p *mountPlayer) Uid() string { return p.uid }

type mountRow struct {
	Id     string `json:"_id" bson:"_id"`
	IID    int32  `json:"iid" bson:"iid"`
	Val    int64  `json:"val" bson:"val"`
	Status int32  `json:"status" bson:"status"`
}

// mountModel 假的临时数据模型：库在内存里，只记录 Getter/Setter 被调了几次。
type mountModel struct {
	rows   map[string]*mountRow
	err    error //非空时 Getter 直接报错
	getter int
	setter int
}

func newMountModel(ids ...string) *mountModel {
	m := &mountModel{rows: map[string]*mountRow{}}
	for _, id := range ids {
		m.rows[id] = &mountRow{Id: id}
	}
	return m
}

func (m *mountModel) TableName() string { return "mount_test_table" }

func (m *mountModel) Schema() *schema.Schema {
	s, _ := schema.Parse(&mountRow{})
	return s
}

func (m *mountModel) Upsert(*Updater, *operator.Operator) bool { return false }

func (m *mountModel) Getter(_ *Updater, data *dataset.Collection, keys []string) error {
	m.getter++
	if m.err != nil {
		return m.err
	}
	for _, k := range keys {
		if row, ok := m.rows[k]; ok {
			data.Receive(k, row)
		}
	}
	return nil
}

func (m *mountModel) Setter(_ *Updater, bulkWrite BulkWrite, _id string, dirty dataset.Update, _ []string) error {
	m.setter++
	bulkWrite.Update(m, dirty, _id)
	return nil
}

// mountBulk 假的 BulkWrite：只计数，不写库。
type mountBulk struct {
	updates []string
	inserts int
	deletes int
	submits int
}

func (b *mountBulk) Submit() error { b.submits++; return nil }
func (b *mountBulk) Update(_ any, _ any, where ...any) {
	b.updates = append(b.updates, where[0].(string))
}
func (b *mountBulk) Insert(_ any, documents ...any) { b.inserts += len(documents) }
func (b *mountBulk) Delete(_ any, where ...any)     { b.deletes += len(where) }
func (b *mountBulk) String() string                 { return "" }

func newMountUpdater(t *testing.T) (*Updater, *mountBulk) {
	t.Helper()
	bw := &mountBulk{}
	old := Config.BulkWrite
	Config.BulkWrite = func(*Updater) BulkWrite { return bw }
	t.Cleanup(func() { Config.BulkWrite = old })

	u := New(&mountPlayer{uid: "mount_uid"})
	if err := u.Loading(); err != nil {
		t.Fatalf("Loading:%v", err)
	}
	u.Reset()
	return u, bw
}

// 🔴 Select 必须置 StatusChanged，否则 Updater.data() 开头那道闸门直接返回、Getter 永不调用。
// 症状是"Select 了、Data 了、Get 拿到 nil，还不报错"，排查方向会全歪到"库里没这条"。
func TestMountSelectTriggersGetter(t *testing.T) {
	u, _ := newMountUpdater(t)
	m := newMountModel("row1")
	coll, err := u.Mount(m)
	if err != nil {
		t.Fatalf("Mount:%v", err)
	}

	coll.Select("row1")
	if err = u.Data(); err != nil {
		t.Fatalf("Data:%v", err)
	}
	if m.getter == 0 {
		t.Fatal("Getter 未被调用：Select 没有置 StatusChanged，Updater.data() 空转返回")
	}
	if coll.Get("row1") == nil {
		t.Fatal("Select→Data 之后取不到数据")
	}
	//已在内存的 key 不重复查库
	coll.Select("row1")
	_ = u.Data()
	if m.getter != 1 {
		t.Fatalf("已在内存的 key 不应重复查库,Getter 调用次数=%d", m.getter)
	}
}

// Mount 带 key 时当场查库，等价于 Select + Data 一步到位。
func TestMountWithKeysLoadsImmediately(t *testing.T) {
	u, _ := newMountUpdater(t)
	m := newMountModel("row1", "row2")

	coll, err := u.Mount(m, "row1", "row2")
	if err != nil {
		t.Fatalf("Mount:%v", err)
	}
	if m.getter != 1 {
		t.Fatalf("带 key 的 Mount 应当场查一次库,Getter 调用次数=%d", m.getter)
	}
	if coll.Get("row1") == nil || coll.Get("row2") == nil {
		t.Fatal("Mount 返回时数据就该在手上")
	}

	//长命场景:后续请求反复这么调,已在内存的 key 不重复查库
	again, err := u.Mount(m, "row1")
	if err != nil {
		t.Fatalf("Mount 幂等:%v", err)
	}
	if again != coll {
		t.Fatal("重复 Mount 应返回同一句柄")
	}
	if m.getter != 1 {
		t.Fatalf("已在内存的 key 不该重复查库,Getter 调用次数=%d", m.getter)
	}

	//没带 key 就不该碰数据库
	u2, _ := newMountUpdater(t)
	m2 := newMountModel("row1")
	if _, err = u2.Mount(m2); err != nil {
		t.Fatalf("Mount:%v", err)
	}
	if m2.getter != 0 {
		t.Fatal("不带 key 的 Mount 不该预加载")
	}
}

// 挂载与取数是两码事：查库失败不影响挂载，句柄照常可用，重试一次就好。
func TestMountGetterErrorKeepsMount(t *testing.T) {
	u, _ := newMountUpdater(t)
	m := newMountModel("row1")
	m.err = errors.New("db down")

	coll, err := u.Mount(m, "row1")
	if err == nil {
		t.Fatal("Getter 报错时 Mount 应把错误返回出来")
	}
	if coll == nil {
		t.Fatal("查库失败不该连句柄一起吞掉:挂载本身是成功的")
	}
	if u.Mounted(m) != coll {
		t.Fatal("句柄应当已经挂上")
	}

	//重试:同一个句柄上再来一次就好
	m.err = nil
	coll.Select("row1")
	if err = coll.Data(); err != nil {
		t.Fatalf("重试取数:%v", err)
	}
	if coll.Get("row1") == nil {
		t.Fatal("重试之后应当取到数据")
	}
}

// Val 取数值字段（默认 dataset.Fields.VAL）；Count 按 iid 统计，且**只覆盖内存里的部分**。
func TestMountValAndCount(t *testing.T) {
	u, _ := newMountUpdater(t)
	m := newMountModel("row1", "row2", "row3")
	m.rows["row1"].IID, m.rows["row1"].Val = 101, 7
	m.rows["row2"].IID, m.rows["row2"].Val = 102, 3
	m.rows["row3"].IID, m.rows["row3"].Val = 101, 5

	coll, err := u.Mount(m, "row1", "row2") //故意不拉 row3
	if err != nil {
		t.Fatalf("Mount:%v", err)
	}

	if got := coll.Val("row1"); got != 7 {
		t.Fatalf("Val 应取 val 字段,期望 7 实际 %d", got)
	}
	if got := coll.Val("nosuchrow"); got != 0 {
		t.Fatalf("文档不存在应返回 0,实际 %d", got)
	}
	if got := coll.Count(101); got != 1 {
		t.Fatalf("Count(101) 期望 1(row3 没拉进来) 实际 %d", got)
	}
	if got := coll.Count(0); got != 2 {
		t.Fatalf("Count(0) 统计全部,期望 2 实际 %d", got)
	}

	//🔴 不完全统计:把 row3 拉进来,同一个 Count 就变了 —— 它统计的是内存不是库
	coll.Select("row3")
	if err = coll.Data(); err != nil {
		t.Fatalf("Data:%v", err)
	}
	if got := coll.Count(101); got != 2 {
		t.Fatalf("拉进来之后 Count(101) 期望 2 实际 %d", got)
	}
}

// 临时句柄与全局句柄同批次：改动先进共享 bulkWrite，直到 Updater.Submit 才一次提交。
func TestMountSubmitSharesBulkWrite(t *testing.T) {
	u, bw := newMountUpdater(t)
	m := newMountModel("row1")
	coll, _ := u.Mount(m)
	coll.Select("row1")
	if err := u.Data(); err != nil {
		t.Fatalf("Data:%v", err)
	}

	if err := coll.Update("row1", dataset.Update{"status": int32(2)}); err != nil {
		t.Fatalf("Update:%v", err)
	}
	if len(bw.updates) != 0 || bw.submits != 0 {
		t.Fatal("改内存阶段不该产生任何写库动作")
	}

	if _, err := u.Submit(); err != nil {
		t.Fatalf("Submit:%v", err)
	}
	if len(bw.updates) != 1 || bw.updates[0] != "row1" {
		t.Fatalf("脏数据没有经 model.Setter 写进 bulkWrite:%v", bw.updates)
	}
	if bw.submits != 1 {
		t.Fatalf("bulkWrite 应恰好提交一次,实际 %d 次", bw.submits)
	}
}

// 请求失败时临时数据也不许落库（与全局句柄同生共死）。
func TestMountSubmitAbortsOnError(t *testing.T) {
	u, bw := newMountUpdater(t)
	m := newMountModel("row1")
	coll, _ := u.Mount(m)
	coll.Select("row1")
	_ = u.Data()
	_ = coll.Update("row1", dataset.Update{"status": int32(2)})

	_ = u.Errorf("业务层报错")
	if _, err := u.Submit(); err == nil {
		t.Fatal("u.Error 非空时 Submit 应返回错误")
	}
	if bw.submits != 0 {
		t.Fatal("请求已失败,bulkWrite 不该提交")
	}
}

// 🔴 defer u.Unmount(...) 在 **handler 返回时**执行，框架 Submit 排在那之后。
// Unmount 当场摘除的话这次改动就永远写不出去 —— 所以它只打标记，摘除留到 Release，
// 短流程照样走完 Data/verify/submit 全套。
func TestMountUnmountDefersToRelease(t *testing.T) {
	u, bw := newMountUpdater(t)
	m := newMountModel("row1")
	coll, err := u.Mount(m, "row1")
	if err != nil {
		t.Fatalf("Mount:%v", err)
	}
	_ = coll.Update("row1", dataset.Update{"status": int32(3)})

	u.Unmount(m) //模拟 handler 里的 defer:此刻框架还没 Submit
	if u.Mounted(m) != coll {
		t.Fatal("Unmount 只该打标记,句柄要留到请求走完")
	}
	if len(bw.updates) != 0 {
		t.Fatal("Unmount 不该自己开旁路刷盘,落库统一走 submit")
	}

	if _, err = u.Submit(); err != nil {
		t.Fatalf("Submit:%v", err)
	}
	if len(bw.updates) != 1 {
		t.Fatal("已标记卸载的句柄仍要正常参与 submit,否则这次改动永久丢失")
	}

	u.Release()
	if u.Mounted(m) != nil {
		t.Fatal("Release 之后才真正摘除")
	}
}

// 标记可撤销：同一请求内改主意再 Mount，句柄还给它。
func TestMountRemountCancelsUnmount(t *testing.T) {
	u, _ := newMountUpdater(t)
	m := newMountModel("row1")
	coll, _ := u.Mount(m, "row1")

	u.Unmount(m)
	again, err := u.Mount(m)
	if err != nil {
		t.Fatalf("Mount:%v", err)
	}
	if again != coll {
		t.Fatal("应取回同一句柄")
	}

	u.Release()
	if u.Mounted(m) != coll {
		t.Fatal("重新 Mount 应撤销卸载标记")
	}
}

// 长命句柄跨请求驻留：release 只清 dirty，内存数据留着。
func TestMountSurvivesRequestBoundary(t *testing.T) {
	u, _ := newMountUpdater(t)
	m := newMountModel("row1")
	coll, _ := u.Mount(m)
	coll.Select("row1")
	if err := u.Data(); err != nil {
		t.Fatalf("Data:%v", err)
	}

	//一次完整的请求边界
	if _, err := u.Submit(); err != nil {
		t.Fatalf("Submit:%v", err)
	}
	u.Release()
	u.Reset()

	if coll.Get("row1") == nil {
		t.Fatal("长命句柄跨请求丢了内存:release 不该清 dataset")
	}
	again, err := u.Mount(m)
	if err != nil {
		t.Fatalf("Mount 幂等取回失败:%v", err)
	}
	if again != coll {
		t.Fatal("重复 Mount 应返回同一句柄")
	}
	if m.getter != 1 {
		t.Fatalf("跨请求不该重新查库,Getter 调用次数=%d", m.getter)
	}
}

// 与已注册的全局模型重名必须报错：撞名就是同一张表两个句柄各写各的，静默数据竞争。
func TestMountRejectsRegisteredName(t *testing.T) {
	u, _ := newMountUpdater(t)
	m := newMountModel()

	backup := modelsRank
	modelsRank = append(append([]*Model{}, modelsRank...), &Model{name: m.TableName()})
	defer func() { modelsRank = backup }()

	if _, err := u.Mount(m); err == nil {
		t.Fatal("与全局模型重名的 Mount 应报错")
	}
}

// Destroy 走 destroy()(刷盘)而不是 release(),并清空挂载表。
func TestMountDestroyFlushes(t *testing.T) {
	u, bw := newMountUpdater(t)
	m := newMountModel("row1")
	coll, _ := u.Mount(m)
	coll.Select("row1")
	_ = u.Data()
	_ = coll.Update("row1", dataset.Update{"status": int32(4)})

	if err := u.Destroy(); err != nil {
		t.Fatalf("Destroy:%v", err)
	}
	if len(bw.updates) != 1 {
		t.Fatal("下线未刷盘:临时句柄的脏数据丢了")
	}
	if u.mounts != nil {
		t.Fatal("Destroy 应清空挂载表")
	}
}

// Receive：把已经在手上的文档直接塞进内存，后续 Select 会跳过它、不再查库。
// 挂载当缓存用时靠它——否则"刚亲手写进去的数据"还得照着 _id 再查一遍。
func TestMountReceiveSkipsGetter(t *testing.T) {
	u, _ := newMountUpdater(t)
	m := newMountModel() //库里空的:证明数据确实来自 Receive 而不是查库
	coll, err := u.Mount(m)
	if err != nil {
		t.Fatalf("Mount:%v", err)
	}

	coll.Receive("row1", &mountRow{Id: "row1", Val: 9})
	if coll.Get("row1") == nil {
		t.Fatal("Receive 之后应当立即取得到")
	}
	if got := coll.Val("row1"); got != 9 {
		t.Fatalf("Val 期望 9 实际 %d", got)
	}

	coll.Select("row1")
	if err = u.Data(); err != nil {
		t.Fatalf("Data:%v", err)
	}
	if m.getter != 0 {
		t.Fatal("Receive 进来的 key 不该再查库")
	}

	//塞进来的只在内存,不记脏、不落库
	if _, err = u.Submit(); err != nil {
		t.Fatalf("Submit:%v", err)
	}
	//改一笔才该落库
	if err = coll.Update("row1", dataset.Update{"status": int32(1)}); err != nil {
		t.Fatalf("Update:%v", err)
	}
}
