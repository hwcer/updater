package test

import (
	"encoding/json"
	"fmt"
	"github.com/hwcer/cosgo/values"
	"github.com/hwcer/updater/v2"
	"math/rand"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	_ = Players.Start()
	userid := "userid"
	//LOGIN
	err := Players.Load(userid, func(player *Player) (err error) {
		if player == nil {
			err = fmt.Errorf("用户不存在:%v", userid)
		}
		return
	})
	if err != nil {
		fmt.Printf("登录失败:%v\n", err)
		return
	}

	if err = service(userid, doWork); err != nil {
		fmt.Printf("服务器错误:%v\n", err)
	}
	for i := 0; i < 10; i++ {
		if err = service(userid, doTask); err != nil {
			fmt.Printf("服务器错误:%v\n", err)
		}
	}

	if err = Players.Close(); err != nil {
		fmt.Printf("关闭服务器错误:%v\n", err)
	}
}

// 模拟服务总入口
func service(uid string, f func(*Player) error) error {
	return Players.Get(uid, func(player *Player) (err error) {
		if player != nil {
			err = f(player)
		} else {
			err = fmt.Errorf("用户没有登录:%v", uid)
		}
		if err != nil {
			return
		}
		if err = player.Save(); err != nil {
			return
		}
		for _, c := range player.Submit() {
			b, _ := json.Marshal(c)
			fmt.Printf("save cache[%v]:%v\n", c.Type.ToString(), string(b))
		}
		return
	})
}

func doWork(player *Player) error {
	st := time.Now()
	player.Listen(updater.ProcessTypePreSave, ListenTest)
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

	player.Set("userid-4001-1", "attach", "霜之哀伤")

	role := player.Handle("role").(*updater.Document)
	role.Set("name", "test2")
	if err := player.Save(); err != nil {
		return err //手动SAVE 强制立即生效
	}
	fmt.Printf("GET 1102:%v\n", player.Val(1102))
	fmt.Printf("GET Name:%v\n", player.Role.Name)
	fmt.Printf("共计用时:%v\n", time.Since(st))
	return nil
}

func doTask(u *Player) error {
	i := rand.Int31n(int32(len(TaskEventsDict)) - 1)
	e := TaskEventsDict[i]
	u.Emit(e, values.Values{"N": i})
	return nil
}

func ListenTest(u *updater.Updater) error {
	fmt.Printf("收到监听PreSave\n")
	return nil
}
