package updater

import (
	"github.com/hwcer/updater/v2/operator"
)

type RAMType int8

const (
	RAMTypeNone   RAMType = iota //实时读写数据
	RAMTypeMaybe                 //按需读写
	RAMTypeAlways                //内存运行
)

type operatorHandle func(t operator.Types, k any, v int64, r any)

type statement struct {
	ram      RAMType
	cache    []*operator.Operator
	values   map[any]int64 //执行过程中的数量过程
	handle   operatorHandle
	Updater  *Updater
	operator []*operator.Operator //操作
	verified bool                 //是否已经检查过
}

func NewStatement(u *Updater, ram RAMType, handle operatorHandle) *statement {
	return &statement{ram: ram, handle: handle, Updater: u}
}

func (stmt *statement) done() {
	stmt.cache = append(stmt.cache, stmt.operator...)
	stmt.operator = nil
}

func (stmt *statement) reset() {
	stmt.values = map[any]int64{}
}

// 每一个执行时都会执行 release
func (stmt *statement) release() {
	stmt.cache = nil
	stmt.values = nil
	stmt.operator = nil
	stmt.verified = false
	//b.Fields.release()
}

func (stmt *statement) Errorf(format any, args ...any) error {
	return stmt.Updater.Errorf(format, args...)
}

func (stmt *statement) Operator(c *operator.Operator, before ...bool) {
	if len(before) > 0 && before[0] {
		stmt.operator = append([]*operator.Operator{c}, stmt.operator...)
	} else {
		stmt.operator = append(stmt.operator, c)
	}
}

//func (b *statement) Has(key string) bool {
//	return b.Fields.Has(key)
//}

func (stmt *statement) submit() []*operator.Operator {
	return stmt.cache
}

// Select 字段名(HASH)或者OID(table)
//func (b *statement) Select(keys ...string) {
//	if r := b.Fields.Select(keys...); r > 0 {
//		b.Updater.changed = true
//	}
//}

func (this *statement) Add(k int32, v int32) {
	if k <= 0 || v <= 0 {
		return
	}
	this.handle(operator.Types_Add, k, int64(v), nil)
}

func (this *statement) Sub(k int32, v int32) {
	if k <= 0 || v <= 0 {
		return
	}
	this.handle(operator.Types_Sub, k, int64(v), nil)
}

func (this *statement) Max(k int32, v int64) {
	if k <= 0 {
		return
	}
	this.handle(operator.Types_Max, k, v, nil)
}

func (this *statement) Min(k int32, v int64) {
	if k <= 0 {
		return
	}
	this.handle(operator.Types_Min, k, v, nil)
}

func (this *statement) Del(k any) {
	this.handle(operator.Types_Del, k, 0, nil)
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
