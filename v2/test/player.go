package test

import (
	"github.com/hwcer/updater/v2"
)

type Player struct {
	*updater.Updater
	Role *Role
}

func NewPlayer(uid string) *Player {
	player := &Player{}
	player.Updater = updater.New(uid)
	player.Updater.Reset(nil)
	defer player.Updater.Release()
	player.Role = player.Updater.Handle("role").(*updater.Document).Interface().(*Role)
	return player
}
