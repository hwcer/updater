package updater

import (
	"context"
	"sync"
)

var Pool = pool{}

type pool struct {
	sync.Pool
}

func init() {
	Pool.Pool.New = func() any {
		return New()
	}
}

func (p *pool) Acquire(uid string, ctx context.Context) *Updater {
	i := p.Pool.Get()
	v, _ := i.(*Updater)
	v.Reset(uid, ctx)
	return v
}

func (p *pool) Release(v *Updater) {
	v.Release()
	p.Pool.Put(v)
}