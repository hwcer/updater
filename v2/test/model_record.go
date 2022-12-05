package test

import (
	"errors"
	"fmt"
	"github.com/hwcer/adapter"
	"github.com/hwcer/cosmo/update"
	"github.com/hwcer/logger"
)

var ITypeRecord = &iTypeRecord{id: 20}

func init() {
	ITypeRecord.Register(2001, 2002, 2003, 2004, 2005)
	if err := updater.Register(&Record{}, ITypeRecord); err != nil {
		fmt.Printf("%v\n", err)
	}
}

type Record struct {
	Oid string           `bson:"_id"`
	Uid string           `bson:"uid"`
	Val map[string]int64 `bson:"val"`
}

func (this *Record) Parser() updater.Parser {
	return updater.ParserTypeHash
}

func (this *Record) Getter(adapter *updater.Updater, keys []string) (map[string]int64, error) {
	r := map[string]int64{}
	for i, k := range keys {
		r[k] = int64(i)
	}
	return r, nil
}

func (this *Record) Setter(_ *updater.Updater, update update.Update) error {
	logger.Info("Record Setter update:%v", update)
	return nil
}

type iTypeRecord struct {
	id int32
}

func (this *iTypeRecord) Id() int32 {
	return this.id
}

func (this *iTypeRecord) New(_ *updater.Updater, cache *updater.Cache) (item any, err error) {
	return nil, errors.New("Record不允许自动创建")
}

func (this *iTypeRecord) Unique() bool {
	return true
}

func (this *iTypeRecord) CreateId(_ *updater.Updater, iid int32) (string, error) {
	return "", fmt.Errorf("Record不应该进入这里:%v", iid)
}

func (this *iTypeRecord) Register(iid ...int32) {
	for _, id := range iid {
		itypes[id] = this.Id()
	}
}
