package bson

import (
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/bsontype"
	"math"
	"strconv"
	"strings"
)

func ParseInt32(i interface{}) (r int32) {
	switch v := i.(type) {
	case int:
		return int32(v)
	case int8:
		return int32(v)
	case int16:
		return int32(v)
	case int32:
		return int32(v)
	case int64:
		return int32(v)
	case uint:
		return int32(v)
	case uint8:
		return int32(v)
	case uint16:
		return int32(v)
	case uint32:
		return int32(v)
	case uint64:
		return int32(v)
	case float32:
		return int32(v)
	case float64:
		return int32(v)
	}
	return
}

func ParseInt64(i interface{}) (r int64) {
	switch v := i.(type) {
	case int:
		return int64(v)
	case int8:
		return int64(v)
	case int16:
		return int64(v)
	case int32:
		return int64(v)
	case int64:
		return int64(v)
	case uint:
		return int64(v)
	case uint8:
		return int64(v)
	case uint16:
		return int64(v)
	case uint32:
		return int64(v)
	case uint64:
		return int64(v)
	case float32:
		return int64(v)
	case float64:
		return int64(v)
	}
	return
}

func IsNumber(i interface{}) (r bool) {
	switch i.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		r = true
	}
	return
}

func Length(src []byte, t bsontype.Type) int {
	switch t {
	case bson.TypeBoolean:
		return 1
	case bson.TypeDateTime, bson.TypeDouble, bson.TypeInt64, bson.TypeTimestamp:
		return 8
	case bson.TypeDecimal128:
		return 16
	case bson.TypeInt32:
		return 4
	default:
		return len(src)
	}
}

func formatDouble(f float64) string {
	var s string
	switch {
	case math.IsInf(f, 1):
		s = "Infinity"
	case math.IsInf(f, -1):
		s = "-Infinity"
	case math.IsNaN(f):
		s = "NaN"
	default:
		// Print exactly one decimalType place for integers; otherwise, print as many are necessary to
		// perfectly represent it.
		s = strconv.FormatFloat(f, 'G', -1, 64)
		if !strings.ContainsRune(s, '.') {
			s += ".0"
		}
	}

	return s
}
