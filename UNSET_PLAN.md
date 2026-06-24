# Updater v1.5.0 升级实现记录

## 版本

- updater v1.5.0（2026-06-24）

## 一、BulkWrite 接口（updater/define.go）

从 dataset 包提升到 updater 包，对齐 cosmo.BulkWrite8 签名。无 Unset 方法（$unset 由 Model.Setter 内部处理）。

```go
type BulkWrite interface {
    Submit() error
    Update(model any, data any, where ...any)
    Insert(model any, documents ...any)
    Delete(model any, where ...any)
    String() string
}
```

## 二、全局工厂 + Updater 缓存

```go
var Config = struct {
    BulkWrite func(*Updater) BulkWrite
}{...}

func (u *Updater) BulkWrite() BulkWrite {
    if u.bulkWrite == nil && Config.BulkWrite != nil {
        u.bulkWrite = Config.BulkWrite(u)
    }
    return u.bulkWrite
}
```

业务层注册：`Config.BulkWrite = func(u) { return db.BulkWrite8() }`

## 三、TypesUnset 操作

`TypesUnset = 6`，Document/Collection/Values 三种 Handle 均支持。

### dataset.Document

```go
type Document struct {
    data  any
    dirty Update
    unset map[string]struct{}  // 用 map 去重
}

func (doc *Document) Unset(k string)                    // 标记 + 内存修改
func (doc *Document) Save() (Update, []string)          // 返回 dirty + unsets
```

内存修改（`unsetMemory`）：ModelUnset 可选接口优先 → schema.SetValue 兜底（有 `.` 时传 `reflect.Value{}` 真删 map key）。

### dataset.Values

```go
func (val *Values) Unset(k int32)                       // delete(data, k) + 记录 unset
func (val *Values) Save() (Data, []int32)               // 返回 dirty + unsets
```

TypesDel 已移除，统一使用 TypesUnset。

## 四、提交流程

```
Updater.Submit()
  收敛循环 (data → verify)
  for each handle:
    handle.submit() → handle.save() → 操作加入共享 BulkWrite
  u.BulkWrite().Submit()               ← 一次原子提交
  u.bulkWrite = nil                    ← 无论成败都清理
```

bw == nil 时返回 `ErrBulkWriteNotInit`，不静默丢弃。
Destroy() 末尾也调 `bw.Submit()` 确保下线刷盘。

## 五、三种 Handle save() 实现

### Collection

通过 `CollectionBulkWrite`（实现 `dataset.CollectionWriter`）封装：
- Delete/Insert → 直接转发到 `Updater.BulkWrite()` 带上 model
- Setter → 调 `model.Setter(u, bw, _id, dirty, unset)` 由 Model 全权处理

```go
// dataset.CollectionWriter 接口
type CollectionWriter interface {
    Delete(where ...any)
    Insert(documents ...any)
    Setter(_id string, dirty Update, unset []string) error
}

// dataset.Collection.Save 只收 CollectionWriter
func (coll *Collection) Save(w CollectionWriter, model any) error
```

### Document

```go
dirty, unsets := this.dataset.Save()
if len(dirty) > 0 || len(unsets) > 0 {
    this.model.Setter(u, bw, dirty, unsets)
}
```

### Values

```go
dirty, unsets := this.dataset.Save()
if len(dirty) > 0 || len(unsets) > 0 {
    this.model.Setter(u, bw, dirty, unsets)
}
```

## 六、Model 接口

```go
type CollectionModel interface {
    IType(iid int32) int32
    Upsert(update *Updater, op *operator.Operator) bool
    Schema() *schema.Schema
    Getter(update *Updater, data *dataset.Collection, keys []string) error
    Setter(update *Updater, bw BulkWrite, _id string, dirty dataset.Update, unset []string) error
}

type DocumentModel interface {
    New(update *Updater) any
    IType(int32) int32
    Field(update *Updater, iid int32) (string, error)
    Getter(update *Updater, data *dataset.Document, keys []string) error
    Setter(update *Updater, bw BulkWrite, dirty dataset.Update, unset []string) error
}

type ValuesModel interface {
    IType(iid int32) int32
    Getter(u *Updater, data *dataset.Values, keys []int32) error
    Setter(u *Updater, bw BulkWrite, dirty dataset.Data, unset []int32) error
}
```

可选接口：`dataset.ModelUnset { Unset(k string) }` — 用于内存修改。
