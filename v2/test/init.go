package test

import (
	"encoding/json"
	"github.com/hwcer/adapter"
	"github.com/hwcer/logger"
)

func Acquire(uid string) *updater.Updater {
	return updater.Pool.Acquire(uid, nil)
}

func Release(v *updater.Updater) {
	updater.Pool.Release(v)
}

type operator struct {
	Cmd   string
	Data  any
	Where any
}

type BulkWrite struct {
	operator []operator
}

func (this *BulkWrite) Save() (err error) {
	for _, op := range this.operator {
		b, _ := json.Marshal(op)
		logger.Info("%v", string(b))
	}
	return
}

func (this *BulkWrite) Update(data interface{}, where ...interface{}) {
	this.operator = append(this.operator, operator{"Update", data, where})
}

func (this *BulkWrite) Insert(documents ...interface{}) {
	this.operator = append(this.operator, operator{"Insert", documents, nil})
}

func (this *BulkWrite) Delete(where ...interface{}) {
	this.operator = append(this.operator, operator{"Delete", nil, where})
}
