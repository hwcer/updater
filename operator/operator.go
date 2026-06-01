package operator

import "sync"

/*
Operator 操作对象

数据结构以及有效字段说明

公共字段，所有模式下都存在，且意义相同：
  - IType: 物品类型 ID，用于标识数据模型
  - OType: 操作类型

各模式下的有效字段：

 1. ParserTypeValues (数字型键值对) :
    - ADD : IID(int32), Value(int64), Result(map[int32]int64)
    - SUB : IID(int32), Value(int64), Result(map[int32]int64)
    - SET : IID(int32), Value(int64), Result(map[int32]int64)
    - DEL : IID(int32), Result(map[int32]int64)

 2. ParserTypeDocument (文档存储) :
    - ADD : Field(string), Value(int64), Result(map[string]any), IType(int32)
    - SUB : Field(string), Value(int64), Result(map[string]any), IType(int32)
    - SET : Field(string), Result(map[string]any), IType(int32)

 3. ParserTypeCollection (文档集合) :
    - ADD : OID(string), IID(int32), Value(int64), Result(map[string]any), IType(int32)
    - SUB : OID(string), IID(int32), Value(int64), Result(map[string]any), IType(int32)
    - DEL : OID(string), IID(int32), IType(int32)
    - SET : OID(string), IID(int32), Result(map[string]any), IType(int32)
    - NEW : OID(string), IID(int32), Result([]any), IType(int32)

使用示例：

	op := operator.New(operator.TypesAdd, "", 100, nil)
	op.IID = 1001
*/

// Flag 操作标志位，按位组合多个行为特征
type Flag uint8

const (
	FlagUpdate  Flag = 1 << iota // 更新
	FlagDisplay                  // 展示
)

const FlagDefault = FlagUpdate | FlagDisplay

// Has 判断是否包含指定标志位
func (f *Flag) Has(flag Flag) bool {
	return *f&flag != 0
}

// Set 设置标志位
func (f *Flag) Set(flags ...Flag) {
	for _, v := range flags {
		*f |= v
	}
}

// Unset 清除指定标志位
func (f *Flag) Unset(flags ...Flag) {
	for _, v := range flags {
		*f &^= v
	}
}

var operatorPool = sync.Pool{
	New: func() any {
		op := &Operator{}
		op.Flag = FlagDefault
		return op
	},
}

// New 创建一个新的操作对象，从池中获取以降低 GC 压力
// t: 操作类型
// v: 增量值，add、sub、new 时有效
// r: 最终结果
func New(opt Types, field string, value int64, result any) *Operator {
	op := operatorPool.Get().(*Operator)
	op.Flag = FlagDefault
	op.SetOType(opt)
	op.Field = field
	op.Value = value
	op.Result = result
	return op
}

// Operator 操作对象，用于描述对数据的各种操作

type Operator struct {
	_      struct{} `json:"-"` // 禁止外部使用字段名方式构造 Operator{Field: Value}
	OID    string `json:"o,omitempty"` // object id，用于标识集合中的单个对象
	IID    int32  `json:"i,omitempty"` // item id，用于标识道具或物品的唯一ID
	Flag   Flag   `json:"f"`           // 操作标志位，按位组合控制更新和展示行为
	OType  Types  `json:"op"`          // 操作类型，如 add、sub、set、del、new 等
	IType  int32  `json:"it"`          // 物品类型 ID，用于标识数据模型
	Field  string `json:"-"`           // 字段名，内部临时变量，不参与序列化
	Value  int64  `json:"v"`           // 增量值，add、sub、new 时有效
	Result any    `json:"r,omitempty"` // 最终结果，根据操作类型和数据模型不同而不同
}

// SetOType 设置操作类型，非有效类型(Drop/Resolve/Overflow)自动清除 FlagDisplay
func (op *Operator) SetOType(t Types) {
	op.OType = t
	if !t.IsValid() {
		op.Flag.Unset(FlagDisplay)
	}
}

// Clone 克隆一个操作对象，并可选择性地修改增量值
// v: 可选参数，用于修改克隆对象的增量值
func (op *Operator) Clone(v ...int64) *Operator {
	r := operatorPool.Get().(*Operator)
	*r = *op
	if len(v) > 0 {
		r.Value = v[0]
	}
	return r
}

// Release 将操作对象归还到池中复用。处理完 Submit() 返回的 Operator 后调用。
func (op *Operator) Release() {
	if op == nil {
		return
	}
	op.OID = ""
	op.IID = 0
	op.Flag = FlagDefault
	op.OType = 0
	op.IType = 0
	op.Field = ""
	op.Value = 0
	op.Result = nil
	operatorPool.Put(op)
}
