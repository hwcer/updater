package dirty

type Operator uint8 //Cache act type

const (
	OperatorTypeAdd      Operator = 1  //添加
	OperatorTypeSub               = 2  //扣除
	OperatorTypeSet               = 3  //set
	OperatorTypeDel               = 4  //del
	OperatorTypeNew               = 5  //新对象
	OperatorTypeResolve           = 6  //自动分解
	OperatorTypeOverflow          = 7  //道具已满使用其他方式(邮件)转发
	OperatorTypeMax               = 8  //最大值写入，最终转换成set或者drop
	OperatorTypeMin               = 9  //最小值写入，最终转换成set或者drop
	OperatorTypeDrop              = 99 //抛弃不执行任何操作
)

func (at Operator) IsValid() bool {
	return at == OperatorTypeAdd || at == OperatorTypeSub || at == OperatorTypeSet || at == OperatorTypeDel || at == OperatorTypeNew
}

func (at Operator) MustSelect() bool {
	return at == OperatorTypeAdd || at == OperatorTypeSub || at == OperatorTypeMax || at == OperatorTypeMin
}

// MustNumber 必须是正整数的操作
func (at Operator) MustNumber() bool {
	return at == OperatorTypeAdd || at == OperatorTypeSub || at == OperatorTypeMax || at == OperatorTypeMin
}

func (at Operator) ToString() string {
	switch at {
	case OperatorTypeAdd:
		return "Add"
	case OperatorTypeSub:
		return "Sub"
	case OperatorTypeSet:
		return "Set"
	case OperatorTypeDel:
		return "Delete"
	case OperatorTypeNew:
		return "Create"
	case OperatorTypeResolve:
		return "Resolve"
	case OperatorTypeMax:
		return "Max"
	case OperatorTypeMin:
		return "Min"
	case OperatorTypeDrop:
		return "Drop"
	default:
		return "unknown"
	}
}
