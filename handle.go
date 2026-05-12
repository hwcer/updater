package updater

// Handle 数据模型的统一操作接口
// 公开方法供业务层直接调用，私有方法由 Updater 生命周期驱动
type Handle interface {
	Get(any) any        //通过 iid 或 oid 获取原始数据
	Val(any) int64      //通过 iid 或 oid 获取数值
	Data() error        //从数据库拉取 Select 标记的数据
	IType(int32) IType  //通过 iid 获取 IType，iid==0 时返回模型默认 IType
	Select(keys ...any) //标记需要从数据库拉取的 key
	Parser() Parser     //返回解析器类型

	save() error               //持久化脏数据到数据库
	reset()                    //每次请求开始时重置状态
	reload() error             //丢弃内存数据并重新加载
	loading() error            //初始化时加载数据
	release()                  //请求结束时释放临时状态
	destroy() error            //玩家下线时强制刷盘
	submit() error             //将 verify 后的操作提交并同步数据库
	verify() error             //校验并执行待处理操作（溢出检查、扣量检查等）
	increase(k int32, v int64) //由 Updater.Add 调用
	decrease(k int32, v int64) //由 Updater.Sub 调用
}
