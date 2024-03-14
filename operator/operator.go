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
	Value  int64  `json:"v"`           //增量,add sub new 时有效
	Result any    `json:"r"`           //最终结果
}

/*
	数据结构以及有效字段说明

	公共字段，所有模式下都存在，且意义相同：Bag,Type

	ParserTypeValues :  IID (int32),Value (int32),Result (int32)

    ParserTypeDocument :   Key,Value,Result

	ParserTypeCollection: OID,IID,Value,Result

*/
