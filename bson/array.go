package bson

import (
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/bsontype"
	"go.mongodb.org/mongo-driver/x/bsonx/bsoncore"
	"strconv"
	"strings"
)

type Array map[string]*Element

func (arr Array) Type() bsontype.Type {
	return bson.TypeArray
}

func (arr Array) Len() (r int) {
	r += 5
	for i, ele := range arr {
		r += len(i) + 2
		if ele != nil {
			r += ele.Len()
		}
	}
	return
}

func (arr Array) Get(key string) (r *Element) {
	k1, k2 := Split(key)
	r = arr[k1]
	if r != nil && k2 != "" {
		r = r.Get(k2)
	}
	return
}

func (arr Array) Set(key string, i any) error {
	if e := arr.Get(key); e != nil {
		return e.Set(i)
	} else {
		return ErrorElementNotExist
	}
}

func (arr Array) Push(i any) error {
	k := strconv.Itoa(len(arr))
	ele := &Element{}
	if err := ele.Marshal(i); err != nil {
		return err
	}
	arr[k] = ele
	return nil
}
func (arr Array) Unset(key string) (err error) {
	return ErrorElementNotDocument
}
func (arr Array) Raw(dst []byte, keys ...string) []byte {
	var l int
	var k string
	if len(keys) > 0 {
		k = keys[0]
		l = len(k) + 2
	}
	if dst == nil {
		dst = make([]byte, 0, l)
	}
	if len(keys) > 0 {
		dst = bsoncore.AppendHeader(dst, bson.TypeArray, k)
	}
	idx, dst := bsoncore.ReserveLength(dst)
	for i := 0; i < len(arr); i++ {
		s := strconv.Itoa(i)
		if e := arr[s]; e != nil {
			dst = e.Raw(dst, s)
		} else {
			dst = bsoncore.AppendNullElement(dst, s)
		}
	}
	dst = append(dst, 0x00)
	dst = bsoncore.UpdateLength(dst, idx, int32(len(dst[idx:])))
	return dst
}

func (arr Array) Value() bsoncore.Value {
	dst := arr.Raw(nil)
	return bsoncore.Value{Type: bson.TypeArray, Data: dst}
}

func (arr Array) String() string {
	var buf strings.Builder
	first := true
	buf.WriteByte('[')
	for i := 0; i < len(arr); i++ {
		s := strconv.Itoa(i)
		if e := arr[s]; e != nil {
			if !first {
				buf.WriteString(",")
			}
			first = false
			buf.WriteString(e.String())
		}
	}
	buf.WriteByte(']')

	return buf.String()
}

func (arr Array) Reset(v []byte) error {
	raw := bsoncore.Array(v)
	if err := raw.Validate(); err != nil {
		return err
	}
	values, err := raw.Values()
	if err != nil {
		return err
	}

	for i, value := range values {
		var ele *Element
		if ele, err = NewElementFromValue(value); err != nil {
			return err
		} else {
			k := strconv.Itoa(i)
			arr[k] = ele
		}
	}
	return nil
}

// Marshal 编译对象
func (arr Array) Marshal(i interface{}) (err error) {
	t, b, err := bson.MarshalValue(i)
	if err != nil {
		return
	}
	if t != bson.TypeArray {
		return ErrorElementNotSlice
	}
	return arr.Reset(b)
}

func (arr Array) Unmarshal(i interface{}) error {
	raw := arr.Raw(nil)
	return bson.Unmarshal(raw, i)
}

// 一次扩容最大量
var ArrayAppendLimit = 100

func (arr Array) append(k string) {
	i, _ := strconv.Atoi(k)
	l := len(arr)
	if i == 0 || i < l {
		return
	}
	if i-l > ArrayAppendLimit {
		return
	}
	for j := l; j <= i; j++ {
		s := strconv.Itoa(j)
		arr[s], _ = NewElement(bson.TypeNull, nil)
	}
}

// loadOrCreate key 大于 len(arr) 时 无法序列化成二进制
func (arr Array) loadOrCreate(key string) (r *Element, loaded bool) {
	k1, k2 := Split(key)
	r, loaded = arr[k1]
	if !loaded {
		arr.append(k1)
		r = &Element{}
		arr[k1] = r
	}
	if k2 == "" {
		return
	}

	if !loaded {
		r.t = bson.TypeEmbeddedDocument
	}
	return r.loadOrCreate(k2)
}
