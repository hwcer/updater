package updater

import (
	"fmt"
	"github.com/hwcer/cosgo/logger"
	"github.com/hwcer/updater/v2/dataset"
	"github.com/hwcer/updater/v2/operator"
)

type Receive func(oid string, doc any)

type collectionModel interface {
	Getter(update *Updater, keys []string, fn Receive) error //keys==nil 初始化所有
	Setter(update *Updater, bulkWrite dataset.BulkWrite) error
	BulkWrite(update *Updater) dataset.BulkWrite
}

type Collection struct {
	*statement
	keys    documentKeys
	model   collectionModel
	dirty   dataset.Dirty
	dataset dataset.Collection
}

func NewCollection(u *Updater, model any, ram RAMType) Handle {
	r := &Collection{}
	r.model = model.(collectionModel)
	r.statement = NewStatement(u, ram, r.Operator)
	return r
}
func (this *Collection) Parser() Parser {
	return ParserTypeCollection
}

func (this *Collection) IType(iid int32) (it ITypeCollection) {
	if i := this.Updater.IType(iid); i != nil {
		it, _ = i.(ITypeCollection)
	}
	return
}

// Has 查询key(DBName)是否已经初始化
func (this *Collection) has(key string) (r bool) {
	if this.statement.ram == RAMTypeAlways || (this.keys != nil && this.keys[key]) {
		return true
	}
	if this.dirty != nil && this.dirty.Has(key) {
		return true
	}
	if this.dataset != nil && this.dataset.Has(key) {
		return true
	}
	return false
}

func (this *Collection) get(k string) (r *dataset.Document) {
	return this.dataset.Get(k)
}

func (this *Collection) val(iid int32) (r int64) {
	if v, ok := this.values[iid]; ok {
		return v
	}
	r = this.dataset.Count(iid)
	this.values[iid] = r
	return
}

func (this *Collection) save() (err error) {
	if len(this.dirty) == 0 {
		return
	}
	bulkWrite := this.model.BulkWrite(this.Updater)
	if err = this.model.Setter(this.statement.Updater, this.dirty.BulkWrite(bulkWrite)); err == nil {
		this.dirty = nil
	}
	return
}

func (this *Collection) reset() {
	this.keys = documentKeys{}
	this.statement.reset()
	if this.dirty == nil {
		this.dirty = dataset.NewDirty()
	}
	if this.dataset == nil {
		this.dataset = dataset.New()
	}
}

func (this *Collection) release() {
	this.keys = nil
	this.statement.release()
	if this.statement.ram == RAMTypeNone {
		this.dirty = nil
		this.dataset = nil
	}
}
func (this *Collection) init() error {
	this.dataset = dataset.New()
	if this.statement.ram == RAMTypeAlways {
		this.Updater.Error = this.model.Getter(this.Updater, nil, this.Receive)
	}
	return this.Updater.Error
}

// 关闭时执行,玩家下线
func (this *Collection) flush() (err error) {
	return this.save()
}

// Get 返回item,不可叠加道具只能使用oid获取
func (this *Collection) Get(key any) (r any) {
	if oid, err := this.ObjectId(key); err == nil {
		if i := this.get(oid); i != nil {
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
		if i := this.get(oid); i != nil {
			r = i.VAL()
		}
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
			this.Operator(operator.Types_Set, k, 0, update)
		} else {
			this.Updater.Error = ErrArgsIllegal(k, v)
		}
	case 2:
		if field, ok := v[0].(string); ok {
			this.Operator(operator.Types_Set, k, 0, dataset.NewUpdate(field, v[1]))
		} else {
			this.Updater.Error = ErrArgsIllegal(k, v)
		}
	default:
		this.Updater.Error = ErrArgsIllegal(k, v)
	}
}

func (this *Collection) New(op *operator.Operator) (err error) {
	if op.OID == "" && op.IID <= 0 {
		return this.Updater.Errorf("operator oid and iid empty:%v", op)
	}
	if op.IID <= 0 {
		op.IID, this.Updater.Error = Config.ParseId(this.Updater, op.OID)
	}
	if this.Updater.Error != nil {
		return this.Updater.Error
	}
	it := this.IType(op.IID)
	if it == nil {
		return this.Updater.Errorf(ErrITypeNotExist(op.IID))
	}
	if !it.Multiple() {
		this.operatorUnique(op)
	} else {
		this.operatorMultiple(op)
	}
	if this.Updater.Error != nil {
		return this.Updater.Error
	}
	if listen, ok := this.model.(Listener); ok {
		listen.Listener(this.Updater, op)
	}
	this.statement.Operator(op)
	if this.verified {
		err = this.Verify()
	}
	return
}

