package updater

import (
	"github.com/hwcer/logger"
)

type processCreator func(updater *Updater) any

var processDefault = map[string]processCreator{}

func RegisterGlobalProcess(name string, creator processCreator) {
	if processDefault[name] != nil {
		logger.Alert("player handle register already registered:%v", name)
	} else {
		processDefault[name] = creator
	}
}

type Process map[string]any

func (pro Process) Has(name string) bool {
	_, ok := pro[name]
	return ok
}

func (pro Process) Set(name string, value any) bool {
	if _, ok := pro[name]; ok {
		return false
	}
	pro[name] = value
	return true
}
func (pro Process) Get(name string) any {
	return pro[name]
}

func (pro Process) Delete(name string) {
	delete(pro, name)
}

func (pro Process) GetOrCreate(u *Updater, name string, f processCreator) any {
	i, ok := pro[name]
	if !ok {
		i = f(u)
		pro[name] = i
	}
	return i
}
