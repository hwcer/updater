package dirty

type BulkWriteType int8

const (
	BulkWriteTypeNone BulkWriteType = iota
	BulkWriteTypeCreate
	BulkWriteTypeDelete
	BulkWriteTypeUpdate
)

type Data struct {
	data      any           //New时新创建的对象,指针指向dataset.data
	update    Update        //需要更新的内容
	bulkWrite BulkWriteType //操作方式
}

func (this Data) Create(v any) {
	this.data = v
	this.bulkWrite = BulkWriteTypeCreate
}
func (this Data) Delete() {
	this.data = nil
	this.update = nil
	this.bulkWrite = BulkWriteTypeDelete
}
func (this Data) Update(update Update) {
	if this.bulkWrite == BulkWriteTypeCreate {
		return //内存绑定关系
	}
	//可以撤销Delete
	this.bulkWrite = BulkWriteTypeUpdate
	if this.update == nil {
		this.update = update
	} else {
		for k, v := range update {
			this.update[k] = v
		}
	}
}
