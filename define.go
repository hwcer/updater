package updater

import (
	"github.com/hwcer/updater/operator"
)

var Config = struct {
	IMax      func(iid int32) int64                                     //通过道具iid查找上限
	IType     func(iid int32) int32                                     //通过道具iid查找IType ID
	ParseId   func(adapter *Updater, oid string) (iid int32, err error) //解析OID获得IID
	BulkWrite func(u *Updater) BulkWrite                                //全局 BulkWrite 工厂
}{}

// Status 状态位标记
type Status uint8

const (
	StatusInit     Status = 1 << iota // 已初始化，按模块预设加载数据
	StatusSubmit                      // 需要触发提交
	StatusChanged                     // 数据变动，需要 Data 更新
	StatusOperated                    // 新操作，需要 Verify 检查
	StatusTesting                     // 测试模式，不写库
	StatusDevelop                     // 开发者模式，业务层自取
)

func (s *Status) Has(flags ...Status) bool {
	for _, f := range flags {
		if *s&f != 0 {
			return true
		}
	}
	return false
}
func (s *Status) Set(flags ...Status) {
	for _, f := range flags {
		*s |= f
	}
}
func (s *Status) Unset(flags ...Status) {
	for _, f := range flags {
		*s &^= f
	}
}

// BulkWrite 跨集合批量写入接口
type BulkWrite interface {
	Submit() error
	Update(model any, data any, where ...any)
	Insert(model any, documents ...any)
	Delete(model any, where ...any)
	String() string
}

// IType 一个IType对于一种数据类型·
// 多种数据类型 可以用一种数据模型(model,一张表结构)
type IType interface {
	ID() int32 //IType 唯一标志
}
type ITypeOID interface {
	GetOID(u *Updater, iid int32) (oid string) //使用IID创建OID,仅限于可以叠加道具,不可以叠加道具返回空,使用NEW来创建
}

type ITypeCollection interface {
	IType
	ITypeOID
	New(u *Updater, op *operator.Operator) (item any, err error) //根据Operator信息生成新对象
	Stacked(int32) bool                                          //是否可以叠加
}

// ITypeResolve 自动分解,如果没有分解方式超出上限则使用系统默认方式（丢弃）处理
// Verify执行的一部分(Data之后Save之前)
// 使用Resolve前，需要使用ITypeListener监听将可能分解成的道具ID使用adapter.Select预读数据
// 使用Resolve时需要关联IMax指定道具上限
type ITypeResolve interface {
	Resolve(u *Updater, iid int32, val int64) error
}

// ITypeResult 设置返回结果
type ITypeResult interface {
	Result(u *Updater, opt *operator.Operator) any
}
type ITypeListener interface {
	Listener(u *Updater, op *operator.Operator)
}

type Keys map[any]struct{}

func (this Keys) Has(k any) (ok bool) {
	_, ok = this[k]
	return
}

func (this Keys) Remove(k any) {
	delete(this, k)
}

func (this Keys) ToString() (r []string) {
	for k := range this {
		if sk, ok := k.(string); ok {
			r = append(r, sk)
		}
	}
	return
}

func (this Keys) ToInt32() (r []int32) {
	for k := range this {
		if ik, ok := k.(int32); ok {
			r = append(r, ik)
		}
	}
	return
}

func (this Keys) Merge(src Keys) {
	for k := range src {
		this[k] = struct{}{}
	}
}

func (this Keys) Select(ks ...any) {
	for _, k := range ks {
		this[k] = struct{}{}
	}
}
