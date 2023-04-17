package test

import (
	"fmt"
	"github.com/hwcer/updater/v2"
	"github.com/hwcer/updater/v2/operator"
	"strconv"
	"strings"
	"sync/atomic"
)

var EquipIType = &equipIType{iType: iType{id: 40}}

type equipIType struct {
	iType
	seed int32
}

func (this *equipIType) New(u *updater.Updater, op *operator.Operator) (any, error) {
	v := updater.ParseInt64(op.Value)
	r := &Item{UID: u.Uid(), IID: op.IID, Val: v}
	r.OID, _ = this.CreateId(u, r.IID)
	fmt.Printf("New Item:%+v\n", r)
	return r, nil
}

// ObjectId 返回空字符串用来标识不可叠加
func (this *equipIType) ObjectId(a *updater.Updater, iid int32) (string, error) {
	return "", nil
}

// CreateId 创建道具唯一ID，注意要求可以使用itypes.go中ParseId函数解析
func (this *equipIType) CreateId(a *updater.Updater, iid int32) (string, error) {
	b := strings.Builder{}
	b.WriteString(a.Uid())
	b.WriteString(Split)
	b.WriteString(strconv.Itoa(int(iid)))
	b.WriteString(Split)
	b.WriteString(strconv.Itoa(int(atomic.AddInt32(&this.seed, 1))))
	return b.String(), nil
}
