package updater

import (
	"github.com/hwcer/cosmo/clause"
)

type Fields struct {
	keys    []string //_id
	history map[string]bool
}

func NewFields() *Fields {
	return &Fields{
		history: make(map[string]bool),
	}
}

func (this *Fields) Has(k string) bool {
	return this.history[k]
}

func (this *Fields) Select(keys ...string) (r int) {
	for _, k := range keys {
		if !this.Has(k) {
			r++
			this.keys = append(this.keys, k)
			this.history[k] = true
		}
	}
	return
}

func (this *Fields) done() {
	this.keys = nil
}

func (this *Fields) reset() {
	this.history = make(map[string]bool)
}

func (this *Fields) release() {
	this.keys = nil
	this.history = nil
}

func (this *Fields) Query() clause.Filter {
	if len(this.keys) == 0 {
		return nil
	}
	query := clause.Filter{}
	for _, v := range this.keys {
		query.Primary(v)
	}
	return query
}
