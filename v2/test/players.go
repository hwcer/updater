package test

import (
	"context"
	"github.com/hwcer/cosgo"
	"github.com/hwcer/cosgo/logger"
	"sync"
	"time"
)

const (
	playersHeartbeat  = 5    //心跳间隔(S)
	playersOnlineSave = 300  //每N秒结算一次在线时间
	playersDisconnect = 30   //N秒无心跳,假死,视为掉线
	playersDestruct   = 3600 //掉线N秒进行销毁
)

var Players = &players{dict: sync.Map{}}

type players struct {
	dict sync.Map
}

func (this *players) Start() error {
	cosgo.CGO(this.daemon)
	return nil
}
func (this *players) Close() error {
	return nil
}

// Get 获取玩家 ,注意返回NIL时,玩家未登录
func (this *players) Get(uid string, handle func(player *Player) error) error {
	var r *Player
	if v, ok := this.dict.Load(uid); ok {
		r = v.(*Player)
		if ok = r.Lock(); ok {
			r.Reset(nil)
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
	p := NewPlayer(uid)
	p.mutex.Lock()
	defer p.mutex.Unlock()
	if i, loaded := this.dict.LoadOrStore(uid, p); loaded {
		np := i.(*Player)
		np.mutex.Lock()
		defer np.mutex.Unlock()
		r = np
	} else if err = p.Construct(); err == nil {
		p.construct()
		r = p
	}
	if err != nil {
		return
	}
	r.Reset(nil)
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

//// Online 在线人数
//func (this *players) Online() int32 {
//	return this.online
//}
//
//// Login 用户上线.必须所内执行
//func (this *players) Login(p *Updater, now int64, log *IGGLog) {
//	if log != nil {
//		p.Values.Set(ValuesIGGLog, log)
//	}
//	this.connect(p, now, false)
//}

// Logout 强制下线
func (this *players) Logout(p *Player) {
	this.disconnect(p, false)
}

func (this *players) disconnect(p *Player, needLock bool) {
	if needLock {
		p.mutex.Lock()
		defer p.mutex.Unlock()
	}
	this.dict.Delete(p.Uid())
}

func (this *players) worker() {
	defer func() {
		if e := recover(); e != nil {
			logger.Debug(e)
		}
	}()
	//now := time.Now().Unix()
	//logoutTime := now - playersDisconnect
	//destroyTime := now - playersDestruct

	this.dict.Range(func(key, value any) bool {
		//检查掉线情况
		//p := value.(*Player)
		//if p.Online.timestamp == 0 && p.Online.heartbeat <= destroyTime {
		//	this.destroyed(p, true)
		//} else if p.Online.heartbeat < logoutTime {
		//	this.disconnect(p, now, true)
		//}
		//else if p.Online.timestamp < saveTime {
		//	this.refresh(p, now)
		//}
		return true
	})
}

func (this *players) daemon(ctx context.Context) {
	t := time.Second * playersHeartbeat
	timer := time.NewTimer(t)
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			this.worker()
			timer.Reset(t)
		}
	}
}
