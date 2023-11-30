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

// 通过MODEL直接获取IType
type modelIType interface {
	IType(iid int32) int32
}

type operatorHandle func(t operator.Types, k any, v int64, r any)

type statement struct {
	ram      RAMType
	values   map[any]int64 //执行过程中的数量过程
	handle   operatorHandle
	Updater  *Updater
	operator []*operator.Operator //操作
	cache    []*operator.Operator
	keys     Keys
	history  Keys
}

func NewStatement(u *Updater, ram RAMType, handle operatorHandle) *statement {
	return &statement{ram: ram, handle: handle, Updater: u}
}

func (stmt *statement) done() {
	//stmt.cache = append(stmt.cache, stmt.operator...)
	if !stmt.Updater.Async {
		stmt.keys = nil
		stmt.values = map[any]int64{}
	}
	stmt.operator = nil
	//stmt.verify = false
	stmt.Updater.Error = nil
}

// Has 查询key(DBName)是否已经初始化  todo
func (stmt *statement) has(key any) bool {
	if stmt.ram == RAMTypeAlways {
		return true
	}
	if stmt.keys != nil && stmt.keys.Has(key) {
		return true
	}
	if stmt.history != nil && stmt.history.Has(key) {
		return true
	}
	return false
}
func (stmt *statement) reset() {
	if stmt.values == nil {
		stmt.values = map[any]int64{}
	}
	if stmt.keys == nil && stmt.ram != RAMTypeAlways {
		stmt.keys = Keys{}
	}
	if stmt.history == nil && stmt.ram == RAMTypeMaybe {
		stmt.history = Keys{}
	}
}

// 每一个执行时都会执行 release
func (stmt *statement) release() {
	stmt.done()
	return
}

// date 执行Data 后操作
func (stmt *statement) date() {
	if stmt.history != nil {
		stmt.history.Merge(stmt.keys)
	}
	stmt.keys = Keys{}
}

// verify 执行verify后操作
func (stmt *statement) verify() {
	if len(stmt.operator) > 0 {
		stmt.cache = append(stmt.cache, stmt.operator...)
		stmt.operator = nil
	}
}

func (stmt *statement) submit() {
	if len(stmt.cache) > 0 {
		stmt.Updater.dirty = append(stmt.Updater.dirty, stmt.cache...)
		stmt.cache = nil
	}
}

func (stmt *statement) Select(key any) {
	if stmt.ram == RAMTypeAlways {
		return
	}
	if !stmt.has(key) {
		stmt.keys.Select(key)
		stmt.Updater.changed = true
	}
}

func (stmt *statement) Errorf(format any, args ...any) error {
	return stmt.Updater.Errorf(format, args...)
}

// Operator 直接调用有问题
func (stmt *statement) Operator(c *operator.Operator, before ...bool) {
	if len(before) > 0 && before[0] {
		stmt.operator = append([]*operator.Operator{c}, stmt.operator...)
	} else {
		stmt.operator = append(stmt.operator, c)
	}
	stmt.Updater.operated = true
}

//func (b *statement) Has(key string) bool {
//	return b.Fields.Has(key)
//}

//func (stmt *statement) submit() []*operator.Operator {
//	return stmt.cache
//}

// Select 字段名(HASH)或者OID(table)
//func (b *statement) Select(keys ...string) {
//	if r := b.Fields.Select(keys...); r > 0 {
//		b.Updater.changed = true
//	}
//}

func (stmt *statement) Add(k any, v int32) {
	if v <= 0 {
		return
	}
	stmt.handle(operator.TypesAdd, k, int64(v), nil)
}

func (stmt *statement) Sub(k any, v int32) {
	if v <= 0 {
		return
	}
	stmt.handle(operator.TypesSub, k, int64(v), nil)
}

func (stmt *statement) Max(k any, v int64) {
	stmt.handle(operator.TypesMax, k, v, nil)
}

func (stmt *statement) Min(k any, v int64) {
	stmt.handle(operator.TypesMin, k, v, nil)
}

func (stmt *statement) Del(k any) {
	stmt.handle(operator.TypesDel, k, 0, nil)
}

// Set set结果
// coll       Set(iid,k,v) Set(iid,map[any]any)
// hash doc   Set(k,v)   Set(map[any]any)
//func (this *statement) Set(k any, v ...any) {
//	switch len(v) {
//	case 0:
//		this.handle(dirty.OperatorTypeSet, k, nil)
//	case 1:
//		this.handle(dirty.OperatorTypeSet, k, v[0])
//	case 2:
//		if field, ok := v[0].(string); ok {
//			this.handle(dirty.OperatorTypeSet, k, dirty.NewUpdate(field, v[1]))
//		} else {
//			logger.Debug("set args error:%v", v)
//		}
//	}
//}
