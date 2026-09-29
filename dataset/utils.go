package dataset

import (
	"go.mongodb.org/mongo-driver/v2/bson"
	"math"
	"reflect"
	"strings"
)

func TryParseInt64(i any) (v int64, ok bool) {
	ok = true
	switch d := i.(type) {
	case int:
		v = int64(d)
	case uint:
		if uint64(d) > math.MaxInt64 {
			ok = false
			return
		}
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
		if d > math.MaxInt64 {
			ok = false
			return
		}
		v = int64(d)
	case float32:
		v = int64(d)
	case float64:
		v = int64(d)
	default:
		//命名整数类型（proto 枚举等，底层 int 系）：类型 switch 认不出命名类型，
		//静默返回 0 会让「脏值透传落库对、内存分发器写零」的数据分叉——dev21
		//joinMode 7008 死循环的根因（2026-09-29 冒烟实锤）。反射按 Kind 收编。
		rv := reflect.ValueOf(i)
		switch rv.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			v, ok = rv.Int(), true
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			if u := rv.Uint(); u <= math.MaxInt64 {
				v, ok = int64(u), true
			} else {
				ok = false
			}
		case reflect.Float32, reflect.Float64:
			v, ok = int64(rv.Float()), true
		default:
			ok = false
		}
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

func Format(s ...string) string {
	return strings.Join(s, ".")
}

func ParseMap(i any) (r map[string]any, ok bool) {
	ok = true
	switch v := i.(type) {
	case map[string]any:
		r = v
	case bson.M:
		r = v
	default:
		ok = false
	}
	return
}
