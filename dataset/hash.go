package dataset

func NewHash(data map[int32]int64, expire int64) *Hash {
	return &Hash{data: data, expire: expire}
}

type Hash struct {
	data   map[int32]int64
	dirty  map[int32]int64
	expire int64
}

func (h *Hash) Has(k int32) (r bool) {
	if h.dirty != nil {
		if _, r = h.dirty[k]; r {
			return
		}
	}
	_, r = h.data[k]
	return
}
func (h *Hash) Val(k int32) (r int64) {
	r, _ = h.Get(k)
	return
}

func (h *Hash) Get(k int32) (r int64, ok bool) {
	if h.dirty != nil {
		if r, ok = h.dirty[k]; ok {
			return
		}
	}
	r, ok = h.data[k]
	return
}

func (h *Hash) Set(k int32, v int64) {
	if h.dirty == nil {
		h.dirty = make(map[int32]int64)
	}
	h.dirty[k] = v
}

func (h *Hash) Add(k int32, v int64) (r int64) {
	d := h.Val(k)
	r = d + v
	h.Set(k, r)
	return r
}
func (h *Hash) Sub(k int32, v int64) (r int64) {
	d := h.Val(k)
	r = d - v
	h.Set(k, r)
	return r
}

func (h *Hash) Save() (r map[int32]int64, expire int64) {
	r = h.dirty
	expire = h.expire
	for k, v := range h.dirty {
		h.data[k] = v
	}
	h.dirty = nil
	return
}

func (h *Hash) Reset(data map[int32]int64, expire int64) {
	h.data = data
	h.dirty = nil
	h.expire = expire
}
