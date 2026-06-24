# Updater 升级计划

## 目标

1. 新增 `TypesUnset` 操作，支持 MongoDB `$unset`
2. 基于 MongoDB 8.0 `client.BulkWrite()` 重构持久化层，所有 Handle 共享一个 BulkWrite，一次原子提交

不考虑向后兼容。

## 已完成

- `cosmo v1.4.0` — `BulkWrite.Unset()` + `BulkWrite8` 跨集合批量写入
- `dataset/define.go` — `BulkWrite` 接口已加 `Unset`

---

## 一、dataset.BulkWrite 接口重定义

对齐 `cosmo.BulkWrite8` 签名，每个操作带 model 参数。`cosmo.BulkWrite8` **直接实现**此接口，零包装。

```go
type BulkWrite interface {
    Submit() error
    Update(model any, data any, where ...any)
    Unset(model any, keys []string, where ...any)
    Insert(model any, documents ...any)
    Delete(model any, where ...any)
    String() string
}
```

## 二、全局工厂 + Updater 缓存

```go
var Config = struct {
    // ...existing...
    BulkWrite func(*Updater) dataset.BulkWrite
}{...}

func (u *Updater) BulkWrite() dataset.BulkWrite {
    if u.bulkWrite == nil && Config.BulkWrite != nil {
        u.bulkWrite = Config.BulkWrite(u)
    }
    return u.bulkWrite
}
```

业务层注册：
```go
updater.Config.BulkWrite = func(u *updater.Updater) dataset.BulkWrite {
    return db.BulkWrite8()
}
```

## 三、TypesUnset 操作

`dataset.Document` 新增 `unset map[string]struct{}` 字段。`Save()` 返回 `[]string`（unset keys），一次调用同时产出 $set 和 $unset。

```go
func (doc *Document) Unset(k string)         // 标记字段待 unset
func (doc *Document) Save(dirty Update) []string  // 返回 unset keys
```

## 四、三种 Handle 统一走 BulkWrite

### 提交流程

```
Updater.Submit()
  收敛循环 (data → verify)
  for each handle:
    handle.submit() → handle.save() → 操作加入共享 BulkWrite
  u.BulkWrite().Submit()  ← 一次原子提交
```

### Handle 各自的 save() 逻辑

**Collection**（Insert/Delete 框架直调 BulkWrite，Update 走 model.Setter 每文档一次）：
```go
bw := u.BulkWrite()
for oid, op := range dataset.dirty {
    if Delete:  bw.Delete(this.model, oid)
    if Insert:  bw.Insert(this.model, doc.Any())
    if Update:
        unsets := doc.Save(dirty)
        this.model.Setter(u, bw, oid, dirty)          // Model 决定如何写
        if len(unsets) > 0 { bw.Unset(this.model, unsets, oid) }
}
```

**Document**：
```go
bw := u.BulkWrite()
unsets := this.dataset.Save(this.setter)
this.model.Setter(u, bw, this.setter)                 // Model 决定如何写
if len(unsets) > 0 { bw.Unset(this.dataset.Any(), unsets, u.Uid()) }
```

**Values**（Model 内部做 iid→field 转换）：
```go
bw := u.BulkWrite()
this.dataset.Save(this.setter)
this.model.Setter(u, bw, this.setter)                 // Model 决定如何写
```

### BulkWrite 的 model 参数

由各 Model 的 Setter 内部决定传什么给 BulkWrite：

| Handle | Setter 内部调用 | model 来源 |
|--------|----------------|-----------|
| Collection | `bw.Update(this, dirty, oid)` | `this`（如 `*Items`） |
| Document | `bw.Update(this.New(u), dirty, uid)` | `New()` 返回值（如 `*Role`） |
| Values | `bw.Update(&Role{}, converted, uid)` | Model 自知宿主文档类型 |

Insert/Delete 由框架直调 `bw.Insert(this.model, ...)` / `bw.Delete(this.model, ...)`。

