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
	//由业务层设置
	ErrCodeArgsIllegal   int32 = 9999
	ErrCodeItemNotExist  int32 = 9999
	ErrCodeItemNotEnough int32 = 9999
	ErrCodeITypeNotExist int32 = 9999
	ErrCodeObjectIdEmpty int32 = 9999
	ErrCodeNewMaxExceed  int32 = 9999
)

var (
	ErrServerDeniedService    = Errorf(500, "Server denied service")                                                //灾难级故障启动，需要人工排查
	ErrBulkWriteNotInitialize = Errorf(500, "updater.Config.BulkWrite not initialized: 数据落库会静默失效,启动时(连完数据库之后)必须设置") //Updater.Loading 开服自检
	ErrParseIdNotInitialize   = Errorf(500, "updater.Options.ParseId not initialized: 无法解析OID,启动时必须设置")              //defaultParseId 的返回错误
	ErrUnableUseIIDOperation  = Errorf(0, "unable to use iid operation")
	ErrSubmitEndlessLoop      = Errorf(0, "submit endless loop") //出现死循环,检查事件和插件是否正确移除(返回false)
)

// Errorf 构造一个 Message 错误（业务码+文案+参数），返回值可直接赋给 Error 字段
func Errorf(code int32, msg any, args ...any) *values.Message {
	return values.Errorf(code, msg, args...)
}

// 以下 helper 的参数**一律只进 Args,不进文案**。
// 文案是固定的错误标识,参数由客户端从 Args 按约定顺序取,不必去解字符串。
// 代价是服务端日志里 err.Error() 只剩这句固定文案,排查时要看 Args。

// ErrArgsIllegal 参数非法。Args 即传入的那组参数,顺序由调用点决定。
func ErrArgsIllegal(args ...any) *values.Message {
	return values.Errorf(ErrCodeArgsIllegal, "args illegal").Clone(args...)
}

// ErrItemNotExist 道具不存在。Args 为 [道具ID或OID]。
func ErrItemNotExist(id any) *values.Message {
	return values.Errorf(ErrCodeItemNotExist, "Item Not Exist").Clone(id)
}

// ErrItemNotEnough 道具不足。
//
// 🔴 Args 顺序固定为 [道具ID, 需要数量, 当前持有] —— 客户端靠它提示「缺哪个道具、还差多少」。
// 改顺序等于改协议,五个调用点(parse_val/parse_doc/parse_coll×2/handle_virtual)必须同时改。
func ErrItemNotEnough(args ...any) *values.Message {
	return values.Errorf(ErrCodeItemNotEnough, "Item Not Enough").Clone(args...)
}

// ErrITypeNotExist IType 不存在。Args 为 [道具ID]。
func ErrITypeNotExist(iid int32) *values.Message {
	return values.Errorf(ErrCodeITypeNotExist, "IType Not Exist").Clone(iid)
}

// ErrObjectIdEmpty OID 为空。Args 即传入的那组参数,通常首位是道具ID。
func ErrObjectIdEmpty(args ...any) *values.Message {
	return values.Errorf(ErrCodeObjectIdEmpty, "oid empty").Clone(args...)
}

// ErrNewMaxExceed 批量创建不可叠加道具的数量超过 Options.NewMax 上限。
// Args 顺序固定为 [道具ID, 申请数量, 上限]。
func ErrNewMaxExceed(args ...any) *values.Message {
	return values.Errorf(ErrCodeNewMaxExceed, "new max exceeded").Clone(args...)
}

// disaster 数据库熔断保护
var disaster = atomic.Int32{}

// monitoring 标记是否已经有监控协程在运行
var monitoring = atomic.Bool{}

// bulkWriteFails bulkWrite 连续提交失败计数(进程级):任何一次提交成功即清零
var bulkWriteFails = atomic.Int32{}

// BulkWriteMaxFails bulkWrite 连续提交失败容忍次数,达到后开启灾难保护
// (disaster 置位,Reset 拒绝后续请求),避免数据库真故障期间无限积累无法落库的数据。
// 恢复由 initiateDatabaseMonitoring 驱动 —— 业务应配置 DatabaseMonitoring 提供健康探测;
// 未配置时默认实现恒真,灾难位会在下个轮询(约1秒)自动解除。<=0 表示不启用。
var BulkWriteMaxFails int32 = 100

// onSubmitResult 记录一次 bulkWrite 提交结果:成功清零失败计数;
// 连续失败达到 BulkWriteMaxFails 时开启灾难保护并启动数据库监控(恢复后自动解除)
func onSubmitResult(err error) {
	if err == nil {
		bulkWriteFails.Store(0)
		return
	}
	if BulkWriteMaxFails <= 0 {
		return
	}
	if n := bulkWriteFails.Add(1); n >= BulkWriteMaxFails {
		if disaster.CompareAndSwap(0, 1) {
			logger.Alert("bulkWrite 连续提交失败 %v 次,开启灾难保护,拒绝服务直到数据库恢复", n)
		}
		initiateDatabaseMonitoring()
	}
}

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

// onBulkWriteError bulkWrite 提交失败的统一处理:失败计数/灾难保护 + 错误分级。
// 返回 false 表示业务把错误分为程序级(结构不一致/主键冲突等,重试无意义),
// 调用方必须随之丢弃队列——否则坏载荷每请求重发,累计到 BulkWriteMaxFails
// 触发灾难保护演变成全服拒绝服务。所有 bulkWrite.Submit 失败分支(Submit/Reset/
// Reload/Destroy/Mount.Submit)都必须走这里,不得只调 onSubmitResult
func onBulkWriteError(updater *Updater, err error) bool {
	onSubmitResult(err)
	retain, newErr := onSaveErrorHandle(updater, err)
	if !retain {
		logger.Alert("bulkWrite 程序级错误,丢弃队列不再重试: %v", newErr)
	}
	return retain
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
