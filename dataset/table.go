package dataset

import (
	"github.com/hwcer/cosmo/schema"
)

func NewTable() *Table {
	table := &Table{}
	table.Release()
	return table
}

type Table struct {
	schema  *schema.Schema
	dataset map[string]*Data
	indexes map[int32][]string
}

func (this *Table) Get(oid string) *Data {
	return this.dataset[oid]
}

func (this *Table) Set(i IModel) (r bool) {
	data := NewData(i)
	oid, _ := data.GetString(ModelNameOID)
	if oid == "" {
		return
	}
	iid, _ := data.GetInt32(ModelNameIID)
	if iid == 0 {
		return
	}
	if _, ok := this.dataset[oid]; !ok {
		this.indexes[iid] = append(this.indexes[iid], oid)
	}
	this.dataset[oid] = data
	return true
}

func (this *Table) Del(oid string) bool {
	data, ok := this.dataset[oid]
	if !ok {
		return true
	}
	delete(this.dataset, oid)
	iid, _ := data.GetInt32(ModelNameIID)
	if iid == 0 {
		return true
	}
	indexes := this.indexes[iid]
	newIndexes := make([]string, 0, len(indexes)-1)
	for _, v := range indexes {
		if v != oid {
			newIndexes = append(newIndexes, v)
		}
	}
	this.indexes[iid] = newIndexes
	return true
}

func (this *Table) Val(oid string) (r int64) {
	data, ok := this.dataset[oid]
	if !ok {
		return
	}
	r, _ = data.GetInt(ModelNameVAL)
	return
}

// Count 统计道具数量,如果道具不可叠加 则统计所有
// 叠加道具效果捅Val
func (this *Table) Count(iid int32) (r int64) {
	for _, oid := range this.Indexes(iid) {
		r += this.Val(oid)
	}
	return
}

// Indexes 配置ID为id的道具oid集合
func (this *Table) Indexes(iid int32) (r []string) {
	if v, ok := this.indexes[iid]; ok {
		r = append(r, v...)
	}
	return
}

// Release 重置清空数据
func (this *Table) Release() {
	this.dataset = make(map[string]*Data)
	this.indexes = make(map[int32][]string)
}
