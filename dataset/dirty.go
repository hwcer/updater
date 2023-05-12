package dataset

import (
	"github.com/hwcer/updater/operator"
)

func NewDirty() Dirty {
	return Dirty{}
}

type Dirty map[string]*bulkWrite

func (this Dirty) Has(key string) bool {
	_, ok := this[key]
	return ok
}

// Update 更新信息
func (this Dirty) Update(op *operator.Operator) {
	bw, ok := this[op.OID]
	if !ok {
		bw = &bulkWrite{}
		this[op.OID] = bw
	}
	switch op.Type {
	case operator.Types_Del:
		bw.Delete()
	case operator.Types_New:
		bw.Create(op.Result.([]any)...)
	case operator.Types_Set:
		if update, ok := op.Result.(Update); ok {
			bw.Update(update)
		}
	case operator.Types_Add, operator.Types_Sub:
		update := NewUpdate(operator.ItemNameVAL, op.Result)
		bw.Update(update)
	}
}

// BulkWrite 使用Dirty数据填充BulkWrite
func (this Dirty) BulkWrite(bw BulkWrite) BulkWrite {
	for k, v := range this {
		switch v.bulkWriteType {
		case bulkWriteTypeDelete:
			bw.Delete(k)
		case bulkWriteTypeCreate:
			bw.Insert(v.data...)
		case bulkWriteTypeUpdate:
			bw.Update(v.Update, k)
		}
	}
	return bw
}
