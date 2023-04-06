package test

import (
	"fmt"
	"github.com/hwcer/updater/v2"
	"github.com/hwcer/updater/v2/dataset"
	"github.com/hwcer/updater/v2/operator"
)

var ITypeItem = &iTypeItem{iType{id: 30, unique: true}}

func init() {
	if err := updater.Register(updater.ParserTypeCollection, updater.RAMTypeMaybe, &Item{}, ITypeItem, ITypeEquip); err != nil {
		fmt.Printf("%v\n", err)
	}
}

// Item 普通道具
type Item struct {
	OID    string `bson:"_id" json:"_id"`
	UID    string `bson:"uid" json:"uid"`
	IID    int32  `bson:"iid" json:"iid"`
	Val    int64  `bson:"val" json:"val"`
	Attach string `bson:"attach" json:"attach"`
}

func (this *Item) Get(k string) any {
	switch k {
	case "_id", "OID":
		return this.OID
	case "uid", "UID":
		return this.UID
	case "iid", "IID":
		return this.IID
	case "val", "Val":
		return this.Val
	case "attach", "Attach":
		return this.Attach
	default:
		return nil
	}
}
func (this *Item) Set(k string, v any) (err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("%v", e)
		}
	}()
	switch k {
	case "_id", "OID":
		this.OID = v.(string)
	case "uid", "UID":
		this.UID = v.(string)
	case "iid", "IID":
		this.IID = v.(int32)
	case "val", "Val":
		this.Val = v.(int64)
	case "attach", "Attach":
		this.Attach = v.(string)
	default:
		err = fmt.Errorf("field not exist:%v", k)
	}
	return
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

type iTypeItem struct {
	iType
}

func (this *iTypeItem) New(u *updater.Updater, op *operator.Operator) (any, error) {
	v := updater.ParseInt64(op.Value)
	r := &Item{UID: u.Uid(), IID: op.IID, Val: v}
	r.OID, _ = this.CreateId(u, r.IID)
	fmt.Printf("New Item:%+v\n", r)
	return r, nil
}
