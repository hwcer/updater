package updater

import (
	"fmt"
	"time"

	"github.com/hwcer/cosgo/schema"
	"github.com/hwcer/logger"
	"github.com/hwcer/updater/dataset"
	"github.com/hwcer/updater/hamster"
	"github.com/hwcer/updater/operator"
)

// 本文件是**根包仅是 IID/IType 包装**的落点：四个适配器把扩展层模型接口
// （首参 *Updater，含 IType/Field 等道具方法，业务实现零改动）桥接成核心版
// hamster 模型，并把道具语义经核心的可选注入接口（Keyer/OperatorDecorator/
// ParseDecorator）送进核心句柄。核心代码里不出现任何道具词汇。

// ---------------- Document ----------------

type docAdapter struct{ m DocumentModel }

func (a *docAdapter) New(s *hamster.Store) any { return a.m.New(updaterOf(s)) }
func (a *docAdapter) Getter(s *hamster.Store, d *dataset.Document, keys []string) error {
	return a.m.Getter(updaterOf(s), d, keys)
}
func (a *docAdapter) Setter(s *hamster.Store, bw hamster.BulkWrite, dirty dataset.Update, unset []string) error {
	return a.m.Setter(updaterOf(s), bw, dirty, unset)
}

// Key 非 string key（iid）→ 字段名
func (a *docAdapter) Key(s *hamster.Store, k any) (string, error) {
	if str, ok := k.(string); ok {
		return str, nil
	}
	return a.m.Field(updaterOf(s), dataset.ParseInt32(k))
}

// DecorateOperator 原 Document.operator 的道具段：IType(0) 必需 + OID 换算 + 预读监听
func (a *docAdapter) DecorateOperator(s *hamster.Store, op *operator.Operator) bool {
	u := updaterOf(s)
	it := modelIType(a.m, 0)
	if it == nil {
		u.Error = fmt.Errorf("document operator key empty:%+v", op)
		return false
	}
	op.IType = it.ID()
	if oc, ok := it.(ITypeOID); ok {
		op.OID = oc.GetOID(u, op.IID)
	}
	if l, ok := it.(ITypeListener); ok {
		l.Listener(u, op)
	}
	return true
}

// DecorateParse 溢出检查 + 余额不足的道具语汇错误
func (a *docAdapter) DecorateParse(h hamster.Handle, s *hamster.Store, op *operator.Operator) (bool, error) {
	u := updaterOf(s)
	doc := h.(*hamster.Document)
	switch op.OType {
	case operator.TypesAdd:
		return false, overflowItem(u, a.m, func(int32) int64 { return doc.Val(op.Field) }, op)
	case operator.TypesSub:
		if d := doc.Val(op.Field); d < op.Value && !s.CreditAllowed {
			return true, ErrItemNotEnough(op.IID, op.Value, d)
		}
	}
	return false, nil
}

// Reset 桥接旧签名的 ModelReset（业务模型保持 Reset(*Updater, time.Time) 不变）
func (a *docAdapter) Reset(s *hamster.Store, last time.Time) bool { return legacyReset(a.m, s, last) }

// legacyReset 旧签名跨天重置桥：业务模型实现 Reset(*Updater, time.Time) bool 即可
func legacyReset(m any, s *hamster.Store, last time.Time) bool {
	if r, ok := m.(interface {
		Reset(*Updater, time.Time) bool
	}); ok {
		return r.Reset(updaterOf(s), last)
	}
	return false
}

// ---------------- Values ----------------

type valuesAdapter struct{ m ValuesModel }

func (a *valuesAdapter) Getter(s *hamster.Store, d *dataset.Values, keys []int32) error {
	return a.m.Getter(updaterOf(s), d, keys)
}
func (a *valuesAdapter) Setter(s *hamster.Store, bw hamster.BulkWrite, dirty dataset.Data, unset []int32) error {
	return a.m.Setter(updaterOf(s), bw, dirty, unset)
}

// DecorateOperator 原 Values.operator 的道具段：IType 查不到**静默丢弃**
func (a *valuesAdapter) DecorateOperator(s *hamster.Store, op *operator.Operator) bool {
	u := updaterOf(s)
	it := modelIType(a.m, op.IID)
	if it == nil {
		logger.Debug("IType not exist:%v", op.IID)
		return false
	}
	op.IType = it.ID()
	if l, ok := it.(ITypeListener); ok {
		l.Listener(u, op)
	}
	return true
}

