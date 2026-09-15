package hamster

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/hwcer/cosgo/scc"
	"github.com/hwcer/cosgo/values"
	"github.com/hwcer/logger"
)

var (
	ErrServerDeniedService = Errorf(500, "Server denied service")                                 //灾难级故障启动，需要人工排查
	ErrBulkWriteNotInit    = Errorf(500, "BulkWrite not initialized: 数据落库会静默失效,启动时(连完数据库之后)必须设置") //Store.Loading 开服自检
)

// 错误码变量：业务可覆写（与扩展层同款约定）
var (
	ErrCodeArgsIllegal   int32 = 0
	ErrCodeItemNotExist  int32 = 0
	ErrCodeNotEnough     int32 = 0
	ErrCodeObjectIdEmpty int32 = 0
)

func Errorf(code int32, msg any, args ...any) error {
	return values.Errorf(code, msg, args...)
}

// ErrArgsIllegal 参数非法。Args 即传入的那组参数,顺序由调用点决定。
func ErrArgsIllegal(args ...any) error {
	return values.Errorf(ErrCodeArgsIllegal, "args illegal").WithArgs(args...)
}

// ErrItemNotExist 数据不存在。Args 为 [OID]。
func ErrItemNotExist(id any) error {
	return values.Errorf(ErrCodeItemNotExist, "Item Not Exist").WithArgs(id)
}

// ErrNotEnough 数量不足（核心版的中性语汇：字段数值扣为负的拦截，不绑定道具语义）。
// Args 顺序 [字段或OID, 需要数量, 当前数值]。
func ErrNotEnough(args ...any) error {
	return values.Errorf(ErrCodeNotEnough, "Not Enough").WithArgs(args...)
}

// ErrObjectIdEmpty OID 为空。
func ErrObjectIdEmpty(args ...any) error {
	return values.Errorf(ErrCodeObjectIdEmpty, "oid empty").WithArgs(args...)
}

var (
	ErrUnableUseIIDOperation = Errorf(0, "unable to use iid operation")
	ErrSubmitEndlessLoop     = Errorf(0, "submit endless loop") //出现死循环,检查事件和插件是否正确移除(返回false)
)

// disaster 数据库熔断保护
var disaster = atomic.Int32{}

// monitoring 标记是否已经有监控协程在运行
var monitoring = atomic.Bool{}

type SaveErrorType int32

const (
	SaveErrorTypeNone     SaveErrorType = iota //一般性错误可以忽略等待下次同步
	SaveErrorTypeNetwork                       //网络错误，可以等待一段时间后恢复,持续的网络错误无法自动恢复会自动升级为灾难性错误
	SaveErrorTypeProgram                       //程序级错误，可能是数据结构不一致，主键重复等无法恢复，操作被丢弃，保留日志记录错误
	SaveErrorTypeDisaster                      //灾难性错误，立即拒绝所有服务，禁止用户操作任何数据
)

// SaveErrorHandle 写入数据库发生错误时查询灾难等级
// err 传递给用的错误信息
var SaveErrorHandle = func(s *Store, err error) (SaveErrorType, error) {
	return SaveErrorTypeNone, err
}

// DatabaseMonitoring 查询数据库监控状态，是否可以
var DatabaseMonitoring = func() bool {
	return true
}

// OnSaveErrorHandle 写库报错分级处理：按 SaveErrorHandle 的结论决定重试/丢弃/熔断。
// 业务模型的 Setter 里落库失败时调用它，把网络故障升级为熔断保护。
func OnSaveErrorHandle(s *Store, err error) (bool, error) {
	t, newErr := SaveErrorHandle(s, err)
	var retain = true
	switch t {
	case SaveErrorTypeNone:
	case SaveErrorTypeNetwork:
		initiateDatabaseMonitoring()
	case SaveErrorTypeProgram:
		retain = false
	case SaveErrorTypeDisaster:
		disaster.Store(1)
	}
	return retain, newErr
}

// initiateDatabaseMonitoring 数据库网络错误时启动数据库监控检查
// 通过持续的调用DatabaseMonitoring 查询数据库是否可用
// 长时间(30s)数据库不可用会进入灾难级错误开启数据库熔断保护，直到数据库恢复可用
// 可能同时出现并发性调用，注意只能启动唯一携程用来监控
// 通过设置 disaster 设定,取消数据库熔断保护
func initiateDatabaseMonitoring() {
	// 使用原子操作确保只有一个监控协程在运行
	if !monitoring.CompareAndSwap(false, true) {
		return
	}
	// 启动监控协程
	scc.CGO(func(ctx context.Context) {
		defer monitoring.Store(false) // 协程结束时重置监控状态
		// 记录开始检查的时间
		startTime := time.Now()
		const timeout = 30 * time.Second
		var sleepTime = time.Second
		timer := time.NewTimer(sleepTime)
		defer timer.Stop()
		// 持续检查数据库状态
		for {
			select {
			case <-ctx.Done():
				// 收到取消信号，退出协程
				return
			case <-timer.C:
				// 检查数据库是否可用
				if DatabaseMonitoring() {
					logger.Trace("数据库已恢复，取消灾难模式")
					disaster.CompareAndSwap(1, 0)
					return
				}

				// 检查是否超过30秒未恢复
				if time.Since(startTime) >= timeout {
					logger.Trace("数据库无法恢复，开启灾难级错误保护")
					disaster.CompareAndSwap(0, 1)
				} else {
					logger.Trace("数据库连接失败，正在检查网络状况")
				}

				// 每隔一段时间检查一次
				timer.Reset(sleepTime)
			}
		}
	})
}
