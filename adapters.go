package updater

import (
	"time"

	"github.com/hwcer/cosgo/schema"
	"github.com/hwcer/updater/dataset"
	"github.com/hwcer/updater/hamster"
	"github.com/hwcer/updater/operator"
)

// 本文件是"根包仅是 IID/IType 包装"的落点：四个适配器把扩展层模型接口
// （首参 *Updater，业务实现零改动）桥接成核心版 hamster 模型，并把模型的可选
// 能力（上限/溢出转化/叠加/OID 生成/文档工厂/跨天重置）翻译成核心的可选接口。
// 核心的数据流（含溢出控制）全部原生；适配器只做签名与词汇的翻译。

// ---------------- Document ----------------

type docAdapter struct{ m DocumentModel }

func (a *docAdapter) New(s *hamster.Store) any { return a.m.New(updaterOf(s)) }
func (a *docAdapter) Getter(s *hamster.Store, d *dataset.Document, keys []string) error {
	return a.m.Getter(updaterOf(s), d, keys)
}
func (a *docAdapter) Setter(s *hamster.Store, bw hamster.BulkWrite, dirty dataset.Update, unset []string) error {
	return a.m.Setter(updaterOf(s), bw, dirty, unset)
}

func (a *docAdapter) Reset(s *hamster.Store, last time.Time) bool { return legacyReset(a.m, s, last) }

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

// Limit 数值键上限
func (a *valuesAdapter) Limit(s *hamster.Store, key any) int64 {
	if iid, ok := key.(int32); ok {
		return modelIMax(a.m, iid)
	}
	return 0
}

// Overflow 超限转化：IType 上的 ITypeResolve（如分解成其他道具）
func (a *valuesAdapter) Overflow(s *hamster.Store, key any, n int64) (map[int32]int64, error) {
	iid, ok := key.(int32)
	if !ok {
		return nil, nil
	}
	u := updaterOf(s)
	if it := modelIType(a.m, iid); it != nil {
		if r, ok := it.(ITypeResolve); ok {
			return r.Resolve(u, iid, n)
		}
	}
	return nil, nil
}

func (a *valuesAdapter) Reset(s *hamster.Store, last time.Time) bool {
	return legacyReset(a.m, s, last)
}

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

func (a *collAdapter) iTypeCollection(u *Updater, iid int32) ITypeCollection {
	it := modelIType(a.m, iid)
	if it == nil {
		return nil
	}
	r, _ := it.(ITypeCollection)
	return r
}

// Limit 分组键（IID）上限
func (a *collAdapter) Limit(s *hamster.Store, key any) int64 {
	if iid, ok := key.(int32); ok {
		return modelIMax(a.m, iid)
	}
	return 0
}

// Overflow 超限转化
func (a *collAdapter) Overflow(s *hamster.Store, key any, n int64) (map[int32]int64, error) {
	iid, ok := key.(int32)
	if !ok {
		return nil, nil
	}
	u := updaterOf(s)
	if it := modelIType(a.m, iid); it != nil {
		if r, ok := it.(ITypeResolve); ok {
			return r.Resolve(u, iid, n)
		}
	}
	return nil, nil
}

// Stacked 同分组键是否单文档（可叠加）
func (a *collAdapter) Stacked(s *hamster.Store, key any) bool {
	iid, ok := key.(int32)
	if !ok {
		return true
	}
	if it := a.iTypeCollection(updaterOf(s), iid); it != nil {
		return it.Stacked(iid)
	}
	return true
}

// OID 数值键 → 文档 OID（仅可叠加形态；不可叠加返回空，由核心按件生成分支处理）
func (a *collAdapter) OID(s *hamster.Store, iid int32) (string, error) {
	it := a.iTypeCollection(updaterOf(s), iid)
	if it == nil {
		return "", ErrITypeNotExist(iid)
	}
	if !it.Stacked(iid) {
		return "", nil //不可叠加：无定点 OID，走按件生成
	}
	return it.GetOID(updaterOf(s), iid), nil
}

// NewDoc 生成新文档对象（新增/改即建语义）
func (a *collAdapter) NewDoc(s *hamster.Store, op *operator.Operator) (any, error) {
	u := updaterOf(s)
	it := a.iTypeCollection(u, op.IID)
	if it == nil {
		return nil, ErrITypeNotExist(op.IID)
	}
	return it.New(u, op)
}

func (a *collAdapter) Reset(s *hamster.Store, last time.Time) bool { return legacyReset(a.m, s, last) }

// ---------------- Virtual ----------------

type virtualAdapter struct{ m VirtualModel }

func (a *virtualAdapter) Has(s *hamster.Store, k any) bool { return a.m.Has(updaterOf(s), k) }
func (a *virtualAdapter) Get(s *hamster.Store, k any) any  { return a.m.Get(updaterOf(s), k) }
func (a *virtualAdapter) Select(s *hamster.Store, ks ...any) {
	a.m.Select(updaterOf(s), ks...)
}
func (a *virtualAdapter) Reload(s *hamster.Store) error { return a.m.Reload(updaterOf(s)) }
func (a *virtualAdapter) Update(s *hamster.Store, op *operator.Operator) {
	a.m.Update(updaterOf(s), op)
}

func (a *virtualAdapter) Reset(s *hamster.Store, last time.Time) bool {
	return legacyReset(a.m, s, last)
}
