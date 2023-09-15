package operator

func New(t Types, v int64, r any) *Operator {
	return &Operator{Type: t, Value: v, Result: r}
}

type Operator struct {
	OID    string `json:"o,omitempty"` //object id
	IID    int32  `json:"i,omitempty"` //item id
	Key    string `json:"k,omitempty"` //字段名
	Bag    int32  `json:"b,omitempty"` //物品类型
	Type   Types  `json:"t"`           //操作类型
	Value  int64  `json:"v"`           //增量
	Result any    `json:"r"`
}
