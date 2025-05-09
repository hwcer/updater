package bson

import (
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/bsontype"
	"go.mongodb.org/mongo-driver/x/bsonx/bsoncore"
	"strings"
)

func New() Document {
	return Document{}
}

func NewArray() *Array {
	return &Array{}
}

func NewElement(t bsontype.Type, v []byte) (r *Element, err error) {
	r = &Element{}
	err = r.Reset(t, v)
	return
}

func NewElementFromValue(v bsoncore.Value) (r *Element, err error) {
	return NewElement(v.Type, v.Data)
}

//func NewArrayElement(key string) *Element {
//	return &Element{key: key, val: bsoncore.Value{Type: bsontype.Array}}
//}
//
//func NewDocumentElement(key string) *Element {
//	return &Element{key: key, val: bsoncore.Value{Type: bsontype.EmbeddedDocument}}
//}

//func NewElementFromValue(key string, val interface{}) (ele *Element, err error) {
//	ele = &Element{key: key}
//	ele.val.Type, ele.val.Data, err = bson.MarshalValue(val)
//	return
//}

func Marshal(i interface{}) (r Document, err error) {
	var ok bool
	if r, ok = i.(Document); ok {
		return
	}
	var raw []byte
	switch v := i.(type) {
	case []byte:
		raw = v
	default:
		raw, err = bson.Marshal(v)
	}
	if err != nil {
		return
	}
	r = New()
	err = r.Reset(raw)
	return
}

func Split(key string) (string, string) {
	idx := strings.Index(key, ".")
	if idx < 0 {
		return key, ""
	} else {
		return key[0:idx], key[idx+1:]
	}
}
