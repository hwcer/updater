package updater

import (
	"github.com/hwcer/cosmo/clause"
)

type fields struct {
	keys    []string //_id
	history map[string]bool
}

func NewFields() *fields {
	return &fields{
		history: make(map[string]bool),
	}
}

func (this *fields) Has(k string) bool {
	if _, ok := this.history[k]; ok {
		return true
	}
	return false
}

func (this *fields) Select(keys ...string) {
	for _, k := range keys {
		if !this.Has(k) {
			this.keys = append(this.keys, k)
			this.history[k] = true
		}
	}
}

func (this *fields) reset() {
	this.keys = nil
}

func (this *fields) release() {
	this.keys = nil
	this.history = make(map[string]bool)
}

func (this *fields) Query(uid string) clause.Filter {
	if len(this.keys) == 0 {
		return nil
	}
	query := clause.Filter{}
	for _, v := range this.keys {
		query.Primary(v)
	}
	//if len(iid) > 0 {
	//	query.Eq(ItemNameUID, uid)
	//}

	return query
}
