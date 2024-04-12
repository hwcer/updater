package verify

import "github.com/hwcer/updater"

const (
	ConditionNone    int32 = 0   //无条件直接返回成功
	ConditionData    int32 = 1   //基础数据,日常，成就记录
	ConditionEvents  int32 = 2   //即时任务，监听数据,仅限于任务
	ConditionMethod  int32 = 9   //需要方法实现
	ConditionWeekly  int32 = 101 //周数据,基于daily
	ConditionHistory int32 = 102 //历史数据
)

var verifyCondition = make(map[int32]verifyConditionHandle)

// verifyConditionHandle times  开始时间，结束时间仅仅用在 TaskConditionHistory 类型的活动中
type verifyConditionHandle func(u *updater.Updater, handle Value) int64

func register(key int32, handle verifyConditionHandle) {
	verifyCondition[key] = handle
}
