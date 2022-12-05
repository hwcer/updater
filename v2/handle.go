package updater

type base struct {
	acts    []*Cache
	cache   []*Cache
	Error   error
	Fields  *Fields
	Adapter *Updater
}

func NewBase(adapter *Updater) (b *base) {
	b = &base{
		acts:    make([]*Cache, 0),
		Fields:  NewFields(),
		Adapter: adapter,
	}
	return
}

func (b *base) reset() {
	b.Fields.reset()
}

func (b *base) release() {
	b.acts = nil
	b.cache = nil
	b.Error = nil
	b.Fields.release()
}

func (b *base) Act(act *Cache, before ...bool) {
	if len(before) > 0 && before[0] {
		b.acts = append([]*Cache{act}, b.acts...)
	} else {
		b.acts = append(b.acts, act)
	}
}

func (b *base) Has(key string) bool {
	return b.Fields.Has(key)
}

func (b *base) Cache() []*Cache {
	return b.cache
}

// Select 字段名(HASH)或者OID(table)
func (b *base) Select(keys ...string) {
	if r := b.Fields.Select(keys...); r > 0 {
		b.Adapter.changed = true
	}
}
