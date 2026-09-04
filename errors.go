package updater

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/hwcer/cosgo/scc"
	"github.com/hwcer/cosgo/values"
	"github.com/hwcer/logger"
)

var (
	ErrCodeArgsIllegal   int32 = 0
	ErrCodeItemNotExist  int32 = 0
	ErrCodeItemNotEnough int32 = 0
	ErrCodeITypeNotExist int32 = 0
	ErrCodeObjectIdEmpty int32 = 0
)

var (
	ErrServerDeniedService = Errorf(500, "Server denied service")                                                //灾难级故障启动，需要人工排查
	ErrBulkWriteNotInit    = Errorf(500, "updater.Config.BulkWrite not initialized: 数据落库会静默失效,启动时(连完数据库之后)必须设置") //Updater.Loading 开服自检
)

func Errorf(code int32, msg any, args ...any) error {
	return values.Errorf(code, msg, args...)
}

// 以下 helper 的参数**一律只进 Args,不进文案**。
// 文案是固定的错误标识,参数由客户端从 Args 按约定顺序取,不必去解字符串。
// 代价是服务端日志里 err.Error() 只剩这句固定文案,排查时要看 Args。

// ErrArgsIllegal 参数非法。Args 即传入的那组参数,顺序由调用点决定。
func ErrArgsIllegal(args ...any) error {
	return values.Errorf(ErrCodeArgsIllegal, "args illegal").WithArgs(args...)
}

// ErrItemNotExist 道具不存在。Args 为 [道具ID或OID]。
func ErrItemNotExist(id any) error {
	return values.Errorf(ErrCodeItemNotExist, "Item Not Exist").WithArgs(id)
}

// ErrItemNotEnough 道具不足。
//
// 🔴 Args 顺序固定为 [道具ID, 需要数量, 当前持有] —— 客户端靠它提示「缺哪个道具、还差多少」。
// 改顺序等于改协议,五个调用点(parse_val/parse_doc/parse_coll×2/handle_virtual)必须同时改。
func ErrItemNotEnough(args ...any) error {
	return values.Errorf(ErrCodeItemNotEnough, "Item Not Enough").WithArgs(args...)
}

// ErrITypeNotExist IType 不存在。Args 为 [道具ID]。
func ErrITypeNotExist(iid int32) error {
	return values.Errorf(ErrCodeITypeNotExist, "IType Not Exist").WithArgs(iid)
}

// ErrObjectIdEmpty OID 为空。Args 即传入的那组参数,通常首位是道具ID。
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
var SaveErrorHandle = func(updater *Updater, err error) (SaveErrorType, error) {
	return SaveErrorTypeNone, err
}

// DatabaseMonitoring 查询数据库监控状态，是否可以
var DatabaseMonitoring = func() bool {
	return true
}

func onSaveErrorHandle(updater *Updater, err error) (bool, error) {
	t, newErr := SaveErrorHandle(updater, err)
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
