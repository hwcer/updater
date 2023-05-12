package demo

import (
	"github.com/hwcer/cosgo/scc"
	"github.com/hwcer/logger"
	"github.com/hwcer/updater/demo/model"
	"sync"
	"time"
)

const (
	playersHeartbeat  = 5    //心跳间隔(S)
	playersDisconnect = 30   //N秒无心跳,假死,视为掉线
	playersDestruct   = 3600 //掉线N秒进行销毁
)

var Players = &players{dict: sync.Map{}}

type players struct {
	dict sync.Map
	stop chan struct{}
}

func (this *players) Start() error {
	this.stop = make(chan struct{})
	scc.GO(this.daemon)
	return nil
}
func (this *players) Close() (err error) {
	select {
	case <-this.stop:
		return nil
	default:
		close(this.stop)
	}
	//关闭所有用户
	this.dict.Range(func(key, value any) bool {
		//检查掉线情况
		p := value.(*Player)
		if err = this.destruct(p); err != nil {
			return false
		}
		return true
	})
	return
}

// Get 获取玩家 ,注意返回NIL时,玩家未登录
func (this *players) Get(uid string, handle func(player *Player) error) error {
	var r *Player
	if v, ok := this.dict.Load(uid); ok {
		r = v.(*Player)
		if ok = r.Lock(); ok {
			r.Reset()
			defer func() {
				r.Release()
				r.Unlock()
			}()
		} else {
			r = nil
		}
	}
	return handle(r)
}

// Load 仅仅登录时使用
func (this *players) Load(uid string, handle func(player *Player) error) (err error) {
	var r *Player
	p := &Player{Role: &model.Role{}}
	p.mutex.Lock()
	defer p.mutex.Unlock()
	if i, loaded := this.dict.LoadOrStore(uid, p); loaded {
		np := i.(*Player)
		np.mutex.Lock()
		defer np.mutex.Unlock()
		r = np
	} else {
		r = p
	}
	//未初始化
	if r.Updater == nil {
		if err = r.init(uid); err != nil {
			return
		}
	}
	r.Reset()
	defer r.Release()
	//处理登录信息
	return handle(r)
}

// LoadWithUnlock 获取无锁状态的Player,慎用
func (this *players) LoadWithUnlock(uid string) (r *Player) {
	v, ok := this.dict.Load(uid)
	if ok {
		r = v.(*Player)
	}
	return
}

// Logout 强制下线,注意不能进用户锁
func (this *players) Logout(p *Player) {
	_ = this.destruct(p)
}

func (this *players) destruct(p *Player) (err error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	if err = p.Destroy(); err == nil {
		this.dict.Delete(p.Uid())
	}
	return
}

func (this *players) disconnect(p *Player, t int64) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	//------------------------------------
	p.Reset()
	_, _ = p.Submit()
	p.Release()
	//------------------------------------
	p.connected = 0
	p.KeepAlive(t)
	//尝试回写数据
	_ = p.Destroy()
}

func (this *players) worker() {
	defer func() {
		if e := recover(); e != nil {
			logger.Debug(e)
		}
	}()
	now := time.Now().Unix()
	logoutTime := now - playersDisconnect
	destructTime := now - playersDestruct

	this.dict.Range(func(key, value any) bool {
		//检查掉线情况
		p := value.(*Player)
		if p.connected == 0 && p.heartbeat <= destructTime {
			_ = this.destruct(p)
		} else if p.heartbeat < logoutTime {
			this.disconnect(p, now)
		}
		return true
	})
}

func (this *players) daemon() {
	t := time.Second * playersHeartbeat
	timer := time.NewTimer(t)
	for {
		select {
		case <-this.stop:
			return
		case <-timer.C:
			this.worker()
			timer.Reset(t)
		}
	}
}
