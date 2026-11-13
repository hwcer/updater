package updater

import (
	"errors"
	"testing"
	"time"

	"github.com/hwcer/updater/dataset"
)

// 本文件钉懒加载转正（Loading(true) → Loading()）与 Loading 失败重试的收尾语义。
// 三个用例对应的都是「跑起来不报错、只是数据静默缺失」的缺陷，单看返回值发现不了。

// promoCollModel 计数全量 Getter 并可注入失败的集合模型
type promoCollModel struct {
	capCollModel
	full int //全量 Getter 次数
	fail bool
}

func (m *promoCollModel) Getter(u *Updater, data *dataset.Collection, keys []string) error {
	if len(keys) == 0 {
		m.full++
		if m.fail {
			return errors.New("coll db down")
		}
	}
	return m.capCollModel.Getter(u, data, keys)
}

// promoValuesModel 计数全量 Getter 并可注入失败的数值模型
type promoValuesModel struct {
	manageValuesModel
	full int
	fail bool
}

func (m *promoValuesModel) Getter(u *Updater, data *dataset.Values, keys []int32) error {
	if len(keys) == 0 {
		m.full++
		if m.fail {
			return errors.New("values db down")
		}
	}
	return m.manageValuesModel.Getter(u, data, keys)
}

// newPromoManage 两个常驻模型：集合（iid 1001 → 9003）在前、数值（其余 → 9004）在后，
// 注册顺序即 modelsRank 顺序——失败注入放在后一个，才能造出「前一个已加载、后一个失败」的中间态
func newPromoManage(t *testing.T, coll *promoCollModel, vals *promoValuesModel) *Manage {
	t.Helper()
	mg := New()
	mg.Config.IType = func(iid int32) int32 {
		if iid == 1001 {
			return 9003
		}
		return 9004
	}
	mg.Config.ParseId = func(_ *Updater, _ string) (int32, error) { return 1001, nil }
	mg.Config.BulkWrite = func(*Updater) BulkWrite { return &mountBulk{} }
	if err := mg.Register(ParserTypeCollection, RAMTypeAlways, coll, capIType{id: 9003, stacked: true}); err != nil {
		t.Fatal(err)
	}
	if err := mg.Register(ParserTypeValues, RAMTypeAlways, vals, testIType(9004)); err != nil {
		t.Fatal(err)
	}
	return mg
}

func newPromoModels() (*promoCollModel, *promoValuesModel) {
	coll := &promoCollModel{capCollModel: capCollModel{rows: map[string]*capRow{"c1": {Id: "c1", IID: 1001, Val: 50}}}}
	vals := &promoValuesModel{manageValuesModel: *newManageValuesModel("promo_vals")}
	return coll, vals
}

// Init = 句柄与缓存就绪（懒加载/在线首载都发一次），Loaded = 从库完整初始化。懒加载首载只发 Init，
// 转正必须补发 Loaded：依赖整档数据的派生状态（如物品索引）挂在 Loaded 上，不补就整个在线期为空。
func TestLazyPromoteEmitsLoaded(t *testing.T) {
	coll, vals := newPromoModels()
	mg := newPromoManage(t, coll, vals)

	var inits int
	var loaded []int //每次 Loaded 时集合句柄里的行数（数据是否真加载了）
	var creates int
	mg.RegisterGlobalEvent(EventTypeInit, func(u *Updater) { inits++ })
	mg.RegisterGlobalEvent(EventTypeLoaded, func(u *Updater) {
		loaded = append(loaded, u.Collection(9003).Len())
	})
	mg.RegisterGlobalCache("promo_cache", func(u *Updater) any { creates++; return creates })

	u := mg.New(&managePlayer{uid: "promo_uid"})
	if _, err := u.Loading(true); err != nil {
		t.Fatal(err)
	}
	if inits != 1 || len(loaded) != 0 {
		t.Fatalf("懒加载首载应只发 Init 不发 Loaded, 实际 inits=%d loaded=%v", inits, loaded)
	}
	u.Reset()
	u.Release()

	st, err := u.Loading()
	if err != nil {
		t.Fatal(err)
	}
	if !st.Has(StatusLoaded) {
		t.Fatalf("转正返回的 status 应含 StatusLoaded, 实际 %b", st)
	}
	if inits != 1 {
		t.Fatalf("Init 只在句柄创建时发一次, 转正不得重发, 实际 %d", inits)
	}
	if len(loaded) != 1 || loaded[0] != 1 {
		t.Fatalf("转正应补发一次 Loaded 且此时数据已加载(1 行), 实际 %v", loaded)
	}
	if creates != 1 {
		t.Fatalf("域级缓存在首载时创建一次, 转正不重建, 实际 %d", creates)
	}

	//在线首载：Init 与 Loaded 各一次
	u2 := mg.New(&managePlayer{uid: "promo_uid2"})
	if _, err := u2.Loading(); err != nil {
		t.Fatal(err)
	}
	if inits != 2 || len(loaded) != 2 || loaded[1] != 1 {
		t.Fatalf("在线首载应发 Init 与 Loaded 各一次且数据已加载, 实际 inits=%d loaded=%v", inits, loaded)
	}
}

