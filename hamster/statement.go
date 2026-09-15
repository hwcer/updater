package hamster

import (
	"github.com/hwcer/updater/operator"
)

type stmHandleExist func(k any) bool
type stmHandleReceiver func(s *Store, ops []*operator.Operator)
type stmHandleResult func(s *Store, op *operator.Operator)

// Statement 所有 Handle 类型的公共基类，管理操作流水线。
//
// 数据流: operator(待处理) → verify(填充Result) → cache(已校验) → submit(通过Receiver分发或默认插入Store.dirty)
//
// 方法大写是因为扩展层(updater)的句柄要跨包调用它们；
// 扩展层句柄把它作为**具名字段** statement 内嵌使用，核心版句柄则直接嵌入。
type Statement struct {
	ram            RAMType
	keys           Keys                 //待拉取的数据库 key，Data 阶段消费后清空
	cache          []*operator.Operator //已通过 verify 校验的操作，等待 submit
	loader         bool                 //是否已完成初始数据加载
	Store          *Store
	operator       []*operator.Operator //待处理的操作，verify 阶段消费
	handleExist    stmHandleExist       //查询数据集中是否已存在指定 key
	handleReceiver stmHandleReceiver    //接收操作结果，默认插入 Store.dirty
	// handleResult verify 搬运时填充 op.Result；nil 则跳过。
	//
	// 🔴 这是"机制必须回调扩展层"的唯一正式通道：扩展层在句柄构造时注入
	// itypesDict[op.IType] → ITypeResult.Result 的原逻辑，机制（本基类）不得感知句柄具体类型、
	// 不得反向引用道具注册表 —— 那正是拆包要斩断的第一硬阻。
	handleResult stmHandleResult
}

// NewStatement 构造句柄基类。exist 为 key 是否已在内存的判定回调（Select 去重用）。
func NewStatement(s *Store, ram RAMType, exist stmHandleExist) *Statement {
	return &Statement{ram: ram, Store: s, handleExist: exist}
}

// Has 查询key是否已经初始化。key 的形态随 handle 而定:Document 是字段名(json 名),
// Collection 是 OID
func (stmt *Statement) Has(key any) bool {
	if stmt.ram == RAMTypeAlways && stmt.loader {
		return true
	}
	if stmt.keys != nil && stmt.keys.Has(key) {
		return true
	}
	return stmt.handleExist(key)
}

func (stmt *Statement) Reset() {}

func (stmt *Statement) Reload() {
	stmt.loader = false
}

// Loading 是否需要执行初始加载
func (stmt *Statement) Loading() bool {
	return stmt.Store.status.Has(StatusInit) && !stmt.loader && (stmt.ram == RAMTypeMaybe || stmt.ram == RAMTypeAlways)
}

// Release 每一个执行时都会执行 release
func (stmt *Statement) Release() {
	stmt.keys = nil
	for _, v := range stmt.cache {
		v.Release()
	}
	stmt.cache = nil
	for _, v := range stmt.operator {
		v.Release()
	}
	stmt.operator = nil
}

// Date 执行Data 后操作
func (stmt *Statement) Date() {
	stmt.keys = nil
}

// Verify 将 operator 转入 cache，并经 handleResult 钩子填充 Result
func (stmt *Statement) Verify() {
	if len(stmt.operator) == 0 {
		return
	}
	if stmt.cache == nil {
		stmt.cache = make([]*operator.Operator, 0, len(stmt.operator))
	}
	for _, v := range stmt.operator {
		if stmt.handleResult != nil {
			stmt.handleResult(stmt.Store, v)
		}
		stmt.cache = append(stmt.cache, v)
	}
	stmt.operator = nil
}

// Receiver 设置操作结果接收器，为nil时恢复默认行为（插入Store.dirty）
func (stmt *Statement) Receiver(f stmHandleReceiver) {
	stmt.handleReceiver = f
}

// ResultSetter 设置 Result 填充钩子（扩展层注入 ITypeResult 逻辑的通道），nil 则跳过填充
func (stmt *Statement) ResultSetter(f stmHandleResult) {
	stmt.handleResult = f
}

// Submit 把已校验的操作交给接收器；无接收器时插入 Store.dirty
func (stmt *Statement) Submit() {
	if len(stmt.cache) == 0 {
		return
	}
	if stmt.handleReceiver != nil {
		stmt.handleReceiver(stmt.Store, stmt.cache)
	} else {
		stmt.Store.dirty = append(stmt.Store.dirty, stmt.cache...)
	}
	stmt.cache = nil
}

// Insert 入队一条待处理操作，before=true 时插队首（后产生的操作先校验）
func (stmt *Statement) Insert(c *operator.Operator, before ...bool) {
	if len(before) > 0 && before[0] {
		stmt.operator = append([]*operator.Operator{c}, stmt.operator...)
	} else {
		stmt.operator = append(stmt.operator, c)
	}
	stmt.Store.status.Set(StatusOperated)
}

func (stmt *Statement) Loader() bool {
	return stmt.loader
}

// Select 标记 key 待拉取，置 StatusChanged 使 Data 阶段执行
func (stmt *Statement) Select(key any) {
	if stmt.Has(key) {
		return
	}
	if stmt.keys == nil {
		stmt.keys = Keys{}
	}
	stmt.keys.Select(key)
	stmt.Store.status.Set(StatusChanged)
}

func (stmt *Statement) RAM() RAMType {
	return stmt.ram
}

// Keys 待拉取 key 集合（Data 阶段消费）
func (stmt *Statement) Keys() Keys {
	return stmt.keys
}

// Ops 待处理操作队列。
// ⚠️ parse 分支可能往队列追加（扩展层 overflow→Resolve），消费方必须每轮重新取
// len(stmt.Ops())，不能缓存切片头 —— append 扩容后旧头看不见新增。
func (stmt *Statement) Ops() []*operator.Operator {
	return stmt.operator
}

// SetLoaded 标记初始数据加载完成
func (stmt *Statement) SetLoaded(v bool) {
	stmt.loader = v
}

func (stmt *Statement) Errorf(format any, args ...any) error {
	return stmt.Store.Errorf(format, args...)
}
