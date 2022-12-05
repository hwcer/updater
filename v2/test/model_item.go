package test

import (
	"fmt"
	"github.com/hwcer/adapter"
	"github.com/hwcer/adapter/bson"
	"github.com/hwcer/cosmo/clause"
	"github.com/hwcer/logger"
)

var ITypeItem = &iTypeItem{iType{id: 30, unique: true}}

func init() {
	ITypeItem.Register(3001, 3002, 3003, 3004, 3005)
	if err := updater.Register(&Item{}, ITypeItem); err != nil {
		fmt.Printf("%v\n", err)
	}
}

type Item struct {
	OID string `bson:"_id"`
	UID string `bson:"uid"`
	IID int32  `bson:"iid"`
	VAL int64  `bson:"val"`
}

func (this *Item) Parser() updater.Parser {
	return updater.ParserTypeCollection
}
func (this *Item) Getter(adapter *updater.Updater, filter clause.Filter) (r []any, err error) {
	logger.Info("Item Getter filter:%v", filter)
	return
}
func (this *Item) BulkWrite(adapter *updater.Updater, model any) updater.BulkWrite {
	return &BulkWrite{}
}

//func (this *Item) Setter(_ *adapter.Updater, update update.Update) error {
//	logger.Info("Record Setter update:%v", update)
//	return nil
//}

func (this *Item) SetOnInsert() (r map[string]interface{}, err error) {
	r = make(map[string]interface{})
	r["uid"] = this.UID
	r["iid"] = this.IID
	return
}

type iTypeItem struct {
	iType
}

func (this *iTypeItem) New(a *updater.Updater, cache *updater.Cache) (any, error) {
	v := bson.ParseInt64(cache.Val)
	r := &Item{UID: a.Uid(), IID: cache.IID, VAL: v}
	r.OID, _ = this.CreateId(a, r.IID)
	logger.Info("New Item:%+v", r)
	return r, nil
}
