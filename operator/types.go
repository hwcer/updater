package operator

func (at Types) IsValid() bool {
	return at == TypesAdd || at == TypesSub || at == TypesSet || at == TypesDel || at == TypesNew
}

func (at Types) MustSelect() bool {
	return at == TypesAdd || at == TypesSub || at == TypesMax || at == TypesMin
}

// MustNumber 必须是正整数的操作
func (at Types) MustNumber() bool {
	return at == TypesAdd || at == TypesSub || at == TypesMax || at == TypesMin
}

func (at Types) ToString() string {
	switch at {
	case TypesAdd:
		return "Add"
	case TypesSub:
		return "Sub"
	case TypesSet:
		return "Set"
	case TypesDel:
		return "Del"
	case TypesNew:
		return "New"
	case TypesResolve:
		return "Resolve"
	case TypesMax:
		return "Max"
	case TypesMin:
		return "Min"
	case TypesDrop:
		return "Drop"
	default:
		return "unknown"
	}
}
