package updater

type base struct {
	acts    []*Cache
	cache   []*Cache
	model   *Model
	fields  *fields
	updater *Updater
	errMsg  error
}

func NewBase(model *Model, updater *Updater) (b *base) {
	b = &base{
		acts:    make([]*Cache, 0),
		model:   model,
		updater: updater,
		fields:  NewFields(),
	}
	return
}

func (b *base) release() {
	b.acts = nil
	b.cache = nil
	b.errMsg = nil
	b.fields.release()
}

func (b *base) Act(act *Cache, before ...bool) {
	if len(before) > 0 && before[0] {
		b.acts = append([]*Cache{act}, b.acts...)
	} else {
		b.acts = append(b.acts, act)
	}
}

func (b *base) Has(key string) bool {
	return b.fields.Has(key)
}

func (b *base) Cache() []*Cache {
	return b.cache
}

func (b *base) Keys(keys ...interface{}) {
	b.fields.Keys(keys...)
	b.updater.changed = true
}

//Fields 字段名(HASH)或者OID(table)
func (b *base) Fields(keys ...string) {
	for _, k := range keys {
		b.fields.Keys(k)
	}
	b.updater.changed = true
}

func (b *base) Updater() *Updater {
	return b.updater
}
