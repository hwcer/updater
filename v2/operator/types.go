package operator

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

func (at Types) IsValid() bool {
	return at == Types_Add || at == Types_Sub || at == Types_Set || at == Types_Del || at == Types_New
}

func (at Types) MustSelect() bool {
	return at == Types_Add || at == Types_Sub || at == Types_Max || at == Types_Min
}

// MustNumber 必须是正整数的操作
func (at Types) MustNumber() bool {
	return at == Types_Add || at == Types_Sub || at == Types_Max || at == Types_Min
}

func (at Types) ToString() string {
	switch at {
	case Types_Add:
		return "Add"
	case Types_Sub:
		return "Sub"
	case Types_Set:
		return "Set"
	case Types_Del:
		return "Del"
	case Types_New:
		return "New"
	case Types_Resolve:
		return "Resolve"
	case Types_Max:
		return "Max"
	case Types_Min:
		return "Min"
	case Types_Drop:
		return "Drop"
	default:
		return "unknown"
	}
}
