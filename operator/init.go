package operator

func New(t Types, v int64, r any) *Operator {
	return &Operator{Type: t, Value: v, Result: r}
}

type Types uint8 //Cache act type

const (
	Types_None     Types = 0  //无意义
	Types_Add      Types = 1  //添加
	Types_Sub      Types = 2  //扣除
	Types_Set      Types = 3  //set
	Types_Del      Types = 4  //del
	Types_New      Types = 5  //新对象
	Types_Max      Types = 10 //最大值写入，最终转换成set或者drop
	Types_Min      Types = 11 //最小值写入，最终转换成set或者drop
	Types_Drop     Types = 90 //抛弃不执行任何操作
	Types_Resolve  Types = 91 //自动分解
	Types_Overflow Types = 92 //道具已满使用其他方式(邮件)转发
)

type Operator struct {
	OID    string `json:"o,omitempty"` //object id
	IID    int32  `json:"i,omitempty"` //item id
	Key    string `json:"k,omitempty"` //字段名
	Type   Types  `json:"t"`           //操作类型
	Value  int64  `json:"v"`           //增量
	Result any    `json:"r"`
}
