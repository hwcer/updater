package dataset

type bulkWriteType int8

const (
	bulkWriteTypeNone bulkWriteType = iota
	bulkWriteTypeCreate
	bulkWriteTypeDelete
	bulkWriteTypeUpdate
)

type bulkWrite struct {
	data          []any         //New时新创建的对象,指针指向dataset.data
	update        Update        //需要更新的内容
	bulkWriteType bulkWriteType //操作方式
}

func (this *bulkWrite) Create(v ...any) {
	this.data = append(this.data, v...)
	this.bulkWriteType = bulkWriteTypeCreate
}
func (this *bulkWrite) Delete() {
	this.data = nil
	this.update = nil
	this.bulkWriteType = bulkWriteTypeDelete
}
func (this *bulkWrite) Update(update Update) {
	if this.bulkWriteType == bulkWriteTypeCreate {
		return //内存绑定关系
	}
	//可以撤销Delete
	this.bulkWriteType = bulkWriteTypeUpdate
	if this.update == nil {
		this.update = Update{}
	}
	for k, v := range update {
		this.update[k] = v
	}

}
