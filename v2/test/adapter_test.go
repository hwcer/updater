package test

import (
	"encoding/json"
	"github.com/hwcer/adapter"
	"github.com/hwcer/logger"
	"testing"
)

func TestNew(t *testing.T) {
	p := Acquire("usr")
	p.Add(1102, 100)
	p.Sub(1102, 20)

	p.Add(2001, 12)
	p.Max(2002, 100)

	p.Add(3001, 100)
	p.Sub(3001, 50)

	role := p.Handle("role").(*updater.Document)
	role.Set("name", "test2")

	if err := p.Save(); err != nil {
		logger.Info("save error:%v", err)
	} else {
		for _, c := range p.Cache() {
			b, _ := json.Marshal(c)
			logger.Info("save cache:%v", string(b))
		}
	}

	logger.Info("GET 1102:%v", p.Val(1102))
	logger.Info("GET Name:%v", role.GetString("name"))
	Release(p)
}
