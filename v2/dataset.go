package updater

import (
	"github.com/hwcer/adapter/bson"
)

type Dataset map[string]int64

func (ds Dataset) Get(k string) int64 {
	return ds[k]
}

func (ds Dataset) Set(k string, v int64) {
	ds[k] = v
}

// Min 如果小于原值就替换，否则就返回一个错误
// 使用 bson.ErrorNotChange 来判断是逻辑错误还是数据无改变
func (ds Dataset) Min(k string, v int64) (r int64, err error) {
	if d, ok := ds[k]; !ok || v < d {
		ds[k] = v
	} else {
		err = bson.ErrorNotChange
	}
	r = ds[k]
	return
}

// Max 如果大于原值就替换，否则就返回一个错误
// 使用 bson.ErrorNotChange 来判断是逻辑错误还是数据无改变
func (ds Dataset) Max(k string, v int64) (r int64, err error) {
	if d, ok := ds[k]; !ok || v > d {
		ds[k] = v
	} else {
		err = bson.ErrorNotChange
	}
	r = ds[k]
	return
}

// Inc 自增
func (ds Dataset) Inc(k string, v int64) (r int64) {
	ds[k] += v
	r = ds[k]
	return
}

func (ds Dataset) Mul(k string, v int64) (r int64) {
	ds[k] *= v
	r = ds[k]
	return
}

// Unset 删除key
func (ds Dataset) Del(k string) {
	delete(ds, k)
	return
}
