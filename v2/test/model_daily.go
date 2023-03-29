package test

import (
	"fmt"
	"github.com/hwcer/updater/v2"
)

var ITypeDaily = &iType{id: 20, unique: true}

func init() {
	if err := updater.Register(updater.ParserTypeHash, updater.RAMTypeAlways, &Daily{}, ITypeDaily); err != nil {
		fmt.Printf("%v\n", err)
	}
}

type Daily struct{}

func (this *Daily) Init(u *updater.Updater, symbol any) (map[int32]int64, error) {
	fmt.Printf("Daily Init:%v \n", symbol)
	return map[int32]int64{}, nil
}
func (this *Daily) Symbol(u *updater.Updater) any {
	r, _ := u.Time().Sign(0)
	fmt.Printf("Daily Symbol:%v \n", r)
	return r
}

func (this *Daily) Getter(u *updater.Updater, symbol any, keys []int32) (map[int32]int64, error) {
	r := map[int32]int64{}
	fmt.Printf("====== Daily Getter Symbol:%v keys:%v \n", symbol, keys)
	return r, nil
}

func (this *Daily) Setter(u *updater.Updater, symbol any, update map[int32]int64) error {
	fmt.Printf("====== Daily Setter Symbol:%v update:%v \n", symbol, update)
	return nil
}
