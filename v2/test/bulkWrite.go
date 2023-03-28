package test

import "fmt"

type BulkWrite struct {
}

func (BulkWrite) Update(data any, where ...any) {
	fmt.Printf("BulkWrite Update,data:%v  where:%v \n", data, where)
}

func (BulkWrite) Insert(documents ...any) {
	fmt.Printf("BulkWrite Insert :%v \n", documents)
}

func (BulkWrite) Delete(where ...any) {
	fmt.Printf("BulkWrite Delete :%v \n", where)
}
