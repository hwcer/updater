package test

import (
	"fmt"
	"github.com/hwcer/updater/v2"
	"strconv"
	"strings"
)

const Split = "-"

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
	id int32
}

func (this *iType) Id() int32 {
	return this.id
}
