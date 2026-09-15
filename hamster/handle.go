package hamster

// Handle 数据句柄接口。
//
// 公开面（Get/Val/Data/Select/Parser）与生命周期驱动面（Save..Verify）都在这里。
//
// 🔴 生命周期方法**必须大写**：Go 的接口未导出方法只能由接口所在包的类型实现，
// 而本接口要被扩展层（updater 包）的句柄实现 —— 这正是"结构内嵌安全、行为覆盖不安全"
// 三条制度里"机制不得感知句柄具体类型"能成立的前提：两层都只认这一组导出方法。
//
// 命名取舍：原 submit() 改名 **Commit**，避开句柄公开的 Submit()（单表独立落库，见 Collection）；
// 其余生命周期名与 Store 的同名方法对齐（不同接收者，不冲突）。
//
// 与扩展层 Handle 接口的差异：没有 Count(iid)/increase/decrease ——
// 那三个是道具概念（溢出检查与 u.Add/Sub 路由专用），收窄到扩展层，
// 与 funcs.go overflowHandle 的同款手术（见 HAMSTER_PLAN.md 第二节 4）。
type Handle interface {
	// ---- 公开面 ----
	Get(any) any        //通过 oid 或字段名获取原始数据
	Val(any) int64      //单条记录的数值
	Data() error        //从数据库拉取 Select 标记的数据
	Select(keys ...any) //标记需要从数据库拉取的 key
	Parser() Parser     //返回解析器类型

	// ---- 生命周期驱动面（Store 驱动，业务勿调） ----
	Save() error    //持久化脏数据到数据库
	Reset()         //每次请求开始时重置状态
	Reload() error  //丢弃内存数据并重新加载
	Loading() error //初始化时加载数据
	Release()       //请求结束时释放临时状态
	Destroy() error //下线时强制刷盘
	Commit() error  //将 verify 后的操作提交并同步数据库（原 submit）
	Verify() error  //校验并执行待处理操作
}
