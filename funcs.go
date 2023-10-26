package updater

import "github.com/hwcer/updater/operator"

func TryParseInt64(i any) (v int64, ok bool) {
	ok = true
	switch d := i.(type) {
	case int:
		v = int64(d)
	case uint:
		v = int64(d)
	case int8:
		v = int64(d)
	case uint8:
		v = int64(d)
	case int16:
		v = int64(d)
	case uint16:
		v = int64(d)
	case int32:
		v = int64(d)
	case uint32:
		v = int64(d)
	case int64:
		v = d
	case uint64:
		v = int64(d)
	case float32:
		v = int64(d)
	case float64:
		v = int64(d)
	default:
		ok = false
	}
	return
}

func TryParseInt32(i any) (v int32, ok bool) {
	var d int64
	if d, ok = TryParseInt64(i); ok {
		v = int32(d)
	}
	return
}

func ParseInt64(i any) (v int64) {
	v, _ = TryParseInt64(i)
	return
}

func ParseInt32(i any) (r int32) {
	return int32(ParseInt64(i))
}

// 溢出判断
func overflow(update *Updater, handle Handle, op *operator.Operator) (err error) {
	if op.Type != operator.TypesAdd || op.IID == 0 {
		return nil
	}
	it := handle.IType(op.IID)
	if it == nil {
		return ErrITypeNotExist(op.IID)
	}
	val := ParseInt64(op.Value)
	num := handle.Val(op.IID)
	tot := val + num
	imax := Config.IMax(op.IID)
	if imax > 0 && tot > imax {
		n := tot - imax
		if n > val {
			n = val //imax有改动
		}
		val -= n
		op.Value = val
		if resolve, ok := it.(ITypeResolve); ok {
			if err = resolve.Resolve(update, op.IID, n); err != nil {
				return
			} else {
				n = 0
			}
		}
		if n > 0 {
			//this.Adapter.overflow[cache.IID] += overflow
		}
	}
	if val == 0 {
		op.Type = operator.TypesResolve
	}
	return
}
