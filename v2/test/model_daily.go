package test

import (
	"errors"
	"fmt"
	"github.com/hwcer/updater/v2"
	"github.com/hwcer/updater/v2/dirty"
)

var ITypeDaily = &iTypeDaily{iType{id: 20, unique: true}}

var DailyRows = map[int32]int64{}

func init() {
	for i, k := range []int32{2001, 2002, 2003, 2004, 2005} {
		DailyRows[k] = int64(i)
	}

	if err := updater.Register(updater.ParserTypeHash, updater.RAMTypeAlways, &Daily{}, ITypeDaily); err != nil {
		fmt.Printf("%v\n", err)
	}
}

type Daily struct{}

func (this *Daily) Init(u *updater.Updater, symbol any) (map[int32]int64, error) {
	fmt.Printf("Daily Init:%v \n", symbol)
	return DailyRows, nil
}
func (this *Daily) Symbol(u *updater.Updater) any {
	r, _ := u.Time().Sign(0)
	fmt.Printf("Daily Symbol:%v \n", r)
	return r
}

func (this *Daily) Getter(u *updater.Updater, symbol any, keys []int32) (map[int32]int64, error) {
	r := map[int32]int64{}
	for _, k := range keys {
		r[k] = DailyRows[k]
	}
	fmt.Printf("Daily Getter Symbol:%v keys:%v \n", symbol, keys)
	return r, nil
}

func (this *Daily) Setter(u *updater.Updater, symbol any, update map[int32]int64) error {
	fmt.Printf("Daily Setter Symbol:%v update:%v \n", symbol, update)
	for k, v := range update {
		DailyRows[k] = v
	}
	return nil
}

type iTypeDaily struct {
	iType
}

func (this *iTypeDaily) Id() int32 {
	return this.id
}

func (this *iTypeDaily) New(_ *updater.Updater, cache *dirty.Cache) (item any, err error) {
	return nil, errors.New("daily不允许自动创建")
}

func (this *iTypeDaily) CreateId(_ *updater.Updater, iid int32) (string, error) {
	return "", fmt.Errorf("daily不应该进入这里:%v", iid)
}
