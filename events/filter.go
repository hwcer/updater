package events

// 条件过滤器
var filters = map[int32]Filter{}

type Filter func(tar, args []int32) bool //条件过滤器

func Register(t int32, f Filter) {
	filters[t] = f
}

func defaultFilter(tar, args []int32) bool {
	if len(tar) > len(args) {
		return false
	}
	for i, v := range tar {
		if v != args[i] {
			return false
		}
	}
	return true
}
