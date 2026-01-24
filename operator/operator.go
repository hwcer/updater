package operator

import (
	"encoding/json"
	"fmt"
)

/*
	Operator 操作对象

	数据结构以及有效字段说明

	公共字段，所有模式下都存在，且意义相同：
	- Mod: 物品类型 model ID，用于标识数据模型
	- Type: 操作类型

	各模式下的有效字段：

	1. ParserTypeValues (数字型键值对) :
	   - ADD : IID (int32), Value (int32), Result (int32), Mod (int32)
	   - SUB : IID (int32), Value (int32), Result (int32), Mod (int32)
	   - SET : IID (int32), Result (int32), Mod (int32)
	   - DEL : IID (int32), Mod (int32)

	2. ParserTypeDocument (文档存储) :
	   - ADD : Key(string), Value(any), Result(any), Mod (int32)
	   - SUB : Key(string), Value(any), Result(any), Mod (int32)
	   - SET : Key(string), Result(any), Mod (int32) {m=10  t = set  k=lv r=10}

	3. ParserTypeCollection (文档集合) :
	   - ADD : OID(string), IID(int32), Value(int32), Result(int32), Mod (int32)
	   - SUB : OID(string), IID(int32), Value(int32), Result(int32), Mod (int32)
	   - DEL : OID(string), IID(int32), Mod (int32)
	   - SET : OID(string), IID(int32), Result(map[string]any), Mod (int32)
	   - NEW : OID(string), IID(int32), Result([]any), Mod (int32)

	使用示例：
	1. 创建一个添加道具的操作
	   op := operator.New(operator.TypesAdd, 10, 100)
	   op.IID = 1001
	   op.Mod = 1

	2. 创建一个设置文档字段的操作
	   op := operator.New(operator.TypesSet, 0, 20)
	   op.Key = "lv"
	   op.Mod = 2

	3. 创建一个删除集合元素的操作
	   op := operator.New(operator.TypesDel, 0, nil)
	   op.OID = "item_123"
	   op.IID = 2001
	   op.Mod = 3
*/

// New 创建一个新的操作对象
// t: 操作类型
// v: 增量值，add、sub、new 时有效
// r: 最终结果
func New(t Types, v int64, r any) *Operator {
	return &Operator{Type: t, Value: v, Result: r}
}

// Operator 操作对象，用于描述对数据的各种操作

type Operator struct {
	OID    string `json:"o,omitempty"` // object id，用于标识集合中的单个对象
	IID    int32  `json:"i,omitempty"` // item id，用于标识道具或物品的唯一ID
	Key    string `json:"k,omitempty"` // 字段名，用于标识文档中的字段
	Mod    int32  `json:"m,omitempty"` // 物品类型 model ID，用于标识数据模型
	Type   Types  `json:"t"`           // 操作类型，如 add、sub、set、del、new 等
	Value  int64  `json:"v"`           // 增量值，add、sub、new 时有效
	Result any    `json:"r"`           // 最终结果，根据操作类型和数据模型不同而不同
}

// Clone 克隆一个操作对象，并可选择性地修改增量值
// v: 可选参数，用于修改克隆对象的增量值
func (op *Operator) Clone(v ...int64) *Operator {
	r := *op
	if len(v) > 0 {
		r.Value = v[0]
	}
	return &r
}

// String 将操作对象转换为字符串
// 返回操作对象的字符串表示
func (op *Operator) String() string {
	return fmt.Sprintf("Operator{OID:%s, IID:%d, Key:%s, Mod:%d, Type:%s, Value:%d, Result:%v}",
		op.OID, op.IID, op.Key, op.Mod, op.Type.ToString(), op.Value, op.Result)
}

// 兼容旧版
func (op *Operator) MarshalJSON() ([]byte, error) {
	data := make(map[string]interface{})
	if op.OID != "" {
		data["o"] = op.OID
	}
	if op.IID != 0 {
		data["i"] = op.IID
	}
	if op.Key != "" {
		data["k"] = op.Key
	}
	if op.Mod != 0 {
		data["m"] = op.Mod
		data["b"] = op.Mod //兼容旧版
	}

	data["m"] = op.Mod
	data["t"] = op.Type
	data["v"] = op.Value
	data["r"] = op.Result

	return json.Marshal(data)
}