## 五、Model 接口统一

所有 Model 保留 `Setter`，签名统一带 `bulkWrite dataset.BulkWrite`，**由 Model 决定写入方式**（传哪个 model struct、如何转换数据）。框架只负责调度。

```go
type CollectionModel interface {
    IType(iid int32) int32
    Upsert(update *Updater, op *operator.Operator) bool
    Schema() *schema.Schema
    Getter(update *Updater, data *dataset.Collection, keys []string) error
    // Setter 每个文档调用一次，Model 内部调 bw.Update(this, dirty, _id)
    Setter(update *Updater, bulkWrite dataset.BulkWrite, _id string, dirty dataset.Update) error
    // 移除: BulkWrite
}

type DocumentModel interface {
    New(update *Updater) any
    IType(int32) int32
    Field(update *Updater, iid int32) (string, error)
    Getter(update *Updater, data *dataset.Document, keys []string) error
    // Model 内部调 bw.Update(this.New(), dirty, uid)
    Setter(update *Updater, bulkWrite dataset.BulkWrite, dirty dataset.Update) error
}

type ValuesModel interface {
    IType(iid int32) int32
    Field(update *Updater, iid int32) (string, error)
    Getter(u *Updater, data *dataset.Values, keys []int32) error
    // Model 内部做 iid→field 转换，调 bw.Update(parentModel, converted, uid)
    Setter(u *Updater, bulkWrite dataset.BulkWrite, dirty dataset.Data) error
}
```

业务层实现示例：
```go
// CollectionModel — 每个文档调用一次
func (this *Items) Setter(u *updater.Updater, bw dataset.BulkWrite, _id string, dirty dataset.Update) error {
    bw.Update(this, map[string]any(dirty), _id)
    return nil
}

// DocumentModel
func (this *RoleHandle) Setter(u *updater.Updater, bw dataset.BulkWrite, dirty dataset.Update) error {
    bw.Update(this.New(u), map[string]any(dirty), u.Uid())
    return nil
}

// ValuesModel — 转换 int32 key → dotted field path
func (this *RoleGoods) Setter(u *updater.Updater, bw dataset.BulkWrite, dirty dataset.Data) error {
    update := make(map[string]any)
    for iid, val := range dirty {
        field, _ := this.Field(u, iid)
        update[field] = val
    }
    bw.Update(&Role{}, update, u.Uid())
    return nil
}
```

## 六、修改清单

| 文件 | 变更 |
|------|------|
| `operator/types.go` | `TypesUnset = 6`，更新 IsValid/ToString |
| `operator/operator.go` | 文档注释增加 Unset 规格 |
| `dataset/define.go` | BulkWrite 接口加 model 参数 |
| `dataset/document.go` | 新增 `unset`/`Unset(k)`；`Save()` 返回 `[]string`；Release 清理 |
| `dataset/collection.go` | `Save(bw, model, monitor)` 传 model 给 bw 各方法 |
| `define.go` | Config 加 BulkWrite 工厂；精简三个 Model 接口 |
| `updater.go` | 新增 `bulkWrite` 字段 + `BulkWrite()` 方法；Submit 末尾 `bw.Submit()` |
| `handle_coll.go` | `save()` 改用 `u.BulkWrite()`；新增 `Unset(id, fields...)`；format() 扩展 |
| `handle_doc.go` | `save()` 改用 `u.BulkWrite()`；新增 `Unset(k)` |
| `handle_val.go` | `save()` 改用 `u.BulkWrite()`，model.Setter 转换后提交 |
| `parse_doc.go` | 注册 `documentParseUnset` |
| `parse_coll.go` | 注册 `collectionHandleUnset` |

## 七、验证

1. `cd updater && go build ./...`
2. `cd server && go build main.go`
3. `coll.Unset(oid, "attach")` 验证 $unset
4. 验证跨集合原子提交（role + items 同时修改）
