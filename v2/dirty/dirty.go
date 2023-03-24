package dirty

import (
	"github.com/hwcer/cosgo/logger"
	"github.com/hwcer/cosmo"
)

func New() Dirty {
	return Dirty{}
}

type Dirty map[string]*Data

func (this Dirty) Has(key string) bool {
	_, ok := this[key]
	return ok
}

// Update 更新信息
func (this Dirty) Update(operator Operator, id string, src any) {
	data, ok := this[id]
	if !ok {
		data = &Data{}
		this[id] = data
	}
	switch operator {
	case OperatorTypeDel:
		data.Delete()
	case OperatorTypeNew:
		data.Create(src)
	case OperatorTypeAdd, OperatorTypeSub, OperatorTypeSet:
		if update, ok := src.(Update); ok {
			data.Update(update)
		} else {
			logger.Debug("Dirty.Update src error:%v", src)
		}
	}
}

// BulkWrite 使用Dirty数据填充BulkWrite
func (this Dirty) BulkWrite(bulkWrite *cosmo.BulkWrite) *cosmo.BulkWrite {
	for k, v := range this {
		switch v.bulkWrite {
		case BulkWriteTypeDelete:
			bulkWrite.Delete(k)
		case BulkWriteTypeCreate:
			bulkWrite.Insert(v.data)
		case BulkWriteTypeUpdate:
			bulkWrite.Update(v.Update, k)
		}
	}
	return bulkWrite
}
