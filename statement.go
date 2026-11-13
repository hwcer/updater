package updater

import (
	"github.com/hwcer/updater/operator"
)

type RAMType int8

const (
	RAMTypeNone   RAMType = iota //实时读写数据
	RAMTypeMaybe                 //按需读写
	RAMTypeAlways                //内存运行
)

type stmHandleExist func(k any) bool
type stmHandleReceiver func(u *Updater, ops []*operator.Operator)

// statement 所有 Handle 类型的公共基类，管理操作流水线
// 数据流: operator(待处理) → verify(填充Result) → cache(已校验) → submit(通过Receiver分发或默认插入Dirty)
type statement struct {
	ram            RAMType
	keys           Keys                 //待拉取的数据库 key，Data 阶段消费后清空
	cache          []*operator.Operator //已通过 verify 校验的操作，等待 submit
	loader         bool                 //是否已完成初始数据加载。🔴 与 dataset 同生死：release 按运行态 None 丢 dataset 时必须同步置 false——否则懒加载丢掉数据后 loader 仍为 true，转正时 tryLoad 判「已加载」不重拉，Always 句柄 has() 恒真、读空写绝对值覆盖库
	Updater        *Updater
	operator       []*operator.Operator //待处理的操作，verify 阶段消费
	handleExist    stmHandleExist       //查询数据集中是否已存在指定 key
	handleReceiver stmHandleReceiver    //接收操作结果，默认插入Dirty
}

func newStatement(u *Updater, m *Model, exist stmHandleExist) *statement {
	return &statement{ram: m.ram, handleExist: exist, Updater: u}
}

// GetRAMType 运行态 RAM 类型：懒加载一律按 RAMTypeNone 运行（不预拉、release 丢
// dataset 不驻留；请求内照常 Select→Data 按需拉→Parse 读改写→save 绝对值），**不改写
// ram 原始注册值**——玩家上线清掉 lazy 后本方法自动恢复原始语义，配合 tryLoad 重新
// 预拉即回到在线形态。消费点一律读这里，不要直读 ram 字段。
func (stmt *statement) GetRAMType() RAMType {
	if stmt.Updater.lazy {
		return RAMTypeNone
	}
	return stmt.ram
}

// Has 查询key是否已经初始化。key 的形态随 handle 而定:Document 是字段名(json 名,
// 见 Document.Name),Collection 是 OID,Values 是 iid
func (stmt *statement) has(key any) bool {
	if stmt.GetRAMType() == RAMTypeAlways && stmt.loader {
		return true
	}
	if stmt.keys != nil && stmt.keys.Has(key) {
		return true
	}
	return stmt.handleExist(key)
}

func (stmt *statement) reset() {
}

func (stmt *statement) reload() {
	stmt.loader = false
}

// 是否需要执行加载
func (stmt *statement) loading() bool {
	return !stmt.loader && (stmt.GetRAMType() == RAMTypeMaybe || stmt.GetRAMType() == RAMTypeAlways)
}

// lazyScan 懒加载禁止扫描：写 u.Error 并返回 true，调用方据此返回空结果。
//
// 懒加载（Loading(true)）不预拉，读数据前由业务自己 Select + Data 拉要用的键；扫描类接口
// （Range/Len/Count/All…）靠 Select 拉不全，一律报错而不是静默返回半份。
func (stmt *statement) lazyScan(name, method string) bool {
	if stmt.Updater == nil || !stmt.Updater.lazy { //Updater 为 nil:脱离 Updater 手工构造的句柄(单测)
		return false
	}
	_ = stmt.Updater.Errorf("懒加载禁止 %s.%s：数据未整表加载，扫描结果不完整", name, method)
	return true
}

// 每一个执行时都会执行 release
func (stmt *statement) release() {
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

// date 执行Data 后操作
func (stmt *statement) date() {
	stmt.keys = nil
}

// verify 将 operator 转入 cache，并通过 ITypeResult 填充 Result
func (stmt *statement) verify() {
	if len(stmt.operator) == 0 {
		return
	}
	if stmt.cache == nil {
		stmt.cache = make([]*operator.Operator, 0, len(stmt.operator))
	}
	for _, v := range stmt.operator {
		stmt.result(v)
		stmt.cache = append(stmt.cache, v)
	}
	stmt.operator = nil
}

func (stmt *statement) result(opt *operator.Operator) {
	it := stmt.Updater.manage.itypesDict[opt.IType]
	if it == nil {
		return
	}
	itr, ok := it.(ITypeResult)
	if !ok {
		return
	}
	opt.Result = itr.Result(stmt.Updater, opt)
}

// Receiver 设置操作结果接收器，为nil时恢复默认行为（插入Dirty）
func (stmt *statement) Receiver(f stmHandleReceiver) {
	stmt.handleReceiver = f
}

func (stmt *statement) submit() {
	if len(stmt.cache) == 0 {
		return
	}
	if stmt.handleReceiver != nil {
		stmt.handleReceiver(stmt.Updater, stmt.cache)
	} else {
		stmt.Updater.dirty = append(stmt.Updater.dirty, stmt.cache...)
	}
	stmt.cache = nil
}

func (stmt *statement) insert(c *operator.Operator, before ...bool) {
	if len(before) > 0 && before[0] {
		stmt.operator = append([]*operator.Operator{c}, stmt.operator...)
	} else {
		stmt.operator = append(stmt.operator, c)
	}
	stmt.Updater.status.Set(StatusOperated)
}

func (stmt *statement) Loader() bool {
	return stmt.loader
}

// Select 标记 key 待拉取，触发 Updater.changed 使 Data 阶段执行
func (stmt *statement) Select(key any) {
	if stmt.has(key) {
		return
	}
	if stmt.keys == nil {
		stmt.keys = Keys{}
	}
	stmt.keys.Select(key)
	stmt.Updater.status.Set(StatusChanged)
}
