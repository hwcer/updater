package updater

// Middleware 中间件，所有 EventType 都会调用 Emit，返回 false 从列表中移除
type Middleware interface {
	Emit(u *Updater, t EventType) (next bool)
}

// 全局中间件，所有 Updater 实例共享，每次 emit 都触发，永不移除
var globalMiddlewares []Middleware

// RegisterGlobalMiddleware 注册全局中间件，必须在初始化时调用
func RegisterGlobalMiddleware(handle Middleware) {
	globalMiddlewares = append(globalMiddlewares, handle)
}

// Middlewares 中间件管理器
type Middlewares map[string]Middleware

// Get 获取中间件
func (m Middlewares) Get(name string) Middleware {
	if m != nil {
		return m[name]
	}
	return nil
}

// Use Set 的别名
func (m Middlewares) Use(name string, handle Middleware) bool {
	return m.Set(name, handle)
}

// Set 设置中间件，已存在时默认不覆盖，replace=true 时覆盖
func (m Middlewares) Set(name string, handle Middleware, replace ...bool) bool {
	if _, ok := m[name]; ok && (len(replace) == 0 || !replace[0]) {
		return false
	}
	m[name] = handle
	return true
}

// LoadOrStore 获取已有中间件，不存在时存入并返回 handle
func (m Middlewares) LoadOrStore(name string, handle Middleware) Middleware {
	if v := m[name]; v != nil {
		return v
	}
	m[name] = handle
	return handle
}

// LoadOrCreate 获取已有中间件，不存在时通过 creator 创建并存入
func (m Middlewares) LoadOrCreate(u *Updater, name string, creator func(*Updater) Middleware) Middleware {
	if v := m[name]; v != nil {
		return v
	}
	v := creator(u)
	m[name] = v
	return v
}

func (m Middlewares) emit(u *Updater, t EventType) {
	for _, p := range globalMiddlewares {
		p.Emit(u, t)
	}
	for k, p := range m {
		if !p.Emit(u, t) {
			delete(m, k)
		}
	}
}