func (this *Collection) Select(keys ...any) {
	if this.ram == RAMTypeAlways {
		return
	}
	for _, k := range keys {
		if oid, err := this.ObjectId(k); err == nil && !this.has(oid) {
			this.keys[oid] = true
		} else {
			logger.Debug(err)
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
	keys := this.keys.Keys()
	err = this.model.Getter(this.Updater, keys, this.Receive)
	this.keys = nil
	return
}

func (this *Collection) Verify() (err error) {
	if this.Updater.Error != nil {
		return this.Updater.Error
	}
	for _, act := range this.statement.operator {
		if err = this.verify(act); err != nil {
			return
		}
		if err = this.Parse(act); err != nil {
			return
		}
	}
	this.statement.verified = true
	return
}

func (this *Collection) Save() (err error) {
	if this.Updater.Error != nil {
		return this.Updater.Error
	}
	//同步到内存
	for _, op := range this.statement.operator {
		if op.Type.IsValid() {
			if err = this.dataset.Update(op); err != nil {
				logger.Warn("数据保存失败已经丢弃,Error:%v,Operator:%+v\n", err, op)
				err = nil
			} else {
				this.dirty.Update(op)
			}
		}
	}
	this.statement.done()
	if err = this.save(); err != nil && this.ram != RAMTypeNone {
		logger.Warn("同步数据失败,等待下次同步:%v", err)
		err = nil
	}
	return
}

func (this *Collection) Operator(t operator.Types, k any, v int64, r any) {
	if this.Updater.Error != nil {
		return
	}
	op := operator.New(t, v, r)
	switch d := k.(type) {
	case string:
		op.OID = d
	default:
		op.IID = ParseInt32(k)
	}

	//del set 使用oid,iid,使用iid时,必须可以无限叠加,具有唯一OID
	//if t == operator.TypeDel || t == operator.TypeSet {
	//	op.OID, this.Updater.Error = this.ObjectId(k)
	//} else {
	//	op.IID = ParseInt32(k)
	//}
	//if this.Updater.Error != nil {
	//	return
	//}
	//if op.IID <= 0 {
	//	_ = this.Errorf("iid illegal:%v", op)
	//	return
	//}
	_ = this.New(op)
}

// Receive 接收业务逻辑层数据
func (this *Collection) Receive(id string, data any) {
	this.dataset.Set(id, data)
}

func (this *Collection) verify(cache *operator.Operator) (err error) {
	it := this.Updater.IType(cache.IID)
	if it == nil {
		return ErrITypeNotExist(cache.IID)
	}
	//溢出判定
	if cache.Type == operator.Types_Add {
		val := ParseInt64(cache.Value)
		num := this.dataset.Count(cache.IID)
		tot := val + num
		imax := Config.IMax(cache.IID)
		if imax > 0 && tot > imax {
			overflow := tot - imax
			if overflow > val {
				overflow = val //imax有改动
			}
			val -= overflow
			cache.Value = val
			if resolve, ok := it.(ITypeResolve); ok {
				if err = resolve.Resolve(this.Updater, cache); err != nil {
					return
				} else {
					overflow = 0
				}
			}
			if overflow > 0 {
				//this.Adapter.overflow[cache.IID] += overflow
			}
		}
		if val == 0 {
			cache.Type = operator.Types_Resolve
		}
	}
	return
}

// operatorUnique 可以无限叠加的道具
func (this *Collection) operatorMultiple(op *operator.Operator) {
	if op.OID == "" {
		op.OID, this.Updater.Error = this.ObjectId(op.IID)
	}
	if op.Type == operator.Types_New {
		op.Type = operator.Types_Add
	}
}

// operatorMultiple 不可以叠加的道具不能SUB,只能DEL
func (this *Collection) operatorUnique(op *operator.Operator) {
	switch op.Type {
	case operator.Types_Sub:
		_ = this.Errorf("sub disabled:%v", op.IID)
	case operator.Types_Add:
		op.Type = operator.Types_New
	case operator.Types_New:

	default:
		if op.OID == "" {
			_ = this.Errorf("operator unique item oid empty:%+v", op) //SET DEL
		}
	}
}

func (this *Collection) ObjectId(key any) (oid string, err error) {
	if v, ok := key.(string); ok {
		return v, nil
	}
	iid := ParseInt32(key)
	if iid <= 0 {
		return "", fmt.Errorf("iid empty:%v", iid)
	}
	it := this.IType(iid)
	if it == nil {
		return "", fmt.Errorf("IType unknown:%v", iid)
	}
	if !it.Multiple() {
		return "", fmt.Errorf("IType Multiple:%v", iid)
	}
	return it.ObjectId(this.Updater, iid)
}
