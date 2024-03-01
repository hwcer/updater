package updater

import (
	"fmt"
	"github.com/hwcer/logger"
	"github.com/hwcer/updater/dataset"
	"github.com/hwcer/updater/operator"
)

type Receive func(oid string, doc any)

type collectionModel interface {
	Upsert(update *Updater, op *operator.Operator) bool
	Getter(update *Updater, keys []string, fn Receive) error //keys==nil 初始化所有
	Setter(update *Updater, bulkWrite dataset.BulkWrite) error
	BulkWrite(update *Updater) dataset.BulkWrite
}

// collectionUpsert set时如果不存在,是否自动转换为new
//type collectionUpsert interface {
//	Upsert(update *Updater, op *operator.Operator) bool
//}

type Collection struct {
	statement
	model   collectionModel
	dirty   dataset.Dirty
	remove  []string //需要移除内存的数据,仅仅RAMMaybe有效
	dataset dataset.Collection
}

func NewCollection(u *Updater, model any, ram RAMType) Handle {
	r := &Collection{}
	r.model = model.(collectionModel)
	r.statement = *newStatement(u, ram, r.operator, r.Has)
	return r
}
func (this *Collection) Parser() Parser {
	return ParserTypeCollection
}

//func (this *Collection) get(k string) (r *dataset.Document) {
//	return this.dataset.Get(k)
//}

func (this *Collection) val(id string) (r int64, ok bool) {
	if r, ok = this.values.get(id); ok {
		return
	} else {
		r, ok = this.dataset.Val(id)
	}
	return
}

func (this *Collection) save() (err error) {
	if this.Updater.Async || len(this.dirty) == 0 {
		return
	}
	bulkWrite := this.model.BulkWrite(this.Updater)
	if err = this.model.Setter(this.statement.Updater, this.dirty.BulkWrite(bulkWrite)); err == nil {
		this.dirty = nil
	}
	return
}

func (this *Collection) reset() {
	this.statement.reset()
	if this.dirty == nil {
		this.dirty = dataset.NewDirty()
	}
	if this.dataset == nil {
		this.dataset = dataset.New()
	}
}

func (this *Collection) release() {
	this.statement.release()
	if !this.Updater.Async && this.statement.ram == RAMTypeNone {
		this.dirty = nil
		this.dataset = nil
	}
}
func (this *Collection) init() error {
	this.dataset = dataset.New()
	if this.statement.ram == RAMTypeMaybe || this.statement.ram == RAMTypeAlways {
		this.Updater.Error = this.model.Getter(this.Updater, nil, this.Receive)
	}
	return this.Updater.Error
}

// 关闭时执行,玩家下线
func (this *Collection) destroy() (err error) {
	return this.save()
}

func (this *Collection) Has(id any) (r bool) {
	if oid, err := this.ObjectId(id); err == nil {
		r = this.dataset.Has(oid)
	} else {
		logger.Debug(err)
	}
	return
}

// Get 返回item,不可叠加道具只能使用oid获取
func (this *Collection) Get(key any) (r any) {
	if oid, err := this.ObjectId(key); err == nil {
		if i := this.dataset.Get(oid); i != nil {
			r = i.Interface()
		}
	} else {
		logger.Debug(err)
	}
	return
}

// Val 直接获取 item中的val值,不可叠加道具只能使用oid获取
func (this *Collection) Val(key any) (r int64) {
	if oid, err := this.ObjectId(key); err == nil {
		r, _ = this.val(oid)
	}
	return
}

// Set 设置 k= oid||iid
// Set(oid||iid,map[string]any)
// Set(oid||iid,key string,val any)
func (this *Collection) Set(k any, v ...any) {
	switch len(v) {
	case 1:
		if update := dataset.ParseUpdate(v[0]); update != nil {
			this.operator(operator.TypesSet, k, 0, update)
		} else {
			this.Updater.Error = ErrArgsIllegal(k, v)
		}
	case 2:
		if field, ok := v[0].(string); ok {
			this.operator(operator.TypesSet, k, 0, dataset.NewUpdate(field, v[1]))
		} else {
			this.Updater.Error = ErrArgsIllegal(k, v)
		}
	default:
		this.Updater.Error = ErrArgsIllegal(k, v)
	}
}

// New 使用全新的模型插入
func (this *Collection) New(v dataset.Model) (err error) {
	op := &operator.Operator{OID: v.GetOID(), IID: v.GetIID(), Type: operator.TypesNew, Result: []any{v}}
	if i, ok := v.(dataset.ModelVal); ok {
		op.Value = i.GetVal()
	} else {
		op.Value = 1
	}
	if err = this.mayChange(op); err != nil {
		return this.Updater.Errorf(err)
	}
	this.statement.Operator(op)
	//if this.verify {
	//	if err = this.Parse(op); err != nil {
	//		return this.Updater.Errorf(err)
	//	}
	//}
	return
}

