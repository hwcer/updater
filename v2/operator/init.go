package operator

func New(t Types, v any) *Operator {
	return &Operator{Type: t, Value: v}
}

type Operator struct {
	OID   string `json:"o,omitempty"` //object id
	IID   int32  `json:"i,omitempty"` //item id
	Key   string `json:"k,omitempty"` //字段名
	Type  Types  `json:"t"`           //操作类型
	Value any    `json:"v"`           //增量
}