// Loading 中途失败后重试必须把失败的句柄重新加载：旧实现只对新建句柄调 loading，
// 重试时全部跳过、照样置 Init|Loaded。另钉 u.Error 不得常驻（否则重试永远带着旧错误返回）。
func TestLoadingRetryAfterHandleFailure(t *testing.T) {
	coll, vals := newPromoModels()
	vals.fail = true
	mg := newPromoManage(t, coll, vals)

	u := mg.New(&managePlayer{uid: "retry_uid"})
	if _, err := u.Loading(); err == nil {
		t.Fatal("数值模型全量加载失败, Loading 应报错")
	}
	if st := u.Status(); st.Has(StatusInit) || u.Loaded() {
		t.Fatal("加载失败不得置 StatusInit/StatusLoaded")
	}
	if u.Error != nil {
		t.Fatalf("加载失败的错误已经由返回值交出, 不得常驻 u.Error: %v", u.Error)
	}

	vals.fail = false
	if _, err := u.Loading(); err != nil {
		t.Fatalf("故障恢复后重试应成功: %v", err)
	}
	if vals.full != 2 {
		t.Fatalf("重试必须重新全量加载失败的句柄, 数值模型全量 Getter 次数应为 2, 实际 %d", vals.full)
	}
	if coll.full != 1 {
		t.Fatalf("已加载的句柄不应重拉, 集合模型全量 Getter 次数应为 1, 实际 %d", coll.full)
	}
	if !u.Loaded() {
		t.Fatal("重试成功后应置 StatusLoaded")
	}
}

// 懒加载 release 丢 dataset 时必须同步复位 loader：否则「部分句柄已加载 → 进懒加载 →
// 丢数据 → 转正」时 tryLoad 判已加载不重拉，Always 句柄 has() 恒真，读空、写绝对值覆盖库。
func TestLazyReleaseResetsLoader(t *testing.T) {
	coll, vals := newPromoModels()
	vals.fail = true
	mg := newPromoManage(t, coll, vals)

	u := mg.New(&managePlayer{uid: "loader_uid"})
	if _, err := u.Loading(); err == nil {
		t.Fatal("数值模型全量加载失败, Loading 应报错")
	}
	if !u.Collection(9003).Loader() {
		t.Fatal("前置条件: 集合句柄应已在首次 Loading 中加载完成")
	}

	//未 Loaded 的实例可以进懒加载；懒加载请求结束 release 丢弃 dataset
	if _, err := u.Loading(true); err != nil {
		t.Fatal(err)
	}
	u.Reset()
	u.Release()
	if u.Collection(9003).Loader() {
		t.Fatal("懒加载 release 丢 dataset 后 loader 必须复位")
	}

	vals.fail = false
	if _, err := u.Loading(); err != nil {
		t.Fatal(err)
	}
	if coll.full != 2 {
		t.Fatalf("转正必须重新全量加载集合句柄, 全量 Getter 次数应为 2, 实际 %d", coll.full)
	}
	u.Reset()
	if v := u.Collection(9003).Val("c1"); v != 50 {
		t.Fatalf("转正后应读到库值 50, 实际 %d", v)
	}
	u.Release()
}

// 转正失败必须退回懒加载：否则成了「非懒加载、未 Loaded」的半成品——扫描不再被拦、半份数据常驻
func TestLazyPromoteFailureStaysLazy(t *testing.T) {
	coll, vals := newPromoModels()
	mg := newPromoManage(t, coll, vals)
	u := mg.New(&managePlayer{uid: "promo_fail_uid"})
	if _, err := u.Loading(true); err != nil {
		t.Fatal(err)
	}

	vals.fail = true
	if _, err := u.Loading(); err == nil {
		t.Fatal("数值模型全量加载失败, 转正应报错")
	}
	if u.Loaded() || !u.lazy {
		t.Fatalf("转正失败应退回懒加载, 实际 lazy=%v loaded=%v", u.lazy, u.Loaded())
	}
	u.Reset()
	if u.Collection(9003).Len() != 0 || u.Error == nil {
		t.Fatal("退回懒加载后扫描仍须被拦, 不得返回转正失败前拉进来的半份")
	}
	u.Release()
	if u.Collection(9003).Loader() {
		t.Fatal("退回懒加载后 release 应丢数据并复位 loader")
	}

	vals.fail = false
	if _, err := u.Loading(); err != nil || !u.Loaded() {
		t.Fatalf("故障恢复后转正应成功, err=%v loaded=%v", err, u.Loaded())
	}
}

