package updater

import (
	"maps"
	"strconv"
	"sync"
	"testing"

	"github.com/hwcer/updater/dataset"
	"github.com/hwcer/updater/hamster"
	"github.com/hwcer/updater/operator"
)

// virtualModel 假的虚拟模型：把值委托给一份 map，并**刻意模拟真实委托方的行为** ——
// Update 不当场写 store，而是记进 pending，等 flush(相当于 verify) 才生效。
// 真实实现(项目里的 role_goods)委托给 Document.Set，也是要到 verify 才写内存。
type virtualModel struct {
	store   map[string]int64 //已生效的值
	pending map[string]int64 //本次请求待生效的值
}

func newVirtualModel() *virtualModel {
	return &virtualModel{store: map[string]int64{}, pending: map[string]int64{}}
}

func (m *virtualModel) Has(*Updater, any) bool { return true }

func (m *virtualModel) Get(_ *Updater, k any) any {
	if key, ok := k.(string); ok {
		return m.store[key]
	}
	return m.store[m.field(dataset.ParseInt32(k))]
}

func (m *virtualModel) field(iid int32) string { return "goods." + strconv.Itoa(int(iid)) }

func (m *virtualModel) Field(iid int32) (string, bool) { return m.field(iid), true }

func (m *virtualModel) Update(_ *Updater, op *operator.Operator) {
	r, ok := op.Result.(map[string]any)
	if !ok {
		return
	}
	for k, v := range r {
		m.pending[k] = dataset.ParseInt64(v) //🔴 不当场写 store,与真实委托方一致
	}
}

func (m *virtualModel) Select(*Updater, ...any) {}
func (m *virtualModel) Reload(*Updater) error   { return nil }

// flush 相当于 verify 阶段把 operator 应用到内存
func (m *virtualModel) flush() {
	maps.Copy(m.store, m.pending)
	m.pending = map[string]int64{}
}

var sharedVirtualModel = newVirtualModel()
var virtualRegisterOnce sync.Once

// namedVirtualModel 给虚拟模型挂表名（Register 派生注册名用）
type namedVirtualModel struct {
	*virtualModel
	name string
}

func (m *namedVirtualModel) TableName() string { return m.name }

// newVirtualUpdater 注册一次（注册表无反注册）并返回驱动好的 Updater、Virtual 句柄与共享模型。
// 每个用例重置共享模型状态（注册表持有的就是这一个实例）。
func newVirtualUpdater(t *testing.T) (*Updater, *hamster.Virtual, *virtualModel) {
	t.Helper()
	old := Config.BulkWrite
	Config.BulkWrite = func(*Updater) BulkWrite { return &mountBulk{} }
	t.Cleanup(func() { Config.BulkWrite = old })

	m := sharedVirtualModel
	m.store = map[string]int64{}
	m.pending = map[string]int64{}

	virtualRegisterOnce.Do(func() {
		if err := Register(ParserTypeVirtual, RAMTypeAlways, &namedVirtualModel{virtualModel: m, name: "virtual_test"}); err != nil {
			t.Fatalf("Register(Virtual):%v", err)
		}
	})

	u := New(&mountPlayer{uid: "virtual_uid"})
	if err := u.Loading(); err != nil {
		t.Fatalf("Loading:%v", err)
	}
	u.Reset()
	v := u.Virtual("virtual_test")
	if v == nil {
		t.Fatal("Virtual 句柄取不到")
	}
	return u, v, m
}

// 🔴 同一请求内对**同一个键**连加两次，必须累加。
//
// Virtual 的 operator 带的是绝对值(d+value)，而委托出去的写要到 verify 才生效 ——
// 不缓存中间态的话，第二次 Add 读到的还是**旧值**，算出的绝对值会把第一次整个覆盖掉。
// 实测踩过：充值发货时首充奖励与常规道具恰好是同一种货币，两次 Add(钻石,6) 只到账 6。
// 不报错、operator 数量也对，纯静默。
func TestVirtualAddSameKeyTwice(t *testing.T) {
	_, v, m := newVirtualUpdater(t)

	v.Add(int32(12001), 6)
	v.Add(int32(12001), 6)
	m.flush()

	if got := m.store["goods.12001"]; got != 12 {
		t.Fatalf("两次 Add(6) 应当累加到 12,实际 %d —— 后一条把前一条覆盖了", got)
	}
}

// Sub 同理，而且更危险：余额校验 `d < value` 也读旧值，
// 不缓存的话同一请求扣两次能扣成负数（绕过 CreditAllowed 那道闸）。
func TestVirtualSubSameKeyTwice(t *testing.T) {
	u, v, m := newVirtualUpdater(t)
	m.store["goods.12001"] = 10

	v.Sub(int32(12001), 6)
	if u.Error != nil {
		t.Fatalf("第一次扣 6 应当成功:%v", u.Error)
	}
	//余额只剩 4,再扣 6 必须被拦下
	v.Sub(int32(12001), 6)
	if u.Error == nil {
		t.Fatal("余额不足时第二次 Sub 应当报错 —— 校验读到的是旧余额,能扣成负数")
	}
	m.flush()
	if got := m.store["goods.12001"]; got != 4 {
		t.Fatalf("只应扣掉第一次的 6,实际余额 %d", got)
	}
}

// Set 之后再 Add，要从 Set 的新值继续算。
func TestVirtualSetThenAdd(t *testing.T) {
	_, v, m := newVirtualUpdater(t)
	m.store["goods.12001"] = 100

	v.Set(int32(12001), 5)
	v.Add(int32(12001), 3)
	m.flush()

	if got := m.store["goods.12001"]; got != 8 {
		t.Fatalf("Set(5)+Add(3) 应当是 8,实际 %d", got)
	}
}

// 缓存只在单次请求内有效：Release 之后必须回到读模型。
func TestVirtualCacheClearedOnRelease(t *testing.T) {
	u, v, m := newVirtualUpdater(t)

	v.Add(int32(12001), 6)
	if got := v.Val(int32(12001)); got != 6 {
		t.Fatalf("同请求内 Val 应读到中间态 6,实际 %d", got)
	}
	m.flush()
	m.store["goods.12001"] = 99 //把模型值改成与缓存不同，才能区分"读了缓存"还是"读了模型"
	u.Release()
	if got := v.Val(int32(12001)); got != 99 {
		t.Fatalf("Release 之后应回落到模型值 99,实际 %d（中间态没清）", got)
	}
}
