package bson

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"github.com/hwcer/logger"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/bsontype"
	"go.mongodb.org/mongo-driver/x/bsonx/bsoncore"
	"math"
	"strconv"
)

type Element struct {
	t bsontype.Type //
	v any           // Array,Document,[]byte
}

func (ele *Element) Len() int {
	switch ele.t {
	case bson.TypeArray:
		return ele.getOrCreateArr().Len()
	case bson.TypeEmbeddedDocument:
		return ele.getOrCreateDoc().Len()
	case bson.TypeNull:
		return 0
	default:
		return Length(ele.v.([]byte), ele.t)
	}
}
func (ele *Element) IsNil() bool {
	return ele.t == 0 || ele.v == bson.TypeNull
}

func (ele *Element) Type() bsontype.Type {
	return ele.t
}

func (ele *Element) IsNumber() bool {
	switch ele.t {
	case bson.TypeDouble, bson.TypeInt32, bson.TypeInt64, bson.TypeDecimal128:
		return true
	default:
		return false
	}
}

func (ele *Element) IsEmbedded() bool {
	return ele.t == bson.TypeArray || ele.t == bson.TypeEmbeddedDocument
}

func (ele *Element) getOrCreateArr() Array {
	if ele.t != bson.TypeArray {
		ele.t = bson.TypeArray
		ele.v = Array{}
	} else if ele.v == nil {
		ele.v = Array{}
	}
	r := ele.v.(Array)
	return r
}

func (ele *Element) getOrCreateDoc() Document {
	if ele.t != bson.TypeEmbeddedDocument {
		ele.t = bson.TypeEmbeddedDocument
		ele.v = Document{}
	} else if ele.v == nil {
		ele.v = Document{}
	}
	return ele.v.(Document)
}

func (ele *Element) Set(i any) error {
	return ele.Marshal(i)
}

// Pop 截取数组最后一位
//func (ele *Element) Pop() (*Element, error) {
//	if ele.t != bson.TypeArray {
//		return nil, ErrorElementNotSlice
//	}
//	return ele.getOrCreateArr().Pop()
//}

// Push i放入数组，Element必须为Array
func (ele *Element) Push(i any) error {
	if ele.t != bson.TypeArray {
		return ErrorElementNotSlice
	}
	return ele.getOrCreateArr().Push(i)
}

func (ele *Element) Unset(key string) error {
	if ele.t != bson.TypeEmbeddedDocument {
		return ErrorElementNotDocument
	}
	return ele.getOrCreateDoc().Unset(key)
}

func (ele *Element) Get(key string) *Element {
	switch ele.t {
	case bson.TypeArray:
		return ele.getOrCreateArr().Get(key)
	case bson.TypeEmbeddedDocument:
		return ele.getOrCreateDoc().Get(key)
	default:
		return ele
	}
}

func (ele *Element) GetBool() (r bool) {
	if ele.t != bson.TypeBoolean {
		return false
	}
	b := ele.v.([]byte)
	return b[0] == 0x01
}

func (ele *Element) GetInt32() (r int32) {
	if !ele.IsNumber() {
		return 0
	}
	return int32(binary.LittleEndian.Uint32(ele.v.([]byte)))
}

func (ele *Element) GetInt64() int64 {
	if !ele.IsNumber() {
		return 0
	}
	return int64(binary.LittleEndian.Uint64(ele.v.([]byte)))
}

func (ele *Element) GetFloat() float64 {
	if !ele.IsNumber() {
		return 0
	}
	v := ele.GetInt64()
	if v == 0 {
		return 0
	}
	return math.Float64frombits(uint64(v))
}

func (ele *Element) GetString() string {
	if ele.t != bson.TypeString {
		return ""
	}
	b := ele.v.([]byte)
	str, _, ok := bsoncore.ReadString(b)
	if !ok {
		logger.Alert(bsoncore.NewInsufficientBytesError(b, b))
	}
	return str
}

func (ele *Element) String() string {
	switch ele.t {
	case bson.TypeArray:
		return ele.getOrCreateArr().String()
	case bson.TypeEmbeddedDocument:
		return ele.getOrCreateDoc().String()
	case bson.TypeDouble:
		f64 := ele.GetFloat()
		return formatDouble(f64)
	case bson.TypeString:
		s := ele.GetString()
		b, _ := json.Marshal(s)
		return string(b)
	case bson.TypeBinary:
		b, _ := json.Marshal(ele.v.([]byte))
		return string(b)
	case bson.TypeUndefined:
		return "null"
	//case bson.TypeObjectID:
	//	oid, ok := v.ObjectIDOK()
	//	if !ok {
	//		return ""
	//	}
	//	return fmt.Sprintf(`{"$oid":"%s"}`, oid.Hex())
	case bson.TypeBoolean:
		b := ele.GetBool()
		return strconv.FormatBool(b)
	//case bsontype.DateTime:
	//	dt, ok := v.DateTimeOK()
	//	if !ok {
	//		return ""
	//	}
	//	return fmt.Sprintf(`{"$date":{"$numberLong":"%d"}}`, dt)
	case bson.TypeNull:
		return "null"
	//case bsontype.Regex:
	//	pattern, options, ok := v.RegexOK()
	//	if !ok {
	//		return ""
	//	}
	//	return fmt.Sprintf(
	//		`{"$regularExpression":{"pattern":%s,"options":"%s"}}`,
	//		escapeString(pattern), sortStringAlphebeticAscending(options),
	//	)
	//case bsontype.DBPointer:
	//	ns, pointer, ok := v.DBPointerOK()
	//	if !ok {
	//		return ""
	//	}
	//	return fmt.Sprintf(`{"$dbPointer":{"$ref":%s,"$id":{"$oid":"%s"}}}`, escapeString(ns), pointer.Hex())
	//case bsontype.JavaScript:
	//	js, ok := v.JavaScriptOK()
	//	if !ok {
	//		return ""
	//	}
	//	return fmt.Sprintf(`{"$code":%s}`, escapeString(js))
	//case bsontype.Symbol:
	//	symbol, ok := v.SymbolOK()
	//	if !ok {
	//		return ""
	//	}
	//	return fmt.Sprintf(`{"$symbol":%s}`, escapeString(symbol))
	//case bsontype.CodeWithScope:
	//	code, scope, ok := v.CodeWithScopeOK()
	//	if !ok {
	//		return ""
	//	}
	//	return fmt.Sprintf(`{"$code":%s,"$scope":%s}`, code, scope)
	case bson.TypeInt32:
		i32 := ele.GetInt32()
		return fmt.Sprintf("%d", i32)
	//case bsontype.Timestamp:
	//	t, i, ok := v.TimestampOK()
	//	if !ok {
	//		return ""
	//	}
	//	return fmt.Sprintf(`{"$timestamp":{"t":%v,"i":%v}}`, t, i)
	case bson.TypeInt64:
		i64 := ele.GetInt64()
		return fmt.Sprintf("%d", i64)
	//case bsontype.Decimal128:
	//	d128, ok := v.Decimal128OK()
	//	if !ok {
	//		return ""
	//	}
	//	return fmt.Sprintf(`{"$numberDecimal":"%s"}`, d128.String())
	//case bsontype.MinKey:
	//	return `{"$minKey":1}`
	//case bsontype.MaxKey:
	//	return `{"$maxKey":1}`
	default:
		return ""
	}
}

// Raw 返回 bsoncore.Element 形式的二进制
func (ele *Element) Raw(dst []byte, keys ...string) []byte {
	var l int
	var k string
	if len(keys) > 0 {
		k = keys[0]
		l = len(k) + 2
	}
	if dst == nil {
		dst = make([]byte, 0, l)
	}
	dst = bsoncore.AppendHeader(dst, ele.t, k)
	switch ele.t {
	case bson.TypeArray:
		dst = ele.getOrCreateArr().Raw(dst)
	case bson.TypeEmbeddedDocument:
		dst = ele.getOrCreateDoc().Raw(dst)
	default:
		dst = append(dst, ele.v.([]byte)...)
	}
	return dst
}

func (ele *Element) Value() bsoncore.Value {
	switch ele.t {
	case bson.TypeArray:
		return ele.getOrCreateArr().Value()
	case bson.TypeEmbeddedDocument:
		return ele.getOrCreateDoc().Value()
	default:
		return bsoncore.Value{Type: ele.t, Data: ele.v.([]byte)}
	}
}

func (ele *Element) Reset(t bsontype.Type, b []byte) error {
	switch t {
	case bson.TypeArray:
		return ele.getOrCreateArr().Reset(b)
	case bson.TypeEmbeddedDocument:
		return ele.getOrCreateDoc().Reset(b)
	default:
		ele.t, ele.v = t, b
	}
	return nil
}

// Marshal 将数据放入元素中
func (ele *Element) Marshal(i interface{}) error {
	t, b, err := bson.MarshalValue(i)
	if err != nil {
		return err
	}
	return ele.Reset(t, b)
}

func (ele *Element) Unmarshal(i interface{}) (err error) {
	switch ele.t {
	case bson.TypeArray:
		return ele.getOrCreateArr().Unmarshal(i)
	case bson.TypeEmbeddedDocument:
		return ele.getOrCreateDoc().Unmarshal(i)
	default:
		raw := bson.RawValue{Value: ele.v.([]byte), Type: ele.t}
		return raw.Unmarshal(i)
	}
}

func (ele *Element) loadOrCreate(key string) (r *Element, loaded bool) {
	switch ele.t {
	case bson.TypeArray:
		return ele.getOrCreateArr().loadOrCreate(key)
	case bson.TypeEmbeddedDocument:
		return ele.getOrCreateDoc().loadOrCreate(key)
	default:
		return ele, true
	}
}
