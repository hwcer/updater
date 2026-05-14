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

type stmHandleDataExist func(k any) bool

// statement 所有 Handle 类型的公共基类，管理操作流水线
// 数据流: operator(待处理) → verify过滤 → cache(已校验) → submit合并到 Updater.dirty(返回前端)
type statement struct {
	ram             RAMType
	keys            Keys                 //待拉取的数据库 key，Data 阶段消费后清空
	cache           []*operator.Operator //已通过 verify 校验的操作，等待 submit
	loader          bool                 //是否已完成初始数据加载
	Updater         *Updater
	operator        []*operator.Operator //待处理的操作，verify 阶段消费
	handleDataExist stmHandleDataExist   //查询数据集中是否已存在指定 key
}

func newStatement(u *Updater, m *Model, exist stmHandleDataExist) *statement {
	return &statement{ram: m.ram, handleDataExist: exist, Updater: u}
}

// Has 查询key(DBName)是否已经初始化
func (stmt *statement) has(key any) bool {
	if stmt.ram == RAMTypeAlways && stmt.loader {
		return true
	}
	if stmt.keys != nil && stmt.keys.Has(key) {
		return true
	}
	return stmt.handleDataExist(key)
}

func (stmt *statement) reset() {
}

func (stmt *statement) reload() {
	stmt.loader = false
}

// 是否需要执行加载
func (stmt *statement) loading() bool {
	return stmt.Updater.init && !stmt.loader && (stmt.ram == RAMTypeMaybe || stmt.ram == RAMTypeAlways)
}

// 每一个执行时都会执行 release
func (stmt *statement) release() {
	stmt.keys = nil
	stmt.cache = nil
	stmt.operator = nil
}

// date 执行Data 后操作
func (stmt *statement) date() {
	stmt.keys = nil
}

// verify 将 operator 通过 Config.Filter 过滤后转入 cache
func (stmt *statement) verify() {
	if len(stmt.operator) == 0 {
		return
	}
	if stmt.cache == nil {
		stmt.cache = make([]*operator.Operator, 0, len(stmt.operator))
	}
	for _, v := range stmt.operator {
		stmt.result(v)
		if Config.Filter(v) {
			stmt.cache = append(stmt.cache, v)
		}
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

func (stmt *statement) submit() {
	if len(stmt.cache) > 0 {
		stmt.Updater.dirty = append(stmt.Updater.dirty, stmt.cache...)
		stmt.cache = nil
	}
}

func (stmt *statement) insert(c *operator.Operator, before ...bool) {
	if len(before) > 0 && before[0] {
		stmt.operator = append([]*operator.Operator{c}, stmt.operator...)
	} else {
		stmt.operator = append(stmt.operator, c)
	}
	stmt.Updater.operated = true
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
	stmt.Updater.changed = true
}

func (stmt *statement) Errorf(format any, args ...any) error {
	return stmt.Updater.Errorf(format, args...)
}
