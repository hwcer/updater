package dataset

import (
	"fmt"
	"github.com/hwcer/updater/v2/operator"
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

func (this Collection) Create(data any) (err error) {
	v := NewDocument(data)
	if id := v.OID(); id != "" {
		this[id] = v
	} else {
		err = fmt.Errorf("data id empty:%+v", data)
	}
	return
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

func (this Collection) create(v any) error {
	if clone, ok := v.(ModelClone); ok {
		v = clone.Clone()
	}
	return this.Create(v)
}

// Update 更新信息
func (this Collection) Update(op *operator.Operator) (err error) {
	switch op.Type {
	case operator.Types_Del:
		delete(this, op.OID)
	case operator.Types_New:
		if values, ok := op.Result.([]any); ok {
			for _, v := range values {
				err = this.create(v)
			}
		} else {
			err = fmt.Errorf("OperatorTypeNew Error:%v", op.Value)
		}
	case operator.Types_Set:
		update, _ := op.Result.(Update)
		err = this.update(op.OID, update)
	case operator.Types_Add, operator.Types_Sub:
		update := NewUpdate(operator.ItemNameVAL, op.Result)
		err = this.update(op.OID, update)
	}
	return
}

func (this Collection) update(id string, src Update) error {
	data := this.Get(id)
	if data == nil {
		return fmt.Errorf("data not exist:%v", id)
	}
	return data.Update(src)
}
