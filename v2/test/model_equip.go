package test

import (
	"fmt"
	"github.com/hwcer/updater/v2"
	"github.com/hwcer/updater/v2/dirty"
)

var ITypeEquip = &iTypeEquip{iType{id: 40, unique: false}}

type iTypeEquip struct {
	iType
}

func (this *iTypeEquip) New(u *updater.Updater, cache *dirty.Cache) (any, error) {
	r := &Item{UID: u.Uid(), IID: cache.IID, VAL: 1}
	r.OID, _ = this.CreateId(u, r.IID)
	fmt.Printf("New equip:%+v\n", r)
	return r, nil
}
