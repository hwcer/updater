package dataset

func ParseInt(i interface{}) (r int64, ok bool) {
	ok = true
	switch v := i.(type) {
	case int:
		r = int64(v)
	case int8:
		r = int64(v)
	case int16:
		r = int64(v)
	case int32:
		r = int64(v)
	case int64:
		r = int64(v)
	default:
		ok = false
	}
	return
}

func ParseUint(i interface{}) (r uint64, ok bool) {
	ok = true
	switch v := i.(type) {
	case uint:
		r = uint64(v)
	case uint8:
		r = uint64(v)
	case uint16:
		r = uint64(v)
	case uint32:
		r = uint64(v)
	case uint64:
		r = uint64(v)
	default:
		ok = false
	}
	return
}

func ParseFloat(i interface{}) (r float64, ok bool) {
	ok = true
	switch v := i.(type) {
	case float32:
		r = float64(v)
	case float64:
		r = v
	default:
		ok = false
	}
	return
}

func ParseInt32(i any) (int32, bool) {
	if v, ok := ParseInt(i); ok {
		return int32(v), true
	}
	return 0, false
}
