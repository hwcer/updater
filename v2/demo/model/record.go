package model

import (
	"fmt"
	"github.com/hwcer/updater"
	"github.com/hwcer/updater/demo/config"
	"math/rand"
)

var RecordIType = NewIType(config.ITypeRecord)

func init() {
	if err := updater.Register(updater.ParserTypeHash, updater.RAMTypeAlways, &Record{}, RecordIType); err != nil {
		fmt.Printf("%v\n", err)
	}
}

type Record struct {
	Model
}

func (this *Record) Symbol(u *updater.Updater) any {
	return ""
}

func (this *Record) Getter(u *updater.Updater, symbol any, keys []int32) (r map[int32]int64, err error) {
	r = map[int32]int64{}
	//内存模式只会拉所有
	if len(keys) > 0 {
		return
	}
	for id, c := range config.Configs {
		if c.IType == RecordIType.Id() {
			r[id] = rand.Int63n(10000)
		}
	}
	//模拟数据
	fmt.Printf("====== Record Getter keys:%v \n", keys)
	return
}

func (this *Record) Setter(u *updater.Updater, symbol any, value map[int32]int64) error {
	fmt.Printf("====== Record Setter update:%v \n", value)
	return nil
}
