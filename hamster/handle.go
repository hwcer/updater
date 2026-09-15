package hamster

// Handle 数据句柄接口。
//
// 公开面（Get/Val/Data/Select/Parser）+ 生命周期驱动面（未导出，与主干一致 ——
// 本包是接口的唯一实现者集合，未导出方法可行；扩展层根包不再有句柄类型，
// 它只是 IID/IType 的转换包装）。
//
// 命名取舍：生命周期 submit() 改名 commit()，避开句柄公开的 Submit()（单表独立落库）。
// 没有 Count(iid)/increase/decrease/IType/IMax —— 道具语义全部经注册期注入的
// 可选接口（Keyer/OperatorDecorator/ParseDecorator/ModelReset）在扩展层实现。
type Handle interface {
	// ---- 公开面 ----
	Get(any) any        //通过 oid 或字段名获取原始数据
	Val(any) int64      //单条记录的数值
	Data() error        //从数据库拉取 Select 标记的数据
	Select(keys ...any) //标记需要从数据库拉取的 key
	Parser() Parser     //返回解析器类型

	// ---- 生命周期驱动面（Store 驱动，业务勿调；未导出 = 天然对业务隐藏） ----
	save() error    //持久化脏数据到数据库
	reset()         //每次请求开始时重置状态
	reload() error  //丢弃内存数据并重新加载
	loading() error //初始化时加载数据
	release()       //请求结束时释放临时状态
	destroy() error //下线时强制刷盘
	commit() error  //将 verify 后的操作提交并同步数据库（原 submit）
	verify() error  //校验并执行待处理操作
}
