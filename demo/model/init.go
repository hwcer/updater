package model

import (
	"fmt"
	"github.com/hwcer/updater"
	"strconv"
	"strings"
	"sync/atomic"
)

func NewModel(uid string, iid int32) *Model {
	return &Model{IID: iid, Uid: uid}
}

type Model struct {
	OID string `bson:"_id" json:"_id"`
	IID int32  `bson:"iid" json:"iid"`
	Uid string `bson:"uid" json:"uid,omitempty"  index:"" `
	//Bag int32  `bson:"bag" json:"bag" index:"name:_idx_uid_bag,sort:ASC,priority:2;" `
	Val int64 `bson:"val" json:"val"`
}

func (this *Model) Clone() *Model {
	x := *this
	return &x
}

func (this *Model) Get(k string) any {
	switch k {
	case "_id", "OID":
		return this.OID
	case "uid", "UID":
		return this.Uid
	case "iid", "IID":
		return this.IID
	case "val", "Val":
		return this.Val
	default:
		return nil
	}
}
func (this *Model) Set(k string, v any) (err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("%v", e)
		}
	}()
	switch k {
	case "_id", "OID":
		this.OID = v.(string)
	case "uid", "UID":
		this.Uid = v.(string)
	case "iid", "IID":
		this.IID = v.(int32)
	case "val", "Val":
		this.Val = v.(int64)
	default:
		err = fmt.Errorf("field not exist:%v", k)
	}
	return
}

func (this *Model) SetOnInsert() (r map[string]interface{}, err error) {
	r = make(map[string]any)
	r["uid"] = this.Uid
	r["iid"] = this.IID
	return
}

const objectSplit = "-"

func ParseId(_ *updater.Updater, oid string) (iid int32, err error) {
	arr := strings.Split(oid, objectSplit)
	if len(arr) < 2 {
		return 0, fmt.Errorf("oid错误:%v", oid)
	}
	var v int
	if v, err = strconv.Atoi(arr[1]); err == nil {
		iid = int32(v)
	}
	return
}

var objectIdSuffix int32

func ObjectId(u *updater.Updater, iid int32, suffix bool) (string, error) {
	b := strings.Builder{}
	b.WriteString(u.Uid())
	b.WriteString(objectSplit)
	b.WriteString(strconv.Itoa(int(iid)))
	if suffix {
		b.WriteString(objectSplit)
		s := atomic.AddInt32(&objectIdSuffix, 1)
		b.WriteString(strconv.Itoa(int(s)))
	}
	return b.String(), nil
}
