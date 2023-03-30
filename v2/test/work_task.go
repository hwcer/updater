package test

import (
	"fmt"
	"github.com/hwcer/cosgo/values"
	"github.com/hwcer/updater/v2"
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
	id    int32
	Val   int32 //当前进度
	Tar   int32 //任务需要达成目标
	Event updater.EventType
}

func (this *Task) handle(u *updater.Updater, args values.Values) bool {
	this.Val += 1
	r := this.Val < this.Tar
	if r {
		fmt.Printf("[%v]当前任务进度%v/%v\n", this.id, this.Val, this.Tar)
	} else {
		fmt.Printf("[%v]当前任务完成%v/%v\n", this.id, this.Val, this.Tar)
	}
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
		v := &Task{id: i, Tar: rand.Int31n(10) + 1}
		v.Event = TaskEventsDict[i%l]
		u.On(v.Event, v.handle)
		this.dict[v.id] = v
	}
}
