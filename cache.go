package updater

import "github.com/hwcer/cosgo/values"

// Cache 自定义缓存
type Cache map[string]any

// Get 获取中间件
func (c Cache) Get(name string) any {
	return c[name]
}

func (c Cache) GetInt32(name string) int32 {
	v := c[name]
	if v == nil {
		return 0
	}
	return values.ParseInt32(v)
}
func (c Cache) GetInt64(name string) int64 {
	v := c[name]
	if v == nil {
		return 0
	}
	return values.ParseInt64(v)
}
func (c Cache) GetString(name string) string {
	v := c[name]
	if v == nil {
		return ""
	}
	return values.ParseString(v)
}

// Set 设置
func (c Cache) Set(name string, value any) {
	c[name] = value
}

// LoadOrStore 获取已有值，不存在时存入并返回
func (c Cache) LoadOrStore(name string, value any) (result any, loaded bool) {
	if v, ok := c[name]; ok {
		return v, true
	}
	c[name] = value
	return value, false
}

// LoadOrCreate 获取已有值，不存在时通过 creator 创建并存入
func (c Cache) LoadOrCreate(u *Updater, name string, creator func(*Updater) any) (result any, loaded bool) {
	if v, ok := c[name]; ok {
		return v, true
	}
	v := creator(u)
	c[name] = v
	return v, false
}
