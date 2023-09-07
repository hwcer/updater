package dataset

import (
	"go.mongodb.org/mongo-driver/bson"
	"strings"
)

func Format(s ...string) string {
	return strings.Join(s, ".")
}

func ParseInt64(i any) (v int64) {
	switch i.(type) {
	case int:
		v = int64(i.(int))
	case int32:
		v = int64(i.(int32))
	case int64:
		v = i.(int64)
	case float32:
		v = int64(i.(float32))
	case float64:
		v = int64(i.(float64))
	}
	return
}

func ParseInt32(i any) (r int32) {
	return int32(ParseInt64(i))
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
