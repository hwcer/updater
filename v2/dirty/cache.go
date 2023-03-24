package dirty

func NewCache(t Operator, v any) *Cache {
	return &Cache{Operator: t, Value: v}
}

//type CacheHandle func(c *Cache, k, v any) any

type Cache struct {
	OID       string   `json:"o"` //object id
	IID       int32    `json:"i"` //item id
	Field     string   `json:"k"` //字段名
	Value     any      `json:"v"` //增量
	Result    any      `json:"r"` //结果,类型和Value一样
	Operator  Operator `json:"t"` //操作类型
	effective bool     //立即生效,仅在需要最终一致时使用,比如体力自动回复
}

//func (this *Cache) Do(f CacheHandle) {
//	for k, v := range this.Value {
//		this.Result[k] = f(this, k, v)
//	}
//}

// Enable 开启立即生效模式
func (this *Cache) Enable() {
	this.effective = true
}

// Effective 是否立即生效
func (this *Cache) Effective() bool {
	return this.effective
}
