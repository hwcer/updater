package model

import "fmt"

// 模拟MONGODB操作
type BulkWrite struct {
}

func (BulkWrite) Update(data any, where ...any) {
	fmt.Printf("====== BulkWrite Update,data:%v  where:%v \n", data, where)
}

func (BulkWrite) Insert(documents ...any) {
	for _, v := range documents {
		fmt.Printf("====== BulkWrite Insert :%v \n", v)
	}
}

func (BulkWrite) Delete(where ...any) {
	fmt.Printf("====== BulkWrite Delete :%v \n", where)
}
