package updater

import (
	"errors"
	"github.com/hwcer/updater/v2/dirty"
)

var documentParseHandle = make(map[dirty.Operator]func(*Document, *dirty.Cache) error)

func init() {
	documentParseHandle[dirty.OperatorTypeAdd] = documentParseAdd
	documentParseHandle[dirty.OperatorTypeSet] = documentParseSet
	documentParseHandle[dirty.OperatorTypeSub] = documentParseSub
	documentParseHandle[dirty.OperatorTypeMax] = documentParseMax
	documentParseHandle[dirty.OperatorTypeMin] = documentParseMin
}

func (this *Document) Parse(act *dirty.Cache) error {
	if f, ok := documentParseHandle[act.Operator]; ok {
		return f(this, act)
	}
	return errors.New("hash_act_parser not exist")
}

func documentParseAdd(this *Document, cache *dirty.Cache) (err error) {
	v := ParseInt64(cache.Value)
	r := this.val(cache.Key) + v
	cache.Result = r
	this.values[cache.Key] = r
	return
}

func documentParseSub(this *Document, cache *dirty.Cache) (err error) {
	d := this.val(cache.Key)
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
	this.values[cache.Key] = r
	return
}

func documentParseSet(this *Document, cache *dirty.Cache) (err error) {
	cache.Result = cache.Value
	if r, ok := TryParseInt64(cache.Value); ok {
		this.values[cache.Key] = r
	}
	return
}

func documentParseMax(this *Document, cache *dirty.Cache) (err error) {
	v := ParseInt64(cache.Value)
	if d := this.val(cache.Key); v > d {
		cache.Result = v
		this.values[cache.Key] = v
		cache.Operator = dirty.OperatorTypeSet
	} else {
		cache.Result = dirty.OperatorTypeDrop
	}
	return
}

func documentParseMin(this *Document, cache *dirty.Cache) (err error) {
	v := ParseInt64(cache.Value)
	if d := this.val(cache.Key); v < d {
		cache.Result = v
		this.values[cache.Key] = v
		cache.Operator = dirty.OperatorTypeSet
	} else {
		cache.Operator = dirty.OperatorTypeDrop
	}
	return
}
