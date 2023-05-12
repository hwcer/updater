package demo

import (
	"github.com/hwcer/updater"
	"github.com/hwcer/updater/demo/model"
	"sync"
)

func NewPlayer(uid string) (player *Player, err error) {
	player = &Player{}
	player.Role = &model.Role{}
	err = player.init(uid)
	return
}

type Player struct {
	*updater.Updater
	Role      *model.Role
	mutex     sync.Mutex
	connected int64 //上线时间
	heartbeat int64 //最后心跳时间
}

func (p *Player) init(uid string) error {
	if u, err := updater.New(uid); err == nil {
		p.Updater = u
	} else {
		return err
	}
	return nil
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
