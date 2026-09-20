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
	//saveFailed 上次落库失败且脏标记已回填(Restore置位):脏标记是下次同步的
	//重试数据源,Release不得清空;下次Save消费脏标记时解除。对齐 Collection 同名语义
	saveFailed bool
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
	if val.unset != nil {
		delete(val.unset, k) //同 key 先 Unset 后 Set:最终语义是 Set,清掉 unset 标记,避免 Save 同时产出 $set+$unset
	}
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
	val.saveFailed = false //本轮重新出发:若Setter随后失败,Restore会重新置位
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
	if val.saveFailed {
		return //🔴 落库失败的脏标记是重试数据源,不得清空——旧实现无条件清空,
		//"失败等待下次同步"在同一请求的 release 里就没了数据,改动静默丢失
	}
	val.dirty = nil
	val.unset = nil
}

// restore 持久化失败时恢复脏标记,使下次Save能重新生成更新载荷
// dirty/unsets为Save刚返回的载荷,直接回填幂等;仅回填脏标记,val.data在Save时已更新
func (val *Values) Restore(dirty Data, unsets []int32) {
	val.saveFailed = true
	if len(dirty) > 0 {
		if val.dirty == nil {
			val.dirty = make(Data, len(dirty))
		}
		for k, v := range dirty {
			val.dirty[k] = v
		}
	}
	if len(unsets) > 0 {
		if val.unset == nil {
			val.unset = make(map[int32]struct{}, len(unsets))
		}
		for _, k := range unsets {
			val.unset[k] = struct{}{}
		}
	}
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