func (this *Collection) Select(keys ...any) {
	for _, k := range keys {
		if oid, err := this.ObjectId(k); err == nil {
			this.statement.Select(oid)
		} else {
			logger.Alert(err)
		}
	}
}

func (this *Collection) Data() (err error) {
	if this.Updater.Error != nil {
		return this.Updater.Error
	}
	if len(this.keys) == 0 {
		return nil
	}
	keys := this.keys.ToString()
	if err = this.model.Getter(this.Updater, keys, this.Receive); err == nil {
		this.statement.date()
	}
	return
}

func (this *Collection) Verify() (err error) {
	if this.Updater.Error != nil {
		return this.Updater.Error
	}
	for _, act := range this.statement.operator {
		if err = this.Parse(act); err != nil {
			return
		}
	}
	this.statement.verify()
	return
}

func (this *Collection) submit() (err error) {
	//defer this.statement.done()
	if err = this.Updater.Error; err != nil {
		return
	}
	//r = this.statement.operator
	//同步到内存
	for _, op := range this.statement.cache {
		if op.Type.IsValid() {
			if err = this.dataset.Update(op); err == nil {
				this.dirty.Update(op)
			} else {
				logger.Alert("数据保存失败已经丢弃,Error:%v,Operator:%+v", err, op)
				err = nil
			}
		}
	}
	this.statement.submit()
	if err = this.save(); err != nil && this.ram != RAMTypeNone {
		logger.Alert("同步数据失败,等待下次同步:%v", err)
		err = nil
	}
	if len(this.remove) > 0 {
		for _, k := range this.remove {
			this.dataset.Del(k)
			//this.statement.history.Remove(k)
		}
		this.remove = nil
	}

	return
}

// Len 总记录数
func (this *Collection) Len() int {
	return len(this.dataset)
}

func (this *Collection) Range(h func(id string, val any) bool) {
	for id, dt := range this.dataset {
		if !h(id, dt.Interface()) {
			return
		}
	}
}

func (this *Collection) IType(iid int32) IType {
	if h, ok := this.model.(modelIType); ok {
		v := h.IType(iid)
		return itypesDict[v]
	} else {
		return this.Updater.IType(iid)
	}
}

func (this *Collection) ITypeCollection(iid int32) (r ITypeCollection) {
	if it := this.IType(iid); it != nil {
		r, _ = it.(ITypeCollection)
	}
	return
}

func (this *Collection) mayChange(op *operator.Operator) (err error) {
	it := this.ITypeCollection(op.IID)
	if it == nil {
		return ErrITypeNotExist(op.IID)
	}
	op.Bag = it.Id()
	if listen, ok := it.(ITypeListener); ok {
		listen.Listener(this.Updater, op)
	}
	if op.Type == operator.TypesDrop || op.Type == operator.TypesResolve {
		return nil
	}
	//可以堆叠道具
	if op.OID == "" && it.Stacked() {
		op.OID, err = it.ObjectId(this.Updater, op.IID)
	}
	if err != nil {
		return
	}
	if op.OID != "" {
		this.statement.Select(op.OID)
	}

	return
}

func (this *Collection) operator(t operator.Types, k any, v int64, r any) {
	if this.Updater.Error != nil {
		return
	}
	op := operator.New(t, v, r)
	switch d := k.(type) {
	case string:
		op.OID = d
	default:
		op.IID = dataset.ParseInt32(k)
	}
	if op.IID == 0 {
		op.IID, this.Updater.Error = Config.ParseId(this.Updater, op.OID)
	}
	if this.Updater.Error != nil {
		return
	}
	if err := this.mayChange(op); err != nil {
		this.Updater.Error = err
		return
	}
	this.statement.Operator(op)
}

// Receive 接收业务逻辑层数据
func (this *Collection) Receive(id string, data any) {
	this.dataset.Set(id, data)
}

func (this *Collection) ObjectId(key any) (oid string, err error) {
	if v, ok := key.(string); ok {
		return v, nil
	}
	iid := dataset.ParseInt32(key)
	if iid <= 0 {
		return "", fmt.Errorf("iid empty:%v", iid)
	}
	it := this.ITypeCollection(iid)
	if it == nil {
		return "", fmt.Errorf("IType unknown:%v", iid)
	}
	if !it.Stacked() {
		return "", ErrOIDEmpty(iid)
	}
	oid, err = it.ObjectId(this.Updater, iid)
	if err == nil && oid == "" {
		err = ErrUnableUseIIDOperation
	}
	return
}

// Remove 从内存中移除可能近期不在需要的数据,仅在RAMTypeMaybe模式下生效
func (this *Collection) Remove(keys ...any) {
	if this.ram != RAMTypeMaybe {
		return
	}
	for _, k := range keys {
		if oid, err := this.ObjectId(k); err == nil {
			this.remove = append(this.remove, oid)
		}
	}
}
