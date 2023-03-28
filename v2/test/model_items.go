package test

import (
	"fmt"
	"github.com/hwcer/updater/v2"
	"github.com/hwcer/updater/v2/dirty"
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
	VAL    int64  `bson:"val"`
	Attach string `bson:"attach"`
}

// Init 获取所有列表
func (this *Item) Init(u *updater.Updater, fn updater.Receive) error {
	fmt.Printf("item init\n")
	return nil
}
func (this *Item) Getter(u *updater.Updater, keys []string, fn updater.Receive) error {
	fmt.Printf("item Getter:%v\n", keys)
	return nil
}
func (this *Item) Setter(u *updater.Updater, bulkWrite dirty.BulkWrite) error {
	fmt.Printf("item Setter\n")
	return nil
}
func (this *Item) BulkWrite(u *updater.Updater) dirty.BulkWrite {
	return &BulkWrite{}
}

type iTypeItem struct {
	iType
}

func (this *iTypeItem) New(u *updater.Updater, cache *dirty.Cache) (any, error) {
	v := updater.ParseInt64(cache.Value)
	r := &Item{UID: u.Uid(), IID: cache.IID, VAL: v}
	r.OID, _ = this.CreateId(u, r.IID)
	fmt.Printf("New Item:%+v\n", r)
	return r, nil
}
