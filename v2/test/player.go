package test

import (
	"github.com/hwcer/updater/v2"
	"sync"
)

func NewPlayer(uid string) *Player {
	player := &Player{}
	player.Updater = updater.New(uid)
	return player
}

type Player struct {
	*updater.Updater
	Role      *Role
	Task      *TaskMgr
	mutex     sync.Mutex
	connected int64 //上线时间
	heartbeat int64 //最后心跳时间
}

func (p *Player) construct() {
	p.Role = p.Updater.Handle("role").(*updater.Document).Interface().(*Role)
	p.Task = &TaskMgr{}
	p.Task.Init(p.Updater)
}

func (p *Player) Lock() bool {
	return p.mutex.TryLock()
}

func (p *Player) Unlock() {
	p.mutex.Unlock()
}

// KeepAlive 保持在线
func (this *Player) KeepAlive(t int64) {
	this.heartbeat = t
}