func (a *valuesAdapter) DecorateParse(h hamster.Handle, s *hamster.Store, op *operator.Operator) (bool, error) {
	u := updaterOf(s)
	vals := h.(*hamster.Values)
	switch op.OType {
	case operator.TypesAdd:
		return false, overflowItem(u, a.m, func(int32) int64 { return vals.Val(op.IID) }, op)
	case operator.TypesSub:
		if d := vals.Val(op.IID); d < op.Value && !s.CreditAllowed {
			return true, ErrItemNotEnough(op.IID, op.Value, d)
		}
	}
	return false, nil
}

func (a *valuesAdapter) Reset(s *hamster.Store, last time.Time) bool { return legacyReset(a.m, s, last) }

// ---------------- Collection ----------------

type collAdapter struct{ m CollectionModel }

func (a *collAdapter) Upsert(s *hamster.Store, op *operator.Operator) bool {
	return a.m.Upsert(updaterOf(s), op)
}
func (a *collAdapter) Schema() *schema.Schema { return a.m.Schema() }
func (a *collAdapter) Getter(s *hamster.Store, d *dataset.Collection, keys []string) error {
	return a.m.Getter(updaterOf(s), d, keys)
}
func (a *collAdapter) Setter(s *hamster.Store, bw hamster.BulkWrite, _id string, dirty dataset.Update, unset []string) error {
	return a.m.Setter(updaterOf(s), bw, _id, dirty, unset)
}

// ValueJSName 数值字段名（核心 Field() 的回落链会用到）
func (a *collAdapter) GetValueJSName() string {
	if f, ok := a.m.(CollectionModelValueJSName); ok {
		return f.GetValueJSName()
	}
	return ""
}

// Key 原 GetOID：string 直通；iid → Stacked 判定 + ITypeOID.GetOID
func (a *collAdapter) Key(s *hamster.Store, k any) (string, error) {
	if str, ok := k.(string); ok {
		return str, nil
	}
	u := updaterOf(s)
	iid := dataset.ParseInt32(k)
	it := a.iTypeCollection(u, iid)
	if it == nil {
		return "", ErrITypeNotExist(iid)
	}
	if !it.Stacked(iid) {
		return "", ErrObjectIdEmpty(iid)
	}
	if oid := it.GetOID(u, iid); oid != "" {
		return oid, nil
	}
	return "", ErrUnableUseIIDOperation
}

func (a *collAdapter) iTypeCollection(u *Updater, iid int32) ITypeCollection {
	it := modelIType(a.m, iid)
	if it == nil {
		return nil
	}
	r, _ := it.(ITypeCollection)
	return r
}

// DecorateOperator 原 mayChange：ParseId/IType/预读监听/OID 填充
func (a *collAdapter) DecorateOperator(s *hamster.Store, op *operator.Operator) bool {
	u := updaterOf(s)
	it := a.iTypeCollection(u, op.IID)
	if it == nil {
		u.Error = ErrITypeNotExist(op.IID)
		return false
	}
	op.IType = it.ID()
	if l, ok := it.(ITypeListener); ok {
		l.Listener(u, op)
	}
	if op.OType == operator.TypesDrop || op.OType == operator.TypesResolve {
		return true
	}
	if op.OID == "" && it.Stacked(op.IID) {
		op.OID = it.GetOID(u, op.IID)
	}
	return true
}

// DecorateParse 溢出 + 不可叠加转 New + 缺失新建 + 道具语汇余额错误
func (a *collAdapter) DecorateParse(h hamster.Handle, s *hamster.Store, op *operator.Operator) (bool, error) {
	u := updaterOf(s)
	c := h.(*hamster.Collection)
	field := c.Field()
	switch op.OType {
	case operator.TypesAdd:
		if err := overflowColl(u, a, c, field, op); err != nil {
			return true, err
		}
		it := a.iTypeCollection(u, op.IID)
		if it != nil && !it.Stacked(op.IID) {
			return true, a.newEquip(u, c, it, field, op) //不可叠加装备类：Add 转 N 件 New
		}
		if !c.Has(op.OID) {
			return true, a.newItem(u, c, it, field, op) //可叠加但文档不存在：生成
		}
	case operator.TypesSub:
		if d := c.Val(op.OID); d < op.Value && !s.CreditAllowed {
			return true, ErrItemNotEnough(op.IID, op.Value, d)
		}
	case operator.TypesSet:
		if !c.Has(op.OID) {
			if a.m.Upsert(u, op) {
				return true, a.newItem(u, c, a.iTypeCollection(u, op.IID), field, op)
			}
			return true, ErrItemNotExist(op.OID)
		}
	}
	return false, nil
}

