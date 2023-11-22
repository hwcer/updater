package dataset

import (
	"go.mongodb.org/mongo-driver/bson"
	"strings"
)

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

func Format(s ...string) string {
	return strings.Join(s, ".")
}

// TODO
func ParseMap(k string, i any) (r map[string]any) {
	if k != "*" {
		r = make(map[string]any)
		r[k] = i
		return r
	}
	switch i.(type) {
	case map[string]interface{}:
		r, _ = i.(map[string]interface{})
	case bson.M:
		r, _ = i.(bson.M)
	default:
		r = make(map[string]interface{})
		r[ItemNameVAL] = i
	}
	return
}
