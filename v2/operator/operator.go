package operator

func New(t Types, v any) *Operator {
	return &Operator{Type: t, Value: v}
}

//type CacheHandle func(c *Cache, k, v any) any

type Operator struct {
	OID  string `json:"o,omitempty"` //object id
	IID  int32  `json:"i,omitempty"` //item id
	Key  string `json:"k,omitempty"` //字段名
	Type Types  `json:"t"`           //操作类型
	//IType  int32  `json:"b,omitempty"`
	Value  any `json:"v"` //增量
	Result any `json:"r"` //结果,类型和Value一样
	//effective bool //立即生效,仅在需要最终一致时使用,比如体力自动回复
}

//func (this *Cache) Do(f CacheHandle) {
//	for k, v := range this.Value {
//		this.Result[k] = f(this, k, v)
//	}
//}

//func (this *Cache) Update() Update {
//	if r := ParseUpdate(this.Value); r != nil {
//		return r
//	}
//	key := this.Key
//	if key == "" {
//		key = "val"
//	}
//	return NewUpdate(key, this.Value)
//}

// Enable 开启立即生效模式
//func (this *Cache) Enable() {
//	this.effective = true
//}
//
//// Effective 是否立即生效
//func (this *Cache) Effective() bool {
//	return this.effective
//}
