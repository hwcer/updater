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

var operatorPool = sync.Pool{
	New: func() any { return &Operator{} },
}

// New 创建一个新的操作对象，从池中获取以降低 GC 压力
// t: 操作类型
// v: 增量值，add、sub、new 时有效
// r: 最终结果
func New(opt Types, field string, value int64, result any) *Operator {
	op := operatorPool.Get().(*Operator)
	op.OType = opt
	op.Field = field
	op.Value = value
	op.Result = result
	return op
}

// Operator 操作对象，用于描述对数据的各种操作

type Operator struct {
	OID    string `json:"o,omitempty"` // object id，用于标识集合中的单个对象
	IID    int32  `json:"i,omitempty"` // item id，用于标识道具或物品的唯一ID
	OType  Types  `json:"op"`          // 操作类型，如 add、sub、set、del、new 等
	IType  int32  `json:"it"`          // 物品类型 ID，用于标识数据模型
	Field  string `json:"-"`           // 字段名，内部临时变量，不参与序列化
	Value  int64  `json:"v"`           // 增量值，add、sub、new 时有效
	Result any    `json:"r,omitempty"` // 最终结果，根据操作类型和数据模型不同而不同
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
	op.OType = 0
	op.IType = 0
	op.Field = ""
	op.Value = 0
	op.Result = nil
	operatorPool.Put(op)
}
