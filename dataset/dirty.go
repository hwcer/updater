package dataset

import (
	"github.com/hwcer/cosgo/values"
	"github.com/hwcer/logger"
)

const (
	collOperatorInsert int = 1
	collOperatorUpdate int = 2
	collOperatorDelete int = 3
)

type DirtyOperator struct {
	op  values.Byte
	doc *Document
}

type Dirty map[string]*DirtyOperator

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

// Delete 标记为删除
func (c Dirty) Delete(k string) {
	i := &DirtyOperator{}
	i.op.Set(collOperatorDelete)
	c[k] = i
}

// Update 标记为更新
func (c Dirty) Update(k string) {
	d, ok := c[k]
	if !ok {
		d = &DirtyOperator{}
		c[k] = d
	}
	if d.op.Has(collOperatorDelete) && !d.op.Has(collOperatorInsert) {
		logger.Alert("已经标记为删除的记录无法直接再次使用Update操作:%v", k)
		return
	}
	d.op.Set(collOperatorUpdate)
}

// Insert 临时缓存新对象
func (c Dirty) Insert(k string, doc *Document) {
	d, ok := c[k]
	if !ok {
		d = &DirtyOperator{}
		c[k] = d
	}
	d.doc = doc
	d.op.Set(collOperatorInsert)
	d.op.Delete(collOperatorUpdate) //Insert取消Update操作
}

func (c Dirty) Release() {
	for _, v := range c {
		if v.op.Has(collOperatorInsert) {
			v.doc.Release()
		}
	}
}
