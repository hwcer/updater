package operator

func New(t Types, v any) *Operator {
	return &Operator{Type: t, Value: v}
}

type Operator struct {
	OID    string `json:"o,omitempty"` //object id
	IID    int32  `json:"i,omitempty"` //item id
	Key    string `json:"k,omitempty"` //字段名
	Type   Types  `json:"t"`           //操作类型
	Value  any    `json:"v"`           //增量
	Result any    `json:"r"`           //结果,类型和Value一样
	//Attach any    `json:"-"`           //保留字段,用于自定义信息
}
