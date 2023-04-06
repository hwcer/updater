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
	OID    string `bson:"_id"`
	UID    string `bson:"uid"`
	IID    int32  `bson:"iid"`
	Val    int64  `bson:"val"`
	Attach string `bson:"attach"`
}

func (this *Item) Clone() any {
	r := *this
	return &r
}

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
