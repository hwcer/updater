package events

type Handle func(l *Listener, val int32) bool //满足条件后的更新器,返回false移除监听

type Listener struct {
	key    any     //任务标识
	args   []int32 //任务匹配参数
	handle Handle  //回调信息
	Filter Filter  //过滤函数
}

func New(k any, args []int32, handle Handle) *Listener {
	return &Listener{key: k, args: args, handle: handle}
}

func (l *Listener) GetKey() any {
	return l.key
}

func (l *Listener) GetArgs() (r []int32) {
	if n := len(l.args); n > 0 {
		r = make([]int32, n)
		copy(r, l.args)
	}
	return
}

func (l *Listener) Handle(t int32, v int32, args []int32) bool {
	if !l.compare(t, args) {
		return true
	}
	return l.handle(l, v)
}

func (l *Listener) compare(t int32, args []int32) bool {
	if len(l.args) == 0 {
		return true
	}
	if l.Filter != nil {
		return l.Filter(l.args, args)
	}
	f := Require(t)
	return f(l.args, args)
}
