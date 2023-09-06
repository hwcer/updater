package operator

func New(t Types, v int64, r any) *Operator {
	return &Operator{Type: t, Value: v, Result: r}
}

type Types uint8 //Cache act type
type IType uint8

const (
	TypesNone     Types = 0  //无意义
	TypesAdd      Types = 1  //添加
	TypesSub      Types = 2  //扣除
	TypesSet      Types = 3  //set
	TypesDel      Types = 4  //del
	TypesNew      Types = 5  //新对象
	TypesMax      Types = 10 //最大值写入，最终转换成set或者drop
	TypesMin      Types = 11 //最小值写入，最终转换成set或者drop
	TypesDrop     Types = 90 //抛弃不执行任何操作
	TypesResolve  Types = 91 //自动分解
	TypesOverflow Types = 92 //道具已满使用其他方式(邮件)转发
)

type Operator struct {
	OID    string `json:"o,omitempty"` //object id
	IID    int32  `json:"i,omitempty"` //item id
	Key    string `json:"k,omitempty"` //字段名
	Type   Types  `json:"t"`           //操作类型
	IType  IType  `json:"b"`           //iid 数据类型
	Value  int64  `json:"v"`           //增量
	Result any    `json:"r"`
}
