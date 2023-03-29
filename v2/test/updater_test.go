package test

import (
	"encoding/json"
	"fmt"
	"github.com/hwcer/updater/v2"
	"testing"
	"time"
)

const (
	eventTest updater.EventType = 1
)

func TestNew(t *testing.T) {
	player := NewPlayer(Userid)
	player.On(eventTest, listenerTest)
	doWork(player)
	for i := 0; i < 10; i++ {
		doEvent(player)
	}
}

func doWork(player *Player) {
	st := time.Now()
	player.Reset(nil)
	defer player.Release()
	//role 信息 doc模型
	player.Add(1102, 100)
	player.Sub(1102, 20)
	player.Set(1102, 10)
	//player.Sub(1102, 20) //Item Not Enough：[1102 20 10]
	//日常数据 hash模型
	player.Add(2001, 10)
	player.Max(2001, 100)
	player.Sub(2001, 20)
	player.Set(2002, 2000)
	player.Del(2002)
	//player.Sub(2002, 1) //Item Not Enough
	//常规道具 coll模型
	player.Add(3001, 100)
	player.Add(3001, 10)
	player.Sub(3001, 20)
	player.Max(3001, 9999)
	player.Set(3001, "val", 5)
	player.Set(3001, "attach", "nbhh")

	//添加装备 coll模型
	player.Add(4001, 2)
	player.Add(4001, 1)

	role := player.Handle("role").(*updater.Document)
	role.Set("name", "test2")

	if err := player.Save(); err != nil {
		fmt.Printf("save error:%v\n", err)
	} else {
		for _, c := range player.Submit() {
			b, _ := json.Marshal(c)
			fmt.Printf("save cache[%v]:%v\n", c.TYP.ToString(), string(b))
		}
	}
	fmt.Printf("GET 1102:%v\n", player.Val(1102))
	fmt.Printf("GET Name:%v\n", player.Role.Name)
	fmt.Printf("共计用时:%v\n", time.Since(st))
}

func doEvent(u *Player) {
	u.Reset(nil)
	defer func() {
		u.Submit()
		u.Release()
	}()

	v := u.Val(3001)
	fmt.Printf("当前道具数量:%v\n", v)
	if v > 0 {
		u.Sub(3001, 1)
	}
	_ = u.Save()
	u.Emit(eventTest, updater.EventArgs{"N": v})
}

func listenerTest(u *updater.Updater, args updater.EventArgs) bool {
	fmt.Printf("收到事件:%v\n", args)
	if args["N"].(int64) <= 2 {
		return false
	}
	return true
}
