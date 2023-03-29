package operator

type Types uint8 //Cache act type

const (
	TypeAdd      Types = 1  //添加
	TypeSub            = 2  //扣除
	TypeSet            = 3  //set
	TypeDel            = 4  //del
	TypeNew            = 5  //新对象
	TypeResolve        = 6  //自动分解
	TypeOverflow       = 7  //道具已满使用其他方式(邮件)转发
	TypeMax            = 8  //最大值写入，最终转换成set或者drop
	TypeMin            = 9  //最小值写入，最终转换成set或者drop
	TypeDrop           = 99 //抛弃不执行任何操作
)

func (at Types) IsValid() bool {
	return at == TypeAdd || at == TypeSub || at == TypeSet || at == TypeDel || at == TypeNew
}

func (at Types) MustSelect() bool {
	return at == TypeAdd || at == TypeSub || at == TypeMax || at == TypeMin
}

// MustNumber 必须是正整数的操作
func (at Types) MustNumber() bool {
	return at == TypeAdd || at == TypeSub || at == TypeMax || at == TypeMin
}

func (at Types) ToString() string {
	switch at {
	case TypeAdd:
		return "Add"
	case TypeSub:
		return "Sub"
	case TypeSet:
		return "Set"
	case TypeDel:
		return "Del"
	case TypeNew:
		return "New"
	case TypeResolve:
		return "Resolve"
	case TypeMax:
		return "Max"
	case TypeMin:
		return "Min"
	case TypeDrop:
		return "Drop"
	default:
		return "unknown"
	}
}
