package updater

import (
	"fmt"
	"github.com/hwcer/updater/v2/dirty"
)

type RAMType int8

const (
	RAMTypeNone   RAMType = iota //实时读写数据
	RAMTypeMaybe                 //按需读写
	RAMTypeAlways                //内存运行
)

type statement struct {
	ram      RAMType
	cache    []*dirty.Cache
	Error    error
	handle   func(t dirty.Operator, k any, v any)
	Updater  *Updater
	operator []*dirty.Cache //操作
}

func NewStatement(u *Updater, ram RAMType, handle func(t dirty.Operator, k any, v any)) *statement {
	return &statement{ram: ram, handle: handle, Updater: u}
}

func (stmt *statement) done() {
	stmt.cache = append(stmt.cache, stmt.operator...)
	stmt.operator = nil
}

func (stmt *statement) reset() {

}

// 每一个执行时都会执行 release
func (stmt *statement) release() {
	stmt.cache = nil
	stmt.Error = nil
	stmt.operator = nil
	//b.Fields.release()
}

func (stmt *statement) Errorf(format string, args ...any) {
	stmt.Error = fmt.Errorf(format, args)
}

func (stmt *statement) Operator(c *dirty.Cache, before ...bool) {
	if len(before) > 0 && before[0] {
		stmt.operator = append([]*dirty.Cache{c}, stmt.operator...)
	} else {
		stmt.operator = append(stmt.operator, c)
	}
}

//func (b *statement) Has(key string) bool {
//	return b.Fields.Has(key)
//}

func (stmt *statement) Cache() []*dirty.Cache {
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
	this.handle(dirty.OperatorTypeAdd, k, v)
}

func (this *statement) Sub(k int32, v int32) {
	if k <= 0 || v <= 0 {
		return
	}
	this.handle(dirty.OperatorTypeSub, k, v)
}

func (this *statement) Max(k int32, v int64) {
	if k <= 0 {
		return
	}
	this.handle(dirty.OperatorTypeMax, k, v)
}

func (this *statement) Min(k int32, v int64) {
	if k <= 0 {
		return
	}
	this.handle(dirty.OperatorTypeMin, k, v)
}

func (this *statement) Del(k any) {
	this.handle(dirty.OperatorTypeDel, k, nil)
}

// Set set结果
// coll       Set(iid,k,v) Set(iid,map[any]any)
// hash doc   Set(k,v)   Set(map[any]any)
func (this *statement) Set(k any, v ...any) {
	switch len(v) {
	case 0:
		this.handle(dirty.OperatorTypeSet, k, nil)
	case 1:
		this.handle(dirty.OperatorTypeSet, k, v[0])
	case 2:
		this.handle(dirty.OperatorTypeSet, k, dirty.NewValue(v[0], v[1]))
	}
}
