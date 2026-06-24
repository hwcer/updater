package dataset

func NewValues() *Values {
	return &Values{}
}

type Data map[int32]int64

func (d Data) Get(k int32) (v int64, ok bool) {
	v, ok = d[k]
	return
}
func (d Data) Set(k int32, v int64) {
	d[k] = v
}
func (d Data) Has(k int32) (ok bool) {
	_, ok = d[k]
	return
}
func (d Data) Del(k int32) {
	delete(d, k)
}

type Values struct {
	data  Data
	dirty Data
	unset map[int32]struct{}
}

func (val *Values) Len() int {
	return len(val.data)
}
func (val *Values) Has(k int32) (r bool) {
	if val.dirty.Has(k) {
		return true
	}
	return val.data.Has(k)
}
func (val *Values) Val(k int32) (r int64) {
	r, _ = val.Get(k)
	return
}

func (val *Values) Get(k int32) (r int64, ok bool) {
	if r, ok = val.dirty.Get(k); ok {
		return
	}
	return val.data.Get(k)
}

func (val *Values) All() Data {
	return val.data
}

func (val *Values) Set(k int32, v int64) {
	if val.dirty == nil {
		val.dirty = Data{}
	}
	val.dirty[k] = v
}

func (val *Values) Add(k int32, v int64) (r int64) {
	d := val.Val(k)
	r = d + v
	val.Set(k, r)
	return r
}
func (val *Values) Sub(k int32, v int64) (r int64) {
	d := val.Val(k)
	r = d - v
	val.Set(k, r)
	return r
}

func (val *Values) Unset(k int32) {
	if val.unset == nil {
		val.unset = make(map[int32]struct{})
	}
	val.unset[k] = struct{}{}
	if val.dirty != nil {
		delete(val.dirty, k)
	}
	val.data.Del(k)
}

func (val *Values) Save() (dirty Data, unsets []int32) {
	if len(val.dirty) > 0 {
		if val.data == nil {
			val.data = Data{}
		}
		dirty = make(Data, len(val.dirty))
		for k, v := range val.dirty {
			dirty[k] = v
			val.data[k] = v
		}
		val.dirty = nil
	}
	if len(val.unset) > 0 {
		unsets = make([]int32, 0, len(val.unset))
		for k := range val.unset {
			unsets = append(unsets, k)
		}
		val.unset = nil
	}
	return
}

func (val *Values) Release() {
	val.dirty = nil
	val.unset = nil
}

func (val *Values) Range(handle func(int32, int64) bool) {
	for k, v := range val.data {
		if !handle(k, v) {
			return
		}
	}
}
func (val *Values) Reset(data Data) {
	if data == nil {
		data = Data{}
	}
	val.data = data
}

// Receive 接收器，接收外部对象放入列表，不进行任何操作，一般用于初始化
func (val *Values) Receive(k int32, v int64) {
	if val.data == nil {
		val.data = Data{}
	}
	val.data[k] = v
}
