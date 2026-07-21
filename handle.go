package updater

// Handle 数据模型的统一操作接口
// 公开方法供业务层直接调用，私有方法由 Updater 生命周期驱动
//
// Val 与 Count 的区别（勿混用）：
//
//	Val(key)   点查单条记录的数值，key 为 iid 或 oid。不可叠加道具(装备)一件一个文档，
//	           无法由 iid 唯一定位，必须传 oid；传 iid 时返回 0
//	Count(iid) 按 iid 汇总的持有总量。可叠加道具等同 Val(iid)；不可叠加道具统计该 iid 的
//	           文档数，且包含本次请求内尚未落库的新增，扫描代价 O(n)
//
// 溢出检查(IMax)比较的是 Count 而非 Val，否则装备类持有量恒为 0、上限永远挡不住
type Handle interface {
	Get(any) any           //通过 iid 或 oid 获取原始数据
	Val(any) int64         //单条记录的数值，不可叠加道具须用 oid 定位，详见上文
	Data() error           //从数据库拉取 Select 标记的数据
	Count(iid int32) int64 //按 iid 汇总的持有总量，详见上文
	IMax(iid int32) int64  //单个道具持有上限，与 Count 比较
	IType(int32) IType     //通过 iid 获取 IType，仅实现 ModelIType 的模型支持 iid==0 取默认值
	Select(keys ...any)    //标记需要从数据库拉取的 key
	Parser() Parser        //返回解析器类型

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
