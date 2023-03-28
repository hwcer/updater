package updater

import (
	"fmt"
	"github.com/hwcer/updater/v2/dirty"
)

var hashParseHandle = make(map[dirty.Operator]func(*Hash, *dirty.Cache) error)

func init() {
	hashParseHandle[dirty.OperatorTypeDel] = hashParseDel
	hashParseHandle[dirty.OperatorTypeAdd] = hashParseAdd
	hashParseHandle[dirty.OperatorTypeSub] = hashParseSub
	hashParseHandle[dirty.OperatorTypeMax] = hashParseMax
	hashParseHandle[dirty.OperatorTypeMin] = hashParseMin
	hashParseHandle[dirty.OperatorTypeSet] = hashParseSet
}

func (this *Hash) Parse(act *dirty.Cache) error {
	if f, ok := hashParseHandle[act.Operator]; ok {
		return f(this, act)
	}
	return fmt.Errorf("map parser not exist:%v", act)
}

func hashParseAdd(this *Hash, cache *dirty.Cache) (err error) {
	v := ParseInt64(cache.Value)
	r := this.val(cache.IID) + v
	cache.Result = r
	this.values[cache.IID] = r
	return
}

func hashParseSub(this *Hash, cache *dirty.Cache) (err error) {
	d := this.val(cache.IID)
	v := ParseInt64(cache.Value)
	if d < v {
		if this.Updater.tolerance {
			cache.Operator = dirty.OperatorTypeDrop
		} else {
			err = ErrItemNotEnough(cache.IID, v, d)
		}
		return
	}
	r := d - v
	cache.Result = r
	this.values[cache.IID] = r
	return
}

func hashParseMax(this *Hash, cache *dirty.Cache) (err error) {
	v := ParseInt64(cache.Value)
	if d := this.val(cache.IID); v > d {
		cache.Result = v
		this.values[cache.IID] = v
		cache.Operator = dirty.OperatorTypeSet
	} else {
		cache.Result = dirty.OperatorTypeDrop
	}
	return
}

func hashParseMin(this *Hash, cache *dirty.Cache) (err error) {
	v := ParseInt64(cache.Value)
	if d := this.val(cache.IID); v < d {
		cache.Result = v
		this.values[cache.IID] = v
		cache.Operator = dirty.OperatorTypeSet
	} else {
		cache.Operator = dirty.OperatorTypeDrop
	}
	return
}

func hashParseSet(this *Hash, cache *dirty.Cache) (err error) {
	v := ParseInt64(cache.Value)
	cache.Result = v
	this.values[cache.IID] = v
	return
}
func hashParseDel(this *Hash, cache *dirty.Cache) (err error) {
	cache.Result = 0
	this.values[cache.IID] = 0
	return
}
