package updater

type flags map[string]interface{}

func (this flags) Set(k string, v interface{}) {
	this[k] = v
}

func (this flags) Get(k string) (v interface{}, ok bool) {
	v, ok = this[k]
	return
}

func (this flags) Has(k string) (ok bool) {
	_, ok = this[k]
	return
}

func (this flags) Del(k string) {
	delete(this, k)
}
