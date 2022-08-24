package dataset

import (
	"errors"
	"fmt"
)

const (
	ModelNameOID = "_id"
	ModelNameIID = "iid"
	ModelNameVAL = "val"
	ModelNameUID = "uid"
)

func NewModel(uid string, iid int32) *Model {
	return &Model{IID: iid, Uid: uid}
}

type Model struct {
	OID string `bson:"_id" json:"_id"`
	IID int32  `bson:"iid" json:"iid"`
	Uid string `bson:"uid" json:"uid,omitempty"  index:"name:_idx_uid_bag,sort:ASC,priority:1" `
	Bag int32  `bson:"bag" json:"bag" index:"name:_idx_uid_bag,sort:ASC,priority:2;" `
	Val int64  `bson:"val" json:"val"`
}

func (this *Model) Copy() IModel {
	x := *this
	return &x
}

func (this *Model) Get(k string) (v any) {
	switch k {
	case "OID", ModelNameOID:
		v = this.OID
	case "IID", ModelNameIID:
		v = this.IID
	case "Uid", ModelNameUID:
		v = this.Uid
	case "Val", ModelNameVAL:
		v = this.Val
	}
	return
}

func (this *Model) Set(k string, v any) (r any, err error) {
	var ok bool
	switch k {
	case "OID", ModelNameOID:
		this.OID, ok = v.(string)
	case "IID", ModelNameIID:
		this.IID, ok = ParseInt32(v)
	case "Uid", ModelNameUID:
		this.Uid, ok = v.(string)
	case "Val", ModelNameVAL:
		this.Val, ok = ParseInt(v)
	default:
		err = fmt.Errorf("unknown field name:%v", k)
	}
	if !ok {
		err = errors.New("error in type")
	}
	r = v
	return
}

func (this *Model) Incr(k string, v int64) (r int64, err error) {
	switch k {
	case "Val", ModelNameVAL:
		this.Val += v
		r = this.Val
	default:
		err = fmt.Errorf("invalid field name:%v", k)
	}
	return
}
