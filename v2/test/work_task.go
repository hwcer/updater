package test

import (
	"fmt"
	"github.com/hwcer/cosgo/values"
	"github.com/hwcer/updater/v2"
	"github.com/hwcer/updater/v2/operator"
	"math/rand"
)

const (
	TaskEvent11 updater.EventType = iota
	TaskEvent12
	TaskEvent13
	TaskEvent14
	TaskEvent15
)

var TaskEventsDict = []updater.EventType{TaskEvent11, TaskEvent12, TaskEvent13, TaskEvent14, TaskEvent15}

type Task struct {
	id  int32
	Val int64 //当前进度
	Tar int64 //任务需要达成目标
}

func (this *Task) handle(u *updater.Updater, args values.Values) bool {
	this.Val += 1
	r := this.Val < this.Tar
	if !r {
		fmt.Printf("[%v]当前任务完成%v/%v\n", this.id, this.Val, this.Tar)
	}
	//模拟同步信息给客户端
	u.Operator(operator.Types_Set, this.id, this.Val, this.Val)
	return r
}

type TaskMgr struct {
	dict map[int32]*Task
}

func (this *TaskMgr) Init(u *updater.Updater) {
	if this.dict == nil {
		this.dict = map[int32]*Task{}
	}
	//获取进行中的任务
	l := int32(len(TaskEventsDict))
	for i := int32(1); i < 100; i++ {

		v := &Task{id: 60000 + i, Tar: rand.Int63n(10) + 1}
		e := TaskEventsDict[i%l] //随机一个事件作为任务监控对象
		u.On(e, v.handle)
		this.dict[v.id] = v
	}
}
