package bson

import (
	"fmt"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/bsontype"
	"go.mongodb.org/mongo-driver/x/bsonx/bsoncore"
	"strings"
)

type Document map[string]*Element

func (doc Document) Type() bsontype.Type {
	return bson.TypeEmbeddedDocument
}

func (doc Document) Len() (r int) {
	r += 5 //size(int32) + 0x00
	for k, ele := range doc {
		r += len(k) + 2
		r += ele.Len()
	}
	return
}

func (doc Document) Keys() (r []string) {
	for k, _ := range doc {
		r = append(r, k)
	}
	return
}

func (doc Document) Has(key string) bool {
	if _, ok := doc[key]; ok {
		return true
	}
	return false
}

// Get Element别名
func (doc Document) Get(key string) (r *Element) {
	k1, k2 := Split(key)
	r = doc[k1]
	if r != nil && k2 != "" {
		r = r.Get(k2)
	}
	return
}

func (doc Document) Set(key string, i any) error {
	ele, _ := doc.loadOrCreate(key)
	return ele.Set(i)
}
func (doc Document) Push(i any) error {
	return ErrorElementNotSlice
}

// Unset 删除key
func (doc Document) Unset(key string) (err error) {
	k1, k2 := Split(key)
	if k2 == "" {
		delete(doc, k1)
	} else if r := doc[k1]; r != nil {
		return r.Unset(k2)
	}
	return
}

func (doc Document) GetBool(key string) (r bool) {
	if ele := doc.Get(key); ele != nil {
		r = ele.GetBool()
	}
	return
}

func (doc Document) GetInt32(key string) (r int32) {
	if ele := doc.Get(key); ele != nil {
		r = ele.GetInt32()
	}
	return
}

func (doc Document) GetInt64(key string) (r int64) {
	if ele := doc.Get(key); ele != nil {
		r = ele.GetInt64()
	}
	return
}

func (doc Document) GetFloat(key string) (r float64) {
	if ele := doc.Get(key); ele != nil {
		r = ele.GetFloat()
	}
	return
}

func (doc Document) GetString(key string) (r string) {
	if ele := doc.Get(key); ele != nil {
		r = ele.GetString()
	}
	return
}

func (doc Document) Raw(dst []byte, key ...string) []byte {
	var l int
	var k string
	if len(key) > 0 {
		k = key[0]
		l = len(key) + 2
	}
	if dst == nil {
		l += doc.Len()
		dst = make([]byte, 0, l)
	}
	if len(key) > 0 {
		dst = bsoncore.AppendHeader(dst, bson.TypeEmbeddedDocument, k)
	}
	idx, dst := bsoncore.ReserveLength(dst)
	for s, e := range doc {
		dst = e.Raw(dst, s)
	}
	dst = append(dst, 0x00)
	dst = bsoncore.UpdateLength(dst, idx, int32(len(dst[idx:])))
	return dst
}

func (doc Document) Value() bsoncore.Value {
	dst := doc.Raw(nil)
	return bsoncore.Value{Type: bson.TypeEmbeddedDocument, Data: dst}
}

func (doc Document) String() string {
	var buf strings.Builder
	first := true
	buf.WriteByte('{')
	for k, e := range doc {
		if !first {
			buf.WriteString(",")
		}
		first = false
		buf.WriteString(fmt.Sprintf(`"%s":`, k))
		buf.WriteString(e.String())
	}
	buf.WriteByte('}')

	return buf.String()
}

func (doc Document) Reset(val []byte) (err error) {
	raw := bsoncore.Document(val)
	if err = raw.Validate(); err != nil {
		return err
	}
	arr, err := raw.Elements()
	if err != nil {
		return err
	}
	for _, v := range arr {
		var ele *Element
		if ele, err = NewElementFromValue(v.Value()); err != nil {
			return err
		} else {
			k := v.Key()
			doc[k] = ele
		}
	}
	return nil
}
func (doc Document) Marshal(i any) (err error) {
	t, b, err := bson.MarshalValue(i)
	if err != nil {
		return
	}
	if t != bson.TypeEmbeddedDocument {
		return ErrorElementNotDocument
	}
	return doc.Reset(b)
}

func (doc Document) Unmarshal(i any) error {
	raw := doc.Raw(nil)
	return bson.Unmarshal(raw, i)
}

func (doc Document) loadOrCreate(key string) (r *Element, loaded bool) {
	k1, k2 := Split(key)
	r, loaded = doc[k1]
	if !loaded {
		r, _ = NewElement(bson.TypeNull, nil)
		doc[k1] = r
	}
	if k2 == "" {
		return
	}

	if !loaded {
		r.t = bson.TypeEmbeddedDocument
	}
	return r.loadOrCreate(k2)
}
