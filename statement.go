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
	loader         bool                 //是否已完成初始数据加载
	Updater        *Updater
	operator       []*operator.Operator //待处理的操作，verify 阶段消费
	handleExist    stmHandleExist       //查询数据集中是否已存在指定 key
	handleReceiver stmHandleReceiver    //接收操作结果，默认插入Dirty
}

func newStatement(u *Updater, m *Model, exist stmHandleExist) *statement {
	return &statement{ram: m.ram, handleExist: exist, Updater: u}
}

// Has 查询key(DBName)是否已经初始化
func (stmt *statement) has(key any) bool {
	if stmt.ram == RAMTypeAlways && stmt.loader {
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
	return stmt.Updater.status.Has(StatusInit) && !stmt.loader && (stmt.ram == RAMTypeMaybe || stmt.ram == RAMTypeAlways)
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
	it := itypesDict[opt.IType]
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

func (stmt *statement) Errorf(format any, args ...any) error {
	return stmt.Updater.Errorf(format, args...)
}
