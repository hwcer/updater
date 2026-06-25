package dataset

import (
	"github.com/hwcer/cosgo/values"
	"github.com/hwcer/logger"
)

const (
	collOperatorInsert int = 1 << iota // 0b001
	collOperatorUpdate                 // 0b010
	collOperatorDelete                 // 0b100
)

type Operator struct {
	op  values.Byte
	doc *Document
}

type Dirty map[string]*Operator

func (c Dirty) Has(k string) (ok, exist bool) {
	v, ok := c[k]
	if !ok {
		return false, false
	}
	if v.op.Has(collOperatorInsert) {
		return true, true
	} else if v.op.Has(collOperatorDelete) {
		return false, true
	}
	return false, false
}

func (c Dirty) Get(k string) *Document {
	if v, ok := c[k]; ok && v.op.Has(collOperatorInsert) {
		return v.doc
	}
	return nil
}

func (c Dirty) Remove(k string) {
	delete(c, k)
}

// Delete 清除所有标记，仅设 Delete
func (c Dirty) Delete(k string) {
	d := c.Operator(k)
	d.op = 0
	d.doc = nil
	d.op.Set(collOperatorDelete)
}

// Insert 清除其他标记，全新插入
func (c Dirty) Insert(k string, doc *Document) {
	d := c.Operator(k)
	d.op = 0
	d.doc = doc
	d.op.Set(collOperatorInsert)
}

// Update 标记为更新，不能与 Delete 共存
func (c Dirty) Update(k string) {
	d := c.Operator(k)
	if d.op.Has(collOperatorDelete) {
		logger.Alert("已标记为删除的记录不能 Update:%v", k)
		return
	}
	d.op.Set(collOperatorUpdate)
}

func (c Dirty) Operator(k string) (r *Operator) {
	r, ok := c[k]
	if !ok {
		r = &Operator{}
		c[k] = r
	}
	return
}
