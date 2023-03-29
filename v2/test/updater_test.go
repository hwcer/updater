package test

import (
	"encoding/json"
	"fmt"
	"github.com/hwcer/updater/v2"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	st := time.Now()
	player := updater.New(Userid)
	player.Reset(nil)
	//role 信息
	player.Add(1102, 100)
	player.Sub(1102, 20)
	player.Set(1102, 10)
	//player.Sub(1102, 20) //Item Not Enough：[1102 20 10]
	//日常数据
	player.Add(2001, 10)
	player.Max(2001, 100)
	player.Sub(2001, 20)
	player.Set(2002, 2000)
	player.Del(2002)
	//player.Sub(2002, 1) //Item Not Enough
	//常规道具
	player.Add(3001, 100)
	player.Add(3001, 10)
	player.Sub(3001, 20)
	player.Max(3001, 9999)
	player.Set(3001, "val", 1)
	player.Set(3001, "attach", "nbhh")
	//player.Sub(3001, 50)

	//添加装备
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
	fmt.Printf("GET Name:%v\n", role.Get("name"))
	player.Release()

	fmt.Printf("共计用时:%v\n", time.Since(st))

}
