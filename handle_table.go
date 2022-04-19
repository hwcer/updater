package updater

import (
	"fmt"
	"github.com/hwcer/cosgo/library/logger"
	"github.com/hwcer/cosmo"
	"github.com/hwcer/cosmo/utils"
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

func (this *Table) Add(k int32, v int32) {
	it := Config.IType(k)
	if it == nil {
		logger.Error("IType unknown:%v", k)
		return
	}
	var act *Cache
	if it.Unique() {
		oid, err := this.CreateId(k)
		if err != nil {
			logger.Error("hmap NewId error:%v", err)
			return
		}
		act = &Cache{OID: oid, IID: k, AType: ActTypeAdd, Key: ItemNameVAL, Val: v}
	} else {
		act = &Cache{OID: "", IID: k, AType: ActTypeNew, Key: "*", Val: v}
	}
	this.act(act)
	if onChange, ok := it.(ITypeOnChange); ok {
		onChange.OnChange(this.updater, k, v)
	}
}

func (this *Table) Sub(k int32, v int32) {
	it := Config.IType(k)
	if it == nil {
		logger.Error("ParseId IType unknown:%v", k)
		return
	}

	if !it.Unique() {
		logger.Error("不可叠加道具只能使用OID进行Del操作:%v", k)
	}

	oid, err := this.CreateId(k)
	if err != nil {
		logger.Error("hmap NewId error:%v", err)
		return
	}
	act := &Cache{OID: oid, IID: k, AType: ActTypeSub, Key: "val", Val: v}
	this.act(act)
	if onChange, ok := it.(ITypeOnChange); ok {
		onChange.OnChange(this.updater, k, -v)
	}
}

//Set id= iid||oid ,v=map[string]interface{}
func (this *Table) Set(id interface{}, v interface{}) {
	val, ok := v.(map[string]interface{})
	if !ok {
		logger.Error("Table set v error")
		return
	}

	iid, oid, err := this.ParseId(id)
	if err != nil {
		logger.Error(err)
		return
	}

	act := &Cache{OID: oid, IID: iid, AType: ActTypeSet, Key: "*", Val: val}
	this.act(act)
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
	_, oid, err := this.ParseId(id)
	if err != nil {
		logger.Error(err)
		return nil, false
	}
	return this.dataset.Data(oid)
}

func (this *Table) Del(id interface{}) {
	iid, oid, err := this.ParseId(id)
	if err != nil {
		logger.Error(err)
		return
	}
	act := &Cache{OID: oid, IID: iid, AType: ActTypeDel, Key: "*", Val: 0}
	this.act(act)
}

func (this *Table) act(act *Cache) {
	if act.AType != ActTypeDel {
		if act.OID != "" {
			this.Keys(act.OID)
		}
	}
	this.base.Act(act)
	if this.bulkWrite != nil {
		_ = this.Verify()
	}
}

func (this *Table) Data() error {
	query := this.base.fields.Query()
	if query == nil || len(query) == 0 {
		return nil
	}
	rows := this.base.MakeSlice()
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
		if err == nil {
			this.base.cache = append(this.base.cache, this.base.acts...)
			this.base.acts = nil
		} else {
			this.bulkWrite = nil
			this.base.errMsg = err
		}
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
	if this.updater.strict && act.AType == ActTypeSub {
		av, _ := ParseInt(act.Val)
		dv := this.Val(act.OID)
		if dv < av {
			return ErrItemNotEnough(act.IID, av, dv)
		}
	}
	it := Config.IType(act.IID)
	if it == nil {
		return ErrITypeNotExist(act.IID)
	}
	//溢出判定
	if act.AType == ActTypeAdd || act.AType == ActTypeNew {
		v, ok := ParseInt(act.Val)
		if !ok || v <= 0 {
			return ErrActValIllegal
		}
		d := this.dataset.Count(act.IID)
		t := v + d
		imax := Config.IMax(act.IID)
		if imax > 0 && t > imax {
			overflow := t - imax
			act.Val = v - overflow
			if resolve, ok := it.(ITypeResolve); ok {
				if newId, NewNum, ok2 := resolve.Resolve(act.IID, int32(overflow)); ok2 {
					overflow = 0
					this.updater.Add(newId, int32(NewNum))
				}
			}
			if overflow > 0 {
				this.updater.overflow[act.IID] += overflow
			}
		}
	}

	if it.Unique() {
		return parseHMap(this, act)
	} else {
		return parseTable(this, act)
	}
}

func (this *Table) CreateId(iid int32) (oid string, err error) {
	it := Config.IType(iid)
	if it == nil {
		return "", ErrITypeNotExist(iid)
	}
	return it.CreateId(this.updater, iid)
}

//ParseId 解析道具，不可叠加道具不能使用iid解析
func (this *Table) ParseId(id interface{}) (iid int32, oid string, err error) {
	switch id.(type) {
	case string:
		oid = id.(string)
		iid, err = Config.ParseId(oid)
	default:
		if iid, _ = ParseInt32(id); iid > 0 {
			it := Config.IType(iid)
			if it == nil {
				err = fmt.Errorf("ParseId IType unknown:%v", id)
			} else if !it.Unique() {
				err = fmt.Errorf("不可叠加道具不能使用IID进行操作:%v", id)
			} else {
				oid, err = this.CreateId(iid)
			}
		} else {
			err = fmt.Errorf("ParseId args illegal:%v", id)
		}
	}
	return
}
