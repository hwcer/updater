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

//New 创建*model对象
func (b *base) New() interface{} {
	if m, ok := b.model.Model.(ModelNew); ok {
		return m.New()
	} else {
		return b.model.Schema.New().Interface()
	}
}

//MakeSlice 创建[]*model 数组对象
func (b *base) MakeSlice() interface{} {
	if m, ok := b.model.Model.(ModelMakeSlice); ok {
		return m.MakeSlice()
	} else {
		return b.model.Schema.MakeSlice().Interface()
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

func (b *base) Updater() *Updater {
	return b.updater
}
