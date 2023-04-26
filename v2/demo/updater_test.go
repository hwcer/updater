package demo

import (
	"encoding/json"
	"fmt"
	"github.com/hwcer/updater"
	"github.com/hwcer/updater/demo/model"
	"github.com/hwcer/updater/operator"
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
	//for i := 0; i < 10; i++ {
	//	if err = service(userid, doTask); err != nil {
	//		fmt.Printf("服务器错误:%v\n", err)
	//	}
	//}

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
		var opts []*operator.Operator
		if opts, err = player.Submit(); err != nil {
			return
		}
		for _, c := range opts {
			if b, e := json.Marshal(c); e != nil {
				fmt.Printf("save error:%v\n-----------%+v\n", e, c)
			} else {
				fmt.Printf("save cache[%v]:%v\n", c.Type.ToString(), string(b))
			}

		}
		return
	})
}

func doWork(player *Player) error {
	st := time.Now()
	//player.Listen(updater.ProcessTypePreSave, ListenTest)
	//role 信息 doc模型
	player.Add(1002, 100)
	player.Sub(1002, 20)
	player.Set(1002, 10)
	role := player.Handle("role").(*updater.Document)
	role.Set("name", "test2")

	//日常,成就数据 hash模型
	player.Add(800001, 10)
	player.Max(800001, 100)
	player.Sub(800001, 20)
	player.Set(800002, 2000)
	player.Max(900001, 20000)
	//player.Del(2002)
	//player.Sub(2002, 1) //Item Not Enough
	//常规道具 coll模型
	player.Add(300001, 100)
	player.Add(300001, 10)
	player.Sub(300001, 20)
	player.Max(300001, 9999)
	//player.Set(300001, "val", 5)
	//player.Set(300001, "attach", "nbhh")
	//添加卡牌 coll模型
	player.Add(400001, 2)
	player.Add(400001, 1)
	//添加装备 coll模型
	player.Add(500001, 2)
	player.Add(500001, 1)
	//体力门票
	player.Sub(600001, 1)
	//定制道具
	item := &model.Item{}
	item.IID = 500001
	item.Uid = player.Uid()
	item.Val = 1
	item.OID, _ = model.ObjectId(player.Updater, item.IID, true)
	_ = item.SetAttach("定制版霜之哀伤")

	_ = player.New(item)

	player.Set(item.OID, "attach", "原版霜之哀伤")

	if _, err := player.Submit(); err != nil {
		return err //手动SAVE 强制立即生效
	}
	fmt.Printf("GET 1002:%v\n", player.Val(1002))
	fmt.Printf("GET Name:%v\n", player.Role.Name)
	fmt.Printf("共计用时:%v\n", time.Since(st))
	return nil
}

//func doTask(u *Player) error {
//	i := rand.Int31n(int32(len(TaskEventsDict)) - 1)
//	e := TaskEventsDict[i]
//	u.Emit(e, values.Values{"N": i})
//	return nil
//}
//
//func ListenTest(u *updater.Updater) error {
//	fmt.Printf("收到监听PreSave\n")
//	return nil
//}
