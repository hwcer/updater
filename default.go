package updater

// Default 默认数据域：包级 API 的委托目标。
// 单域（只管玩家）用法零改动；新域请 New。
var Default = New()

// Config 兼容锚点：原包级配置，现为 Default 域配置（*Options）的别名。
var Config = Default.Config

// ===================== 兼容层（一行委托 Default 域，旧代码零改动） =====================

// ITypes 遍历默认域的 IType 表
func ITypes(f func(int32, IType) bool) { Default.ITypes(f) }

// Models 遍历默认域的模型表
func Models(f func(int32, any) bool) { Default.Models(f) }

// Register 注册模型到默认域
func Register(parser Parser, ram RAMType, model any, its ...IType) error {
	return Default.Register(parser, ram, model, its...)
}

// NewHandle 覆盖默认域的句柄工厂
func NewHandle(name Parser, f handleFunc) { Default.NewHandle(name, f) }

// RegisterGlobalCache 注册默认域的缓存构造器
func RegisterGlobalCache(name string, creator CacheCreator) {
	Default.RegisterGlobalCache(name, creator)
}

// RegisterGlobalEvent 注册默认域的事件
func RegisterGlobalEvent(t EventType, handle func(u *Updater)) {
	Default.RegisterGlobalEvent(t, handle)
}

// RegisterGlobalMiddleware 注册默认域的中间件
func RegisterGlobalMiddleware(handle Middleware) {
	Default.RegisterGlobalMiddleware(handle)
}
