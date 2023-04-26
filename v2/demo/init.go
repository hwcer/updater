package demo

import (
	"github.com/hwcer/updater"
	"github.com/hwcer/updater/demo/config"
	"github.com/hwcer/updater/demo/model"
)

func init() {
	updater.Config.IMax = config.IMax
	updater.Config.IType = config.IType
	updater.Config.ParseId = model.ParseId
	_ = model.Start()
}
