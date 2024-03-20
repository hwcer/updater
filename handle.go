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
	IType(iid any) int32
}

type stmHandleOptCreate func(t operator.Types, k any, v int64, r any)
type stmHandleDataExist func(k any) bool

//
//type stmDatasetValues struct {
//	v map[any]int64
//}
//
//func (this *stmDatasetValues) get(k any) (v int64, ok bool) {
//	if this.v != nil {
//		v, ok = this.v[k]
//	}
//	return
//}
//
//func (this *stmDatasetValues) set(k any, v int64) {
//	if this.v == nil {
//		this.v = map[any]int64{}
//	}
//	this.v[k] = v
//}
//
//func (this *stmDatasetValues) add(k any, v int64) {
//	if this.v == nil {
//		this.v = map[any]int64{}
//	}
//	this.v[k] += v
//}
//
//func (this *stmDatasetValues) sub(k any, v int64) {
//	if this.v == nil {
//		this.v = map[any]int64{}
//	}
//	this.v[k] -= v
//}
//
//func (this *stmDatasetValues) release() {
//	this.v = nil
//}

type statement struct {
	ram   RAMType
	keys  Keys
	cache []*operator.Operator
	//values          stmDatasetValues //执行过程中的数量过程
	Updater         *Updater
	operator        []*operator.Operator //操作
	handleOptCreate stmHandleOptCreate
	handleDataExist stmHandleDataExist //查询数据集中是否存在
}

func newStatement(u *Updater, ram RAMType, opt stmHandleOptCreate, exist stmHandleDataExist) *statement {
	return &statement{ram: ram, handleOptCreate: opt, handleDataExist: exist, Updater: u}
}

//func (stmt *statement) done() {
//	//stmt.cache = append(stmt.cache, stmt.operator...)
//	if !stmt.Updater.Async {
//		stmt.keys = nil
//		stmt.values.release()
//	}
//	stmt.operator = nil
//	//stmt.verify = false
//	//stmt.Updater.Error = nil
//}

// Has 查询key(DBName)是否已经初始化
func (stmt *statement) has(key any) bool {
	if stmt.ram == RAMTypeAlways {
		return true
	}
	if stmt.keys != nil && stmt.keys.Has(key) {
		return true
	}
	return stmt.handleDataExist(key)
}
func (stmt *statement) reset() {
	//if stmt.values == nil {
	//	stmt.values = map[any]int64{}
	//}
	//if stmt.keys == nil && stmt.ram != RAMTypeAlways {
	//	stmt.keys = Keys{}
	//}
}

// 每一个执行时都会执行 release
func (stmt *statement) release() {
	//if !stmt.Updater.Async {
	//	stmt.values.release()
	//}
	stmt.keys = nil
	stmt.operator = nil
}

// date 执行Data 后操作
func (stmt *statement) date() {
	stmt.keys = nil
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
	stmt.handleOptCreate(operator.TypesAdd, k, int64(v), nil)
}

func (stmt *statement) Sub(k any, v int32) {
	if v <= 0 {
		return
	}
	stmt.handleOptCreate(operator.TypesSub, k, int64(v), nil)
}

func (stmt *statement) Max(k any, v int64) {
	stmt.handleOptCreate(operator.TypesMax, k, v, nil)
}

func (stmt *statement) Min(k any, v int64) {
	stmt.handleOptCreate(operator.TypesMin, k, v, nil)
}

func (stmt *statement) Del(k any) {
	stmt.handleOptCreate(operator.TypesDel, k, 0, nil)
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
