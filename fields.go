package updater

import (
	"github.com/hwcer/cosmo/clause"
)

type fields struct {
	fields  []interface{}
	history map[interface{}]bool
}

func NewFields() *fields {
	return &fields{
		history: make(map[interface{}]bool),
	}
}

func (this *fields) Keys(keys ...interface{}) {
	var arr []interface{}
	for _, k := range keys {
		if !this.Has(k) {
			arr = append(arr, k)
		}
	}
	this.fields = append(this.fields, arr...)
}

func (this *fields) Has(k interface{}) bool {
	if _, ok := this.history[k]; ok {
		return true
	}
	for _, f := range this.fields {
		if f == k {
			return true
		}
	}
	return false
}

func (this *fields) reset() {
	for _, k := range this.fields {
		this.history[k] = true
	}
	this.fields = nil
}

func (this *fields) release() {
	this.fields = nil
	this.history = make(map[interface{}]bool)
}

func (this *fields) String() (r []string) {
	for _, v := range this.fields {
		if s, ok := v.(string); ok {
			r = append(r, s)
		}
	}
	return
}

func (this *fields) Query() clause.Filter {
	if len(this.fields) == 0 {
		return nil
	}
	var oid []string
	var iid []interface{}
	for _, v := range this.fields {
		switch v.(type) {
		case string:
			oid = append(oid, v.(string))
		case int, int32, int64, uint, uint32, uint64:
			iid = append(iid, v)
		}
	}
	query := clause.Filter{}
	if len(oid) > 0 && len(iid) > 0 {
		q1 := clause.Filter{}
		for _, v := range oid {
			q1.Primary(v)
		}
		q2 := clause.Filter{}
		for _, v := range iid {
			q2.Eq(ItemNameIID, v)
		}
		query.OR([]interface{}{q1, q2})
	} else if len(oid) > 0 {
		for _, v := range oid {
			query.Primary(v)
		}
	} else if len(iid) > 0 {
		for _, v := range iid {
			query.Eq(ItemNameIID, v)
		}
	}
	return query
}