// resetCollModel 可控制「是否跨天重置」的集合模型(ModelReset)
type resetCollModel struct {
	promoCollModel
	reset bool
}

func (m *resetCollModel) Reset(*Updater, time.Time) bool { return m.reset }

// EventTypeLoaded = 从数据库完整初始化：Reload（全部句柄整档重拉）要抛；跨天 ModelReset 的单句柄重载
// 不算、不抛；懒加载实例不抛
func TestReloadEmitsLoaded(t *testing.T) {
	coll := &resetCollModel{promoCollModel: promoCollModel{capCollModel: capCollModel{rows: map[string]*capRow{"c1": {Id: "c1", IID: 1001, Val: 50}}}}}
	mg := New()
	mg.Config.IType = func(int32) int32 { return 9003 }
	mg.Config.ParseId = func(_ *Updater, _ string) (int32, error) { return 1001, nil }
	mg.Config.BulkWrite = func(*Updater) BulkWrite { return &mountBulk{} }
	if err := mg.Register(ParserTypeCollection, RAMTypeAlways, coll, capIType{id: 9003, stacked: true}); err != nil {
		t.Fatal(err)
	}
	var loaded int
	mg.RegisterGlobalEvent(EventTypeLoaded, func(*Updater) { loaded++ })

	u := mg.New(&managePlayer{uid: "reload_uid"})
	if _, err := u.Loading(); err != nil || loaded != 1 {
		t.Fatalf("在线首载应发一次 Loaded, err=%v loaded=%d", err, loaded)
	}
	u.Reset()
	u.Release()
	if loaded != 1 {
		t.Fatalf("普通请求不应发 Loaded, 实际 %d", loaded)
	}

	if err := u.Reload(); err != nil || loaded != 2 {
		t.Fatalf("Reload 后应补发 Loaded, err=%v loaded=%d", err, loaded)
	}
	u.Reset()
	u.Release()
	if loaded != 2 {
		t.Fatalf("Reload 补发过后下个请求不得重发, 实际 %d", loaded)
	}

	//请求带着业务错误时 Reload 必须原样拒绝、什么都不动:否则第一个句柄换完 dataset 才报错,内存坏一半
	u.Reset()
	_ = u.Errorf("业务错误")
	if err := u.Reload(); err == nil {
		t.Fatal("带错误的请求里 Reload 应返回该错误")
	}
	if !u.Collection(9003).Loader() || u.Collection(9003).Val("c1") != 50 || loaded != 2 {
		t.Fatalf("被拒绝的 Reload 不得动内存: loader=%v val=%d loaded=%d",
			u.Collection(9003).Loader(), u.Collection(9003).Val("c1"), loaded)
	}
	u.Release()

	//Testing(false) 关闭时 Reload 失败要保持测试模式,不能让测试期间的内存数据转成正式数据
	u.Reset()
	_ = u.Testing(true)
	_ = u.Errorf("业务错误")
	if err := u.Testing(false); err == nil || !u.status.Has(StatusTesting) {
		t.Fatalf("Testing(false) 重载失败应报错并保持测试模式, err=%v testing=%v", err, u.status.Has(StatusTesting))
	}
	u.Release()
	if err := u.Testing(false); err != nil || loaded != 3 { //请求外关闭:Reload 成功,抛一次 Loaded
		t.Fatalf("无错误时 Testing(false) 应重载并抛 Loaded, err=%v loaded=%d", err, loaded)
	}

	coll.reset = true
	u.Reset() //跨天 ModelReset → 句柄 reload
	u.Release()
	coll.reset = false
	if loaded != 3 {
		t.Fatalf("跨天 ModelReset 重载不是完整初始化, 不得抛 Loaded, 实际 %d", loaded)
	}

	lz := mg.New(&managePlayer{uid: "reload_lazy_uid"})
	if _, err := lz.Loading(true); err != nil {
		t.Fatal(err)
	}
	if err := lz.Reload(); err != nil || loaded != 3 {
		t.Fatalf("懒加载实例 Reload 不得发 Loaded, err=%v loaded=%d", err, loaded)
	}
}

// 请求中途转正时已有的业务错误不得被 loadHandles 吞掉
func TestLoadingKeepsPreexistingError(t *testing.T) {
	coll, vals := newPromoModels()
	mg := newPromoManage(t, coll, vals)
	u := mg.New(&managePlayer{uid: "keep_err_uid"})
	if _, err := u.Loading(true); err != nil {
		t.Fatal(err)
	}
	u.Reset()
	_ = u.Errorf("业务先前的错误")
	if _, err := u.Loading(); err == nil {
		t.Fatal("带着已有错误转正应返回该错误")
	}
	if u.Error == nil || u.Error.Error() != "业务先前的错误" {
		t.Fatalf("请求原有的 u.Error 不得被清掉或替换, 实际 %v", u.Error)
	}
	u.Release()
}
