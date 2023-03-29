package test

import (
	"errors"
	"fmt"
	"github.com/hwcer/updater/v2"
	"github.com/hwcer/updater/v2/operator"
	"strconv"
	"strings"
	"sync/atomic"
)

const Split = "-"
const Userid = "test"

func init() {
	updater.Config.IMax = func(iid int32) int64 {
		return 0
	}
	updater.Config.IType = func(iid int32) int32 {
		s := strconv.Itoa(int(iid))
		v, _ := strconv.ParseInt(s[0:2], 10, 32)
		return int32(v)
	}
	updater.Config.ParseId = ParseId
}

// ParseId  oid TO iid
func ParseId(_ *updater.Updater, oid string) (iid int32, err error) {
	arr := strings.Split(oid, Split)
	if len(arr) < 2 {
		return 0, fmt.Errorf("oid错误:%v", oid)
	}
	var v int
	if v, err = strconv.Atoi(arr[1]); err == nil {
		iid = int32(v)
	}
	return
}

type iType struct {
	id     int32
	seed   int32
	unique bool
}

func (this *iType) Id() int32 {
	return this.id
}

func (this *iType) New(_ *updater.Updater, op *operator.Operator) (any, error) {
	return nil, errors.New("没有重载New方法，理论上不应该调用此方法，请检查代码")
}

func (this *iType) Unique() bool {
	return this.unique
}

// CreateId 创建道具唯一ID，注意要求可以使用itypes.go中ParseId函数解析
func (this *iType) CreateId(a *updater.Updater, iid int32) (string, error) {
	b := strings.Builder{}
	b.WriteString(a.Uid())
	b.WriteString(Split)
	b.WriteString(strconv.Itoa(int(iid)))
	if !this.Unique() {
		b.WriteString(Split)
		b.WriteString(strconv.Itoa(int(atomic.AddInt32(&this.seed, 1))))
	}
	return b.String(), nil
}
