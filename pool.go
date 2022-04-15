package updater

import (
	"sync"
)

func NewPool() *Pool {
	i := &Pool{pool: sync.Pool{}}
	i.pool.New = func() (u interface{}) {
		return New()
	}
	return i
}

type Pool struct {
	pool sync.Pool
}

func (s *Pool) Acquire(uid string) (u *Updater) {
	u = s.pool.Get().(*Updater)
	u.Reset(uid)
	return u
}

func (s *Pool) Release(u *Updater) {
	u.Release()
	s.pool.Put(u)
}
