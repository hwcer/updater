package updater

import (
	"fmt"
	"github.com/hwcer/cosgo/logger"
	"github.com/hwcer/cosmo"
	"github.com/hwcer/cosmo/utils"
	"go.mongodb.org/mongo-driver/bson"
)

/*
Table 适用列表
*/

type Table struct {
	*base
	dataset   *Dataset
	bulkWrite *cosmo.BulkWrite
}

func NewTable(model *Model, updater *Updater) *Table {
	_ = model.Model.(ModelTable)
	b := NewBase(model, updater)
	i := &Table{base: b, dataset: NewDataset(model.Schema)}
	i.release()
	return i
}

func (this *Table) release() {
	this.bulkWrite = nil
	this.base.release()
	this.dataset.release()
}

func (this *Table) Sub(k int32, v int32) {
	this.addAct(ActTypeSub, k, v)
}

func (this *Table) Max(k int32, v int32) {
	this.addAct(ActTypeMax, k, v)
}

func (this *Table) Min(k int32, v int32) {
	this.addAct(ActTypeMin, k, v)
}

func (this *Table) Add(k int32, v int32) {
	if k == 0 || v <= 0 {
		return
	}
	it := Config.IType(k)
	if it == nil {
		logger.Debug("IType unknown:%v", k)
		return
	}
	var act *Cache
	if it.Stackable() {
		oid, err := it.CreateId(this.base.updater, k)
		if err != nil {
			logger.Debug("hmap NewId error:%v", err)
			return
		}
		act = &Cache{OID: oid, IID: k, AType: ActTypeAdd, Key: ItemNameVAL, Val: v}
	} else {
		act = &Cache{OID: "", IID: k, AType: ActTypeNew, Key: "*", Val: v}
	}
	act.IType = it
	this.act(act)
	//可能需要分解
	if resolve, ok := it.(ITypeResolve); ok {
		if newId, newNum, ok2 := resolve.Resolve(k, v); ok2 && newNum > 0 {
			this.updater.Keys(newId)
		}
	}
}

//Val  oid --堆数量,iid所有数据(不可堆叠)
func (this *Table) Val(id interface{}) (r int64) {
	switch id.(type) {
	case string:
		r = this.dataset.Val(id.(string))
	default:
		if iid, ok := ParseInt32(id); ok && iid > 0 {
			r = this.dataset.Count(iid)
		}
	}
	return
}

//Get 返回道具对象
func (this *Table) Get(id interface{}) (interface{}, bool) {
	_, oid, _, err := this.ParseId(id)
	if err != nil {
		logger.Error(err)
		return nil, false
	}
	return this.dataset.Data(oid)
}

//Set id= iid||oid ,v=map[string]interface{} || bson.M
//v 非Map对象时，一律转换为Map{"val":v}
func (this *Table) Set(id interface{}, v interface{}) {
	iid, oid, it, err := this.ParseId(id)
	if err != nil {
		logger.Error(err)
		return
	}
	var k string
	switch v.(type) {
	case map[string]interface{}, bson.M:
		k = "*"
	default:
		k = ItemNameVAL
	}
	act := &Cache{OID: oid, IID: iid, AType: ActTypeSet, Key: k, Val: v}
	act.IType = it
	this.act(act)
}

func (this *Table) Del(id interface{}) {
	iid, oid, it, err := this.ParseId(id)
	if err != nil {
		logger.Error(err)
		return
	}
	act := &Cache{OID: oid, IID: iid, AType: ActTypeDel, Key: "*", Val: 0}
	act.IType = it
	this.act(act)
}

func (this *Table) Keys(ids ...int32) {
	for _, id := range ids {
		_, oid, _, err := this.ParseId(id)
		if err == nil {
			this.Select(oid)
		}
	}
}

func (this *Table) act(act *Cache) {
	if act.IType == nil {
		act.IType = Config.IType(act.IID)
	}
	if act.IType == nil {
		logger.Error("IType Not Exist :%v", act.IID)
		return
	}
	if onChange, ok := act.IType.(ITypeOnChange); ok {
		if !onChange.OnChange(this.updater, act) {
			return
		}
	}
	if act.AType != ActTypeDel && act.OID != "" {
		this.base.Select(act.OID)
	}
	if this.bulkWrite == nil {
		this.base.Act(act)
	} else {
		_ = this.doAct(act)
	}
}

