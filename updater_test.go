package updater

import (
	"encoding/json"
	"fmt"
	"github.com/hwcer/cosmo"
	"github.com/hwcer/updater/models"
	"strconv"
	"strings"
	"testing"
	"time"
)

type Role struct {
	Id     string `bson:"_id"`
	Lv     int32
	Name   string
	Create int64
}

type Item struct {
	OID string `bson:"_id" json:"_id"`
	IID int32  `bson:"id" json:"id"`
	Val int64  `json:"val" bson:"val"`
	Uid string `bson:"uid" json:"uid" gorm:"index:"`
}

func (this *Role) SetOnInert(uid string, now time.Time) map[string]interface{} {
	r := make(map[string]interface{})
	r["_id"] = uid
	r["create"] = now.Unix()
	return r
}

var iTypes = make(map[string]*IType)

func init() {
	db = cosmo.New()
	if err := db.Start("test", "mongodb://127.0.0.1:27017"); err != nil {
		fmt.Printf("%v", err)
	}
	_ = Register(models.ParseTypeHash, &Role{})
	_ = Register(models.ParseTypeTable, &Item{})

	iTypes["role"] = &IType{Model: "role"}
	iTypes["item"] = &IType{Model: "item", Unique: true}
	iTypes["equip"] = &IType{Model: "item", Unique: false}

	Config.IMax = func(iid int32) int64 {
		return 0
	}

	Config.IType = func(iid int32) *IType {
		if iid == 1100 {
			return iTypes["role"]
		} else if iid == 1101 {
			return iTypes["item"]
		} else {
			return iTypes["equip"]
		}
	}
	Config.Field = func(iid int32) (key string) {
		if iid == 1100 {
			return "lv"
		}
		return ""
	}

	ObjectID.Parse = func(oid string) (iid int32, err error) {
		arr := strings.Split(oid, "-")
		if len(arr) < 2 {
			return
		}
		var v int
		v, err = strconv.Atoi(arr[1])
		if err == nil {
			iid = int32(v)
		}
		return
	}
	ObjectID.Create = func(u *Updater, iid int32, unique bool) (string, error) {
		if iid == 0 {
			return u.Uid(), nil
		} else if unique {
			return fmt.Sprintf("%v-%v", u.Uid(), iid), nil
		} else {

			return fmt.Sprintf("%v-%v-%v", u.Uid(), iid, u.Time().Unix()), nil
		}
	}
}

func TestRegister(t *testing.T) {
	u := New()
	u.Reset("hwc")
	u.Add(1100, 1)
	u.Add(1101, 1)
	u.Add(1102, 1)
	r, err := u.Save()
	if err != nil {
		t.Errorf("ERR:%v", err)
	} else {
		b, _ := json.Marshal(r)
		t.Logf("cache:%v", string(b))
	}
}
