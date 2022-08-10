package updater

import "go.mongodb.org/mongo-driver/bson"

func ParseInt(i interface{}) (v int64, ok bool) {
	ok = true
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
	default:
		ok = false
	}
	return
}

func ParseInt32(i interface{}) (r int32, ok bool) {
	var v int64
	if v, ok = ParseInt(i); ok {
		r = int32(v)
	}
	return
}
//TODO
func ParseMap(k string, i interface{}) (r map[string]interface{}) {
	if k!="*"{
		r = make(map[string]interface{})
		r[ItemNameVAL] = i
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
