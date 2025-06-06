package updater

import "github.com/hwcer/updater/operator"

type Handle interface {
	Get(k any) any                                  //获取值
	Val(k any) int64                                //获取val值
	Add(k any, v any)                               //自增v   int32 | int64
	Sub(k any, v any)                               //扣除v int32 | int64
	Del(k any)                                      //删除道具
	Set(k any, v ...any)                            //设置v值
	Data() error                                    //非内存模式获取数据库中的数据
	IType(int32) IType                              //根据iid获取IType
	Select(keys ...any)                             //非内存模式时获取特定道具
	Parser() Parser                                 //解析模型
	Operator(op *operator.Operator, before ...bool) //直接添加并执行封装好的Operator,不会触发任何事件
	save() error                                    //保存所有数据
	reset()                                         //运行时开始时
	reload() error                                  // 抛弃内存数据重新加载
	loading() error                                 //构造方法,load 是否需要加载数据库数据
	release()                                       //运行时释放缓存信息,并返回所有操作过程
	destroy() error                                 //同步所有数据到数据库,手动同步,或者销毁时执行
	submit() error                                  //即时同步,提交所有操作,缓存生效,同步数据库
	verify() error                                  //验证数据,执行过程的数据开始按顺序生效,但不会修改缓存
}
