package operator

// Types 操作类型枚举
// 用于标识对数据的各种操作类型

type Types uint8

const (
	TypesNone     Types = 0  // 无意义
	TypesAdd      Types = 1  // 添加操作，用于增加数值或添加元素
	TypesSub      Types = 2  // 扣除操作，用于减少数值或移除元素
	TypesSet      Types = 3  // 设置操作，用于直接设置字段值
	TypesDel      Types = 4  // 删除操作，用于删除元素或字段
	TypesNew      Types = 5  // 新对象操作，等同于add，但是装备之类不能叠加时，会走NEW生成新对象
	// TypesMax      Types = 10 // 最大值写入，最终转换成set或者drop
	// TypesMin      Types = 11 // 最小值写入，最终转换成set或者drop
	TypesDrop     Types = 90 // 抛弃操作，不执行任何操作
	TypesResolve  Types = 91 // 自动分解操作，用于自动分解道具
	TypesOverflow Types = 92 // 溢出操作，道具已满使用其他方式(邮件)转发
)

// IsValid 判断操作类型是否有效
// 有效的操作类型包括：Add、Sub、Set、Del、New
func (at Types) IsValid() bool {
	return at == TypesAdd || at == TypesSub || at == TypesSet || at == TypesDel || at == TypesNew
}

// MustSelect 判断操作是否需要选择
// 需要选择的操作类型包括：Add、Sub
func (at Types) MustSelect() bool {
	return at == TypesAdd || at == TypesSub
}

// MustNumber 判断操作是否必须是正整数
// 必须是正整数的操作类型包括：Add、Sub
func (at Types) MustNumber() bool {
	return at == TypesAdd || at == TypesSub
}

// ToString 将操作类型转换为字符串
// 返回操作类型的字符串表示
func (at Types) ToString() string {
	switch at {
	case TypesAdd:
		return "add"
	case TypesSub:
		return "sub"
	case TypesSet:
		return "set"
	case TypesDel:
		return "del"
	case TypesNew:
		return "insert"
	case TypesResolve:
		return "resolve"
	case TypesDrop:
		return "discard"
	default:
		return "unknown"
	}
}
