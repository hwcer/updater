package operator

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