package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/hwcer/cosgo/values"
	"github.com/hwcer/updater"
	"github.com/hwcer/updater/dataset"
	"github.com/hwcer/updater/demo/config"
	"github.com/hwcer/updater/operator"
)

var ItemIType = &itemIType{}

func init() {
	ItemIType.id = config.ITypeItem
	ItemIType.register(ItemIType)
}

func Start() error {
	return updater.Register(updater.ParserTypeCollection, updater.RAMTypeNone, &Item{}, ItemIType.itypes...)
}

type Item struct {
	Model  `bson:"inline"`
	Used   string        `json:"used"` //装备佩戴heroid
	Attach values.Attach `json:"attach" bson:"attach"`
}

type itemIType struct {
	IType
	itypes []updater.IType
}

func (this *itemIType) register(it updater.IType) {
	this.itypes = append(this.itypes, it)
}

// Clone 复制对象,可以将NEW新对象与SET操作解绑
func (this *Item) Clone() any {
	r := *this
	return &r
}

// ----------------- 作为MODEL方法--------------------
func (this *Item) Getter(u *updater.Updater, keys []string, fn updater.Receive) error {
	fmt.Printf("====== item Getter:%v\n", keys)
	return nil
}
func (this *Item) Setter(u *updater.Updater, bulkWrite dataset.BulkWrite) error {
	fmt.Printf("====== item Setter\n")
	return nil
}
func (this *Item) BulkWrite(u *updater.Updater) dataset.BulkWrite {
	fmt.Printf("====== item BulkWrite\n")
	return &BulkWrite{}
}

// Listener 监控道具变化
func (this *Item) Listener(u *updater.Updater, op *operator.Operator) {
	it := u.IType(op.IID)
	if it == nil {
		return
	}
	if l, ok := it.(updater.ModelListener); ok {
		l.Listener(u, op)
	}
}

func (this *Item) Get(k string) any {
	switch k {
	case "Attach", "attach":
		return this.Attach
	case "Used", "used":
		return this.Used
	default:
		return this.Model.Get(k)
	}
}

func (this *Item) Set(k string, v any) (err error) {
	switch k {
	case "Attach", "attach":
		return this.SetAttach(v)
	case "Used", "used":
		if s, ok := v.(string); ok {
			this.Used = s
		} else {
			return errors.New("error in type")
		}
	default:
		return this.Model.Set(k, v)
	}
	return
}

func (this *Item) SetAttach(i any) error {
	v, err := json.Marshal(i)
	if err == nil {
		this.Attach = values.Attach(v)
	}
	return err
}

func (this *Item) GetAttach(i any) (err error) {
	if len(this.Attach) == 0 {
		return
	}
	return json.Unmarshal([]byte(this.Attach), i)
}

func (this *itemIType) New(u *updater.Updater, op *operator.Operator) (any, error) {
	i := &Item{}
	if oid, err := this.ObjectId(u, op.IID); err != nil {
		return nil, err
	} else {
		i.OID = oid
	}
	i.Uid = u.Uid()
	i.IID = op.IID
	i.Val = op.Value
	return i, nil
}

func (this *itemIType) ObjectId(u *updater.Updater, iid int32) (string, error) {
	return ObjectId(u, iid, false)
}

// Listener  处理普通道具获得与扣除时对应成就 todo
func (this *itemIType) Listener(u *updater.Updater, op *operator.Operator) {
	if op.Type == operator.Types_Add {
		//u.Select(1111)  碎片ID
	}
}
