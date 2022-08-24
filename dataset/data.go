package dataset

func NewData(item IModel) *Data {
	i := &Data{IModel: item}
	return i
}

type Data struct {
	IModel
}

func (this *Data) GetInt(key string) (int64, bool) {
	v := this.Get(key)
	if v == nil {
		return 0, false
	}
	return ParseInt(v)
}

func (this *Data) GetInt32(key string) (int32, bool) {
	v := this.Get(key)
	if v == nil {
		return 0, false
	}
	return ParseInt32(v)
}

func (this *Data) GetString(key string) (r string, ok bool) {
	v := this.Get(key)
	if v == nil {
		return "", false
	}
	r, ok = v.(string)
	return
}

func (this *Data) MSet(vs map[string]any) (err error) {
	for k, v := range vs {
		if _, err = this.Set(k, v); err != nil {
			return
		}
	}
	return
}
