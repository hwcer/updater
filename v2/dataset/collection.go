package dataset

import (
	"fmt"
	"github.com/hwcer/updater/v2/dirty"
)

func New() Collection {
	return Collection{}
}

type Collection map[string]*Document

func (this Collection) Has(id string) bool {
	_, ok := this[id]
	return ok
}

func (this Collection) Get(id string) *Document {
	return this[id]
}

func (this Collection) Set(id string, data any) {
	this[id] = NewDocument(data)
}

func (this Collection) Del(id string) {
	delete(this, id)
}

func (this Collection) Count(iid int32) (r int64) {
	//TODO 索引
	for _, v := range this {
		if v.IID() == iid {
			r += v.VAL()
		}
	}
	return
}

// Update 更新信息
func (this Collection) Update(operator dirty.Operator, id string, src any) (err error) {
	switch operator {
	case dirty.OperatorTypeDel:
		delete(this, id)
	case dirty.OperatorTypeNew:
		this.Set(id, src)
	case dirty.OperatorTypeAdd, dirty.OperatorTypeSub, dirty.OperatorTypeSet:
		if update, ok := src.(dirty.Update); ok {
			err = this.update(id, update)
		} else {
			err = fmt.Errorf("dataset.Update src error:%v", src)
		}
	}
	return
}

func (this Collection) update(id string, src dirty.Update) error {
	data := this.Get(id)
	if data == nil {
		return fmt.Errorf("data not exist:%v", id)
	}
	return data.Update(src)
}
