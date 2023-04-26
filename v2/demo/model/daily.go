package model

import (
	"fmt"
	"github.com/hwcer/cosgo/utils"
	"github.com/hwcer/updater"
	"github.com/hwcer/updater/dataset"
	"github.com/hwcer/updater/demo/config"
	"math/rand"
)

var DailyIType = NewIType(config.ITypeDaily)

func init() {
	if err := updater.Register(updater.ParserTypeHash, updater.RAMTypeAlways, &Daily{}, DailyIType); err != nil {
		fmt.Printf("%v\n", err)
	}
}

func NewDaily() *Daily {
	return &Daily{
		Attach: map[int32]int64{},
	}
}

type Daily struct {
	OID    string          `bson:"_id" json:"_id"`
	Uid    string          `bson:"uid" json:"uid,omitempty"  index:"name:_idx_uid_sign,priority:1" `
	Sign   int32           `json:"sign" bson:"sign omitempty" index:"name:_idx_uid_sign,sort:DESC,priority:2;"`
	Attach map[int32]int64 `json:"attach" bson:"attach"`
}

func (this *Daily) Symbol(u *updater.Updater) any {
	r, _ := utils.Time.New(u.Time).Sign(0)
	//fmt.Printf("Daily Symbol:%v \n", r)
	return r
}

func (this *Daily) Getter(u *updater.Updater, symbol any, keys []int32) (r map[int32]int64, err error) {
	r = map[int32]int64{}
	//内存模式只会拉所有
	if len(keys) > 0 {
		return
	}
	//id := fmt.Sprintf("daily-%v-%v", u.Uid(), symbol)
	for id, c := range config.Configs {
		if c.IType == DailyIType.Id() {
			r[id] = rand.Int63n(10000)
		}
	}
	//模拟数据
	fmt.Printf("====== Daily Getter Symbol:%v keys:%v \n", symbol, keys)
	return
}

func (this *Daily) Setter(u *updater.Updater, symbol any, value map[int32]int64) error {
	//return errors.New("测试数据同步失败")
	//生成mongo 语句
	data := map[string]int64{}
	format := "attach.%v"
	for k, v := range value {
		field := fmt.Sprintf(format, k)
		data[field] = v
	}
	update := map[string]any{}
	update["$set"] = data

	setOnInsert := map[string]any{}
	setOnInsert["uid"] = u.Uid()
	setOnInsert["sign"] = dataset.ParseInt32(symbol)
	update["SetOnInsert"] = setOnInsert

	id := fmt.Sprintf("daily-%v-%v", u.Uid(), symbol)
	fmt.Printf("====== Daily Setter ID:%v update:%v \n", id, update)
	return nil
}