func (this *Table) addAct(t ActType, k int32, v int32) {
	if k == 0 {
		return
	}
	it := Config.IType(k)
	if it == nil {
		logger.Error("ParseId IType unknown:%v", k)
		return
	}
	if !it.Stackable() {
		logger.Error("不可叠加道具只能使用OID进行Del操作:%v", k)
	}

	oid, err := it.CreateId(this.base.updater, k)
	if err != nil {
		logger.Error("updater.Table CreateId error:%v", err)
		return
	}
	act := &Cache{OID: oid, IID: k, AType: t, Key: "val", Val: v}
	act.IType = it
	this.act(act)
}

func (this *Table) Data() error {
	query := this.base.fields.Query(this.updater.uid)
	if query == nil || len(query) == 0 {
		return nil
	}
	rows := this.model.Model.(ModelTable).MakeSlice()
	tx := db.Find(rows, query)
	if tx.Error != nil {
		return tx.Error
	} else if tx.RowsAffected == 0 {
		return nil
	}
	for _, v := range utils.ToArray(rows) {
		this.dataset.Set(v)
	}
	this.base.fields.reset()
	return nil
}

func (this *Table) Verify() (err error) {
	defer func() {
		this.base.acts = nil
	}()
	_ = this.BulkWrite()
	if len(this.base.acts) == 0 {
		return nil
	}
	for _, act := range this.base.acts {
		if err = this.doAct(act); err != nil {
			return
		}
	}
	return nil
}

func (this *Table) Save() (cache []*Cache, err error) {
	if this.base.errMsg != nil {
		return nil, this.base.errMsg
	}
	if this.bulkWrite == nil {
		return
	}
	_, err = this.bulkWrite.Save()
	if err == nil {
		cache = this.cache
		this.cache = nil
	}
	return
}

func (this *Table) BulkWrite() *cosmo.BulkWrite {
	if this.bulkWrite == nil {
		this.bulkWrite = db.BulkWrite(this.model.Model)
	}
	return this.bulkWrite
}

func (this *Table) doAct(act *Cache) (err error) {
	defer func() {
		if err == nil {
			this.base.cache = append(this.base.cache, act)
		} else {
			this.bulkWrite = nil
			this.base.errMsg = err
		}
	}()
	if this.updater.strict && act.AType == ActTypeSub {
		av, _ := ParseInt(act.Val)
		dv := this.Val(act.OID)
		if dv < av {
			return ErrItemNotEnough(act.IID, av, dv)
		}
	}
	it := act.IType
	//溢出判定
	if act.AType == ActTypeAdd || act.AType == ActTypeNew {
		v, ok := ParseInt(act.Val)
		if !ok || v <= 0 {
			return ErrActValIllegal(act)
		}
		d := this.dataset.Count(act.IID)
		t := v + d
		imax := Config.IMax(act.IID)

		if imax > 0 && t > imax {
			overflow := t - imax
			if overflow > v {
				overflow = v
			}
			v = v - overflow
			act.Val = v
			if resolve, ok1 := it.(ITypeResolve); ok1 {
				if newId, NewNum, ok2 := resolve.Resolve(act.IID, int32(overflow)); ok2 {
					overflow = 0
					this.updater.Add(newId, int32(NewNum))
				}
			}
			if overflow > 0 {
				this.updater.overflow[act.IID] += overflow
			}
		}
		if v == 0 {
			act.AType = ActTypeResolve
		}
	}

	if it.Stackable() {
		return parseHMap(this, act)
	} else {
		return parseTable(this, act)
	}
}

//ParseId 解析道具，不可叠加道具不能使用iid解析
// it 可能为空(用不到)
func (this *Table) ParseId(id interface{}) (iid int32, oid string, it IType, err error) {
	switch id.(type) {
	case string:
		oid = id.(string)
		iid, err = Config.ParseId(oid)
	default:
		if iid, _ = ParseInt32(id); iid > 0 {
			it = Config.IType(iid)
			if it == nil {
				err = fmt.Errorf("ParseId IType unknown:%v", id)
			} else if !it.Stackable() {
				err = fmt.Errorf("不可叠加道具不能使用IID进行操作:%v", id)
			} else {
				oid, err = it.CreateId(this.base.updater, iid)
			}
		} else {
			err = fmt.Errorf("ParseId args illegal:%v", id)
		}
	}
	return
}