func (a *collAdapter) Reset(s *hamster.Store, last time.Time) bool { return legacyReset(a.m, s, last) }

// newEquip 不可叠加装备类：按 Value 逐件 it.New 后插入（原 collectionHandleNewEquip）
func (a *collAdapter) newEquip(u *Updater, c *hamster.Collection, it ITypeCollection, field string, op *operator.Operator) error {
	op.OType = operator.TypesNew
	if op.Value == 0 {
		op.Value = 1
	}
	if op.Result != nil {
		return nil
	}
	var items []any
	cc := op.Clone(1)
	defer cc.Release()
	var err error
	var item any
	for i := int64(1); i <= op.Value; i++ {
		if item, err = it.New(u, cc); err != nil {
			return err
		}
		items = append(items, item)
	}
	op.Result, op.OID, err = c.InsertValues(items...)
	return err
}

// newItem 文档不存在时经 ITypeCollection.New 生成（原 collectionHandleNewItem）
func (a *collAdapter) newItem(u *Updater, c *hamster.Collection, it ITypeCollection, field string, op *operator.Operator) error {
	if it == nil {
		return ErrITypeNotExist(op.IID)
	}
	i, err := it.New(u, op)
	if err != nil {
		return err
	}
	if op.OType == operator.TypesSet {
		doc := dataset.NewDoc(i)
		doc.Update(op.Result.(dataset.Update))
		doc.Save()
		op.Value = doc.GetInt64(field)
	}
	op.OType = operator.TypesNew
	op.Result, op.OID, err = c.InsertValues(i)
	return err
}

// overflowColl 集合的持有量统计：可叠加取其文档 val，不可叠加扫描计数（含未落库新增）
func overflowColl(u *Updater, a *collAdapter, c *hamster.Collection, field string, op *operator.Operator) error {
	if !op.OType.IsAdd() || op.IID == 0 {
		return nil
	}
	imax := modelIMax(a.m, op.IID)
	if imax <= 0 {
		return nil
	}
	it := modelIType(a.m, op.IID)
	if it == nil {
		return ErrITypeNotExist(op.IID)
	}
	count := func() int64 {
		ic, ok := it.(ITypeCollection)
		if ok && ic.Stacked(op.IID) {
			return c.Val(op.IID)
		}
		var n int64
		c.Range(func(id string, doc *dataset.Document) bool {
			if docIID(doc) == op.IID {
				n++
			}
			return true
		})
		return n
	}
	return overflowValue(u, it, count, op, func() int64 { return imax })
}

// ---------------- Virtual ----------------

type virtualAdapter struct{ m VirtualModel }

func (a *virtualAdapter) Has(s *hamster.Store, k any) bool    { return a.m.Has(updaterOf(s), k) }
func (a *virtualAdapter) Get(s *hamster.Store, k any) any     { return a.m.Get(updaterOf(s), k) }
func (a *virtualAdapter) Select(s *hamster.Store, ks ...any)  { a.m.Select(updaterOf(s), ks...) }
func (a *virtualAdapter) Reload(s *hamster.Store) error       { return a.m.Reload(updaterOf(s)) }
func (a *virtualAdapter) Update(s *hamster.Store, op *operator.Operator) { a.m.Update(updaterOf(s), op) }

// Key iid → 委托字段名
func (a *virtualAdapter) Key(s *hamster.Store, k any) (string, error) {
	if str, ok := k.(string); ok {
		return str, nil
	}
	f, ok := a.m.Field(dataset.ParseInt32(k))
	if !ok {
		return "", ErrArgsIllegal(k)
	}
	return f, nil
}

// DecorateOperator IType 盖键（可选，查不到不拦截）
func (a *virtualAdapter) DecorateOperator(s *hamster.Store, op *operator.Operator) bool {
	if it := modelIType(a.m, op.IID); it != nil {
		op.IType = it.ID()
	}
	return true
}

func (a *virtualAdapter) Reset(s *hamster.Store, last time.Time) bool { return legacyReset(a.m, s, last) }
