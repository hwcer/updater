package updater

import (
	"testing"

	"github.com/hwcer/updater/dataset"
)

// 本文件钉的是 Manage 数据域的核心语义。全是"跑一遍看不出来"的静默失效点：
// 双域串扰、域级事件泄漏、Load 锁形同虚设、兼容层悄悄改了行为。

type testIType int32

func (i testIType) ID() int32 { return int32(i) }

type managePlayer struct{ uid string }

func (p *managePlayer) Id() string { return p.uid }

// manageValuesModel 假的数值模型：库在内存 map，记录 Getter/Setter 次数。
// 两个域刻意用**同一张表名**、注册**同一个 IType ID** —— 隔离的前提恰恰是这些都能撞。
type manageValuesModel struct {
	table  string
	data   map[int32]int64
	getter int
	setter int
}

func newManageValuesModel(table string) *manageValuesModel {
	return &manageValuesModel{table: table, data: map[int32]int64{}}
}

func (m *manageValuesModel) TableName() string { return m.table }

func (m *manageValuesModel) Getter(_ *Updater, data *dataset.Values, keys []int32) error {
	m.getter++
	for _, k := range keys {
		if v, ok := m.data[k]; ok {
			data.Set(k, v)
		}
	}
	return nil
}

func (m *manageValuesModel) Setter(_ *Updater, bw BulkWrite, dirty dataset.Data, _ []int32) error {
	m.setter++
	bw.Update(m, dirty, m.table) //Values 的落库没有 where，补表名占位供 mountBulk 计数
	return nil
}

// newTestManage 造一个配置齐全的测试域：IType 恒路由到 9001，BulkWrite 走假实例
func newTestManage(bw *mountBulk, model *manageValuesModel) *Manage {
	mg := New()
	mg.Config.IType = func(int32) int32 { return 9001 }
	if bw != nil {
		mg.Config.BulkWrite = func(*Updater) BulkWrite { return bw }
	}
	if err := mg.Register(ParserTypeValues, RAMTypeMaybe, model, testIType(9001)); err != nil {
		panic(err)
	}
	return mg
}

// 双域隔离：两个域注册**同一个 IType ID** 指向不同模型，各自 Add/Submit 互不串扰，
// 各自的 BulkWrite 只收到自己域的改动 —— 这是"一个公会就相当于一个玩家"的物理基础。
func TestManageTwoDomainsIsolation(t *testing.T) {
	mA := newManageValuesModel("same_table")
	mB := newManageValuesModel("same_table") //表名都一样：隔离不依赖表名
	bwA, bwB := &mountBulk{}, &mountBulk{}
	mgA := newTestManage(bwA, mA)
	mgB := newTestManage(bwB, mB)

	run := func(mg *Manage, m *manageValuesModel, uid string, iid int32) *Updater {
		u := mg.New(&managePlayer{uid: uid})
		if err := u.Loading(); err != nil {
			t.Fatalf("Loading:%v", err)
		}
		u.Reset()
		u.Add(iid, 5)
		if _, err := u.Submit(); err != nil {
			t.Fatalf("Submit:%v", err)
		}
		u.Release()
		return u
	}

	ua := run(mgA, mA, "uid_a", 100)
	ub := run(mgB, mB, "uid_b", 100) //同一 iid

	if got := ua.Val(100); got != 5 {
		t.Fatalf("域A的 iid=100 应为 5,实际 %d", got)
	}
	if got := ub.Val(100); got != 5 {
		t.Fatalf("域B的 iid=100 应为 5,实际 %d", got)
	}
	//各自的模型各自落库（假模型不回写 data，看 Setter 计数）：各一次，互不代跑
	if mA.setter != 1 || mB.setter != 1 {
		t.Fatalf("两域应各收到一次自己的落库,A=%d B=%d", mA.setter, mB.setter)
	}
	if len(bwA.updates) == 0 || len(bwB.updates) == 0 {
		t.Fatalf("两域 BulkWrite 应各收到落库,A=%d B=%d", len(bwA.updates), len(bwB.updates))
	}
	//同一 IType ID 在域内唯一：再注册一次必须报错（跨域重复合法，域内不行）
	if err := mgA.Register(ParserTypeValues, RAMTypeMaybe, newManageValuesModel("x"), testIType(9001)); err == nil {
		t.Fatal("同域重复注册 IType ID 应报错")
	}
}

// 域级事件（原全局事件）按域隔离：域A的事件不因域B的实例 Emit 而触发。
func TestManageDomainEventsIsolation(t *testing.T) {
	mgA := newTestManage(nil, newManageValuesModel("evt_a"))
	mgB := newTestManage(nil, newManageValuesModel("evt_b"))

	aCount := 0
	mgA.RegisterGlobalEvent(EventTypeReset, func(u *Updater) { aCount++ })

	ub := mgB.New(&managePlayer{uid: "uid_b"})
	ub.Reset()
	if aCount != 0 {
		t.Fatal("域B的 Reset 不该触发域A的事件")
	}

	ua := mgA.New(&managePlayer{uid: "uid_a"})
	ua.Reset()
	if aCount != 1 {
		t.Fatalf("域A自己的 Reset 应触发域A事件一次,实际 %d", aCount)
	}
}

// 默认域兼容层：包级 Register + Config.IType 的旧注册形态仍走 Default 域，
// 实例经 Default.New 创建，跑通全周期。
func TestDefaultDomainCompat(t *testing.T) {
	const iid = int32(4321)
	model := newManageValuesModel("compat_default_table")

	//注册进 Default（测试结束手工还原，不污染其他用例）
	rankBackup := Default.modelsRank
	if err := Register(ParserTypeValues, RAMTypeMaybe, model, testIType(9001)); err != nil {
		t.Fatalf("Register:%v", err)
	}
	defer func() {
		Default.modelsRank = rankBackup
		delete(Default.itypesDict, 9001)
		delete(Default.modelsDict, 9001)
	}()

	itpBackup := Config.IType
	Config.IType = func(int32) int32 { return 9001 }
	defer func() { Config.IType = itpBackup }()

	bw := &mountBulk{}
	bwBackup := Config.BulkWrite
	Config.BulkWrite = func(*Updater) BulkWrite { return bw }
	defer func() { Config.BulkWrite = bwBackup }()

	u := Default.New(&managePlayer{uid: "compat_uid"}) //实例经 Default 域创建
	if u.Manage() != Default {
		t.Fatal("包级 New 产出的实例应属于 Default 域")
	}
	if err := u.Loading(); err != nil {
		t.Fatalf("Loading:%v", err)
	}
	u.Reset()
	u.Add(iid, 3)
	if _, err := u.Submit(); err != nil {
		t.Fatalf("Submit:%v", err)
	}
	if got := u.Val(iid); got != 3 {
		t.Fatalf("兼容路径 Val 应为 3,实际 %d", got)
	}
	if model.setter == 0 {
		t.Fatal("兼容路径应触发落库")
	}
	u.Release()
}
