package updater

import (
	"encoding/json"
	"fmt"
	"github.com/hwcer/cosmo"
	"strconv"
	"strings"
	"testing"
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

type iTypeTest struct {
	model     string
	stackable bool
}

func newiTypeiTypeTest(model string, stackable bool) *iTypeTest {
	return &iTypeTest{model: model, stackable: stackable}
}

func (this *iTypeTest) Model() string {
	return this.model
}
func (this *iTypeTest) Stackable() bool {
	return this.stackable
}

func (this *iTypeTest) CreateId(u *Updater, iid int32) (string, error) {
	if iid == 1100 {
		return "lv", nil
	} else if iid == 1101 {
		return fmt.Sprintf("%v-%v", u.Uid(), iid), nil
	} else {
		return fmt.Sprintf("%v-%v-%v", u.Uid(), iid, u.Time().Unix()), nil
	}
}

var iTypes = make(map[string]*iTypeTest)

func init() {
	db = cosmo.New()
	if err := db.Start("test", "mongodb://127.0.0.1:27017"); err != nil {
		fmt.Printf("%v", err)
	}
	_ = Register(ParseTypeHash, &Role{})
	_ = Register(ParseTypeTable, &Item{})

	iTypes["role"] = newiTypeiTypeTest("role", true)
	iTypes["item"] = newiTypeiTypeTest("item", true)
	iTypes["equip"] = newiTypeiTypeTest("item", false)

	Config.IMax = func(iid int32) int64 {
		return 0
	}

	Config.IType = func(iid int32) IType {
		if iid == 1100 {
			return iTypes["role"]
		} else if iid == 1101 {
			return iTypes["item"]
		} else {
			return iTypes["equip"]
		}
	}
	Config.ParseId = func(oid string) (iid int32, err error) {
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
}

func TestRegister(t *testing.T) {
	u := New()
	u.Reset("hwc")
	u.Add(1100, 1)
	u.Add(1101, 1)
	//u.Add(1102, 1)
	r, err := u.Save()
	if err != nil {
		t.Errorf("ERR:%v", err)
	} else {
		b, _ := json.Marshal(r)
		t.Logf("cache:%v", string(b))
	}
}

func BenchmarkNew(b *testing.B) {
	u := New()
	for i := 0; i < b.N; i++ {
		u.Reset("hwc")
		//u.Add(1100, 1)
		u.Add(1101, 1)
		u.Add(1101, 10)
		u.Add(1101, 100)
		//u.Add(1102, 1)
		_, _ = u.Save()
		u.Release()
	}

}
