# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Test

```bash
go build ./...
go test ./...
go test ./dataset/...       # run tests for dataset subpackage only
go test ./operator/...      # run tests for operator subpackage only
go test -run TestNew ./operator  # run a single test
go vet ./...
```

## What This Module Does

`updater` is a game player data management framework that sits between the database and business logic. It manages in-memory caching, dirty tracking, overflow handling, and batch persistence for player item/resource data.

Module path: `github.com/hwcer/updater`

## Architecture

### Core Lifecycle

Every request follows this strict sequence:

```
Loading → Reset → Business ops (Add/Sub/Set/Del) → Data (lazy DB fetch) → Verify (validation + overflow) → Submit (persist) → Release
```

- `Loading` initializes handles based on registered models
- `Reset` starts each request cycle, sets the clock, checks disaster state
- `Submit` runs a convergence loop: `data → verify → submit` repeating until no more changes (capped at 100 iterations to prevent infinite loops)
- `Release` clears per-request state; `Destroy` flushes everything to DB on player logout

### Four Data Models (Parser Types)

Each model type has a matching trio: `handle_*.go` (Handle implementation), `parse_*.go` (operator dispatch table), and a `dataset/*` backing store. Virtual is the exception — it has no parse file and no backing store; it delegates all operations to other modules.

| Parser | Handle | Key Type | Backing Store | DB Model Interface |
|--------|--------|----------|---------------|--------------------|
| `ParserTypeValues` | `Values` | `int32` (IID) | `dataset.Values` (`map[int32]int64`) | `valuesModel` |
| `ParserTypeDocument` | `Document` | `string` (field name) | `dataset.Document` (struct wrapper) | `documentModel` |
| `ParserTypeCollection` | `Collection` | `string` (OID) | `dataset.Collection` (map of Documents) | `collectionModel` |
| `ParserTypeVirtual` | `Virtual` | `any` | delegates to another module | `virtualModel` |

另有 `ParserTypeMount`（值 `-1`，刻意落在 iota 序列之外）—— 临时挂载集合，不进全局注册表，见下节。

### 临时挂载集合 (Mount / MountCollection)

给"Updater 之外、但要与玩家数据**同批次原子写库**"的数据用：邮件领取标记、兑换码占用、临时战斗副本。设计方案见 `HANDLER_MOUNT_PLAN.md`。

```go
coll, err := u.Mount(&model.Mail{}, ids...) // 挂载 + 当场查库(= Select + Data)
defer u.Unmount(&model.Mail{})              // 标记卸载，真正摘除在 Release 阶段
coll.Update(id, dataset.Update{...})        // 改内存记脏，Submit 时经 model.Setter 落库
```

`Mount` 幂等，重复 Mount 取回同一句柄；带 key 时当场查库（已在内存的 key 自动跳过，
长命句柄反复这么调不会重复查），不带 key 则纯挂载、**不做任何预加载**。

- **复用 `dataset.Collection`（纯容器），不复用 `statement`（算子流水线）** —— 不产 operator、不参与 verify、不进 IType 路由。`u.Add/u.Sub/u.Set/u.Select` **路由不到临时句柄**，只能拿 `*MountCollection` 本身操作；
- 🔴 **key 只能是文档 `_id`（string），不能用数字**：临时集合不进 IType 路由，没有 iid→oid 的转换规则。类型特有方法（`Document`/`Has`/`Update`/`Set`/`Delete`/`Remove`）直接收 `string` 编译期挡住；`Get`/`Val`/`Select` 的 `any` 是 `Handle` 接口锁的，非字符串键运行时告警并跳过；
- `Val` 取数值字段（字段名由 `Field()` 定，默认 `dataset.Fields.VAL`），`Count(iid)` 按 iid 统计、传 0 统计全部。⚠️ **Count 是不完全统计**：临时集合按 key 惰性加载，它数的是内存不是库；`IMax`/`IType` 是纯占位（不参与溢出检查）；
- 模型要实现 `MountModel` = `CollectionModel` + `schema.Tabler`（挂载名取 `TableName()`，与已注册全局模型重名直接报错）；
- **两档生命周期**：短命 `defer Unmount`；长命（战斗副本）每个 handler 开头 `Mount` 幂等取回，业务在全部结束路径显式 `Unmount`。框架零自动清理，兜底是玩家下线 `Destroy` 刷盘；
- 🔴 **三个"释放"不是一回事**：逐请求 `release()` 只清 dirty **保留内存**（长命驻留靠它）／`Unmount` 只打标记、Release 阶段才摘除／`destroy()` 是下线刷盘。混用会让某一档静默失效；
- 🔴 `MountCollection.Select` 必须置 `StatusChanged`（`Updater.data()` 开头有闸门），漏了的症状是"Select 了、Data 了、Get 拿到 nil、还不报错"；
- 🔴 `Unmount` **不当场摘除也不自己刷盘**：`defer` 在 **handler 返回时**执行，框架 `Submit` 排在那之后，当场摘掉这次改动就永久丢失；自己 `save()` 则是绕开 `submit` 另开旁路，短流程与长流程走的路会不一样。打标记 + Release 摘除，两档走同一条路。

回归测试在 `handle_mount_test.go`，钉的全是上面这几条静默失效。

### Key Dispatch Chain

`Updater.Add(iid, num)` → `dataset.ParseInt64(num)` → `Updater.handle(iid)` resolves IID → IType → Model → Handle name → `handle.increase()` → creates `operator.Operator` → enqueued into `statement.operator` via `insert()` → processed in `verify()` via `Parse()` dispatch table. Virtual skips this pipeline — it delegates directly to `model.Add/Sub/Set`, and optionally records operators for frontend forwarding via `Forward(true)`.

Each `parse_*.go` registers its operator handlers in `init()` using a `map[operator.Types]func(...)` dispatch table pattern.

### IType System

IType is the central type-routing mechanism. Every item ID (IID) maps to an IType ID via `Config.IType(iid)`. IType determines which Model/Handle processes the item. Registration order matters — `modelsRank` is sorted by `TableOrder()` descending and controls the loading/processing sequence.

Key interfaces consumers implement:
- `IType` — base, provides `ID()`
- `ITypeCollection` — adds `New()`, `Stacked()`, `GetOID()`（嵌入 `ITypeOID`）
- `ITypeResolve` — overflow auto-decomposition
- `ITypeResult` — custom Result formatting for Operator output
- `ITypeListener` — pre-operation hook for Select pre-fetching

### Memory Strategy (RAMType)

- `RAMTypeNone` — no caching, dataset discarded after Release
- `RAMTypeMaybe` — on-demand loading, stays in memory after Loading
- `RAMTypeAlways` — full load at Loading, never discarded

RAMType affects `statement.has()` logic, `loading()` behavior, and `release()` cleanup.

### Monitor 监控系统

`dataset.Monitor` 接口定义 `Insert(doc)` / `Delete(doc)` 两个回调，在 `Save()` 持久化时触发。`dataset.Monitors`（`map[string]Monitor`）支持多种监控共存，按 key 注册/移除。

注册表的方法挂在 `Monitors` 自身上（`Get` / `Set` / `Remove`，指针接收者以便在 nil map 上就地初始化），`Collection` 只暴露一个入口：

```go
coll.Monitors().Set("items", &itemsIndexesMonitor{...})
coll.Monitors().Remove("items")
```

`Monitors` 自身也实现 `Monitor` 接口（`Insert`/`Delete` 值接收者），`Save()` 靠它扇出到所有注册项。注意 `Remove(key)` 是「摘掉观察者」，`Delete(doc)` 是「有文档被删了」——同一类型上两个语义相反的方法，别混。

#### 🔴 Monitor 的触发位置不可上移，operator 层的任何回调都替代不了它

`Collection.Save()` 内当同一 OID 同时带 insert 与 update 标志时会 `doc = doc.Clone()`（`dataset/collection.go` insert 分支）——**进 `Collection.dataset` 的与 Monitor 收到的都是这个 clone**。

而 `Collection.submit()` 的顺序是 `statement.submit()` → `save()`，所以 statement / operator 层的任何钩子都早于 clone 决策：
- 那时 `op.Result` 存的是 `collectionHandleInsert` 里 `r = append(r, v)` 记下的 **clone 前**对象；
- 回查 `coll.Get(oid)` 也没用，`Get` 优先查 `dirty`，返回的仍是旧对象，`dirty` 要到 `save()` 末尾才置 nil。

「建道具 → 写 Attach」是常见流程（抽卡就走这条），据 operator 建的索引会锁死在插入瞬间的快照上，是**比现状更糟的静默错误**。

回归测试 `dataset/collection_monitor_test.go:TestSaveNotifiesMonitorWithFinalDoc` 钉死了这一点——任何把 Monitor 挪到 operator 层的尝试都会让它立刻变红。

### Cursor 游标 (dataset package)

`dataset.Cursor` 提供 Collection 的内存分页能力。创建时快照当前 dataset 中所有 doc 指针到 `[]*Document` 切片。

- `Collection.Cursor(key)` — 创建或获取已有游标，key 为使用方标记
- `Cursor.Range(offset, size, func(*Document) bool)` — 按偏移量分页遍历
- `Cursor.Paging(page, size, func(*Document) bool)` — 按页码分页遍历（page 从 1 开始）
- `Cursor.Len()` — 快照中的元素总数
- `Cursor.Close(key)` — 移除使用方，所有使用方关闭后释放资源

多个使用方可共享同一 Cursor 实例（引用计数）。Cursor 通过未导出的 `cursorMonitor` 适配器注册到 Monitor（注册键 `collectionMonitorKey` **不导出**，业务无法覆盖框架自己的这份订阅），新元素插入时自动追加到快照尾部。全部使用方 Close 后自动从 Monitor 注销。Release/Reset 不清除 Cursor，Cursor 仅通过 Close 释放。

⚠ 已知缺陷（未修）：`cursorMonitor.Delete` 是空实现，快照不会移除已删文档，分页列表可能翻出已删对象。

### Dirty Tracking (dataset package)

`dataset.Dirty` uses a 3-state bit flag per key (`collOperatorInsert | collOperatorUpdate | collOperatorDelete`). Insert cancels Update. Delete on an inserted item preserves both bits. `Save()` iterates dirty entries and dispatches to `BulkWrite` (Insert/Update/Delete).

`dataset.Document` also tracks dirty fields in an `Update` map — reads check dirty first, then fall back to the underlying struct (via `ModelGet` interface or reflection).

### Disaster Circuit Breaker (errors.go)

DB write failures are classified by `SaveErrorHandle` into 4 levels. Network errors launch a single monitoring goroutine (guarded by `atomic.Bool`) that polls `DatabaseMonitoring()` every second for 30s. If DB doesn't recover, `disaster` atomic flag is set and all writes are rejected until recovery.

### Event System

Two mechanisms:
- **Listeners** (`Events.On`): per-event callbacks, auto-removed when returning `false`
- **Middlewares** (`Events.Use`): named, receive all event types via `Emit()`, cleaned up via `Release()`

Global events (`RegisterGlobalEvent`) are persistent and fire for all Updater instances.

### statement Base

All four Handle types embed `statement`, which manages: `keys` (pending DB fetches), `operator` (pending operations), `cache` (post-verify operators), and the `Select → Data → verify → submit` pipeline. Operators are added via `statement.insert()`. The flow: `statement.operator` → verify(填充 Result) → `statement.cache` → submit(通过 Receiver 分发或默认插入 `Updater.dirty`)。每个 Handle 可通过 `Receiver()` 独立设置操作结果接收器。

### Operator struct (operator package)

操作类型包括: Add/Sub/Set/Del/New/Drop/Resolve/Overflow（Overflow 用于通过替代方式如邮件处理溢出）。

The operation descriptor passed through the entire pipeline. Key fields:
- `OType` (operator.Types) — operation type (Add/Sub/Set/Del/New/Drop/Resolve/Overflow)。修改 OType 应使用 `SetOType()`，非有效类型(Drop/Resolve/Overflow)会自动清除 `FlagDisplay`
- `IID` (int32) — item ID
- `OID` (string) — object ID (Collection only)
- `Flag` (operator.Flag) — 位标志，`FlagUpdate`(更新数据) + `FlagDisplay`(展示给前端)，所有 operator 都会发送到前端，客户端根据 Flag 自行判断是否展示和更新
- `IType` (int32) — item type ID for routing
- `Field` (string, `json:"-"`) — internal temporary field name, not serialized
- `Value` (int64) — numeric operand
- `Result` (any) — final result, type varies by Handle
- `Attach` (`values.Values`，`json:"-"`) — 业务层临时数据，随 Operator 在管线内传递，不序列化给前端；用 `SetAttach/GetAttach` 读写（`SetAttach` 会懒创建 map），`Clone()` 与原对象共享同一 map，`Release()` 时置 nil
- `SetResolve(items map[int32]int64)` — 一键分解：把 OType 置为 `TypesResolve`，并将分解材料存入 `Attach[AttachResolve]`（常量 `operator.AttachResolve = "resolve"`），用 `GetResolve()` 取回。`ITypeResolve.Resolve` 返回的材料由 `funcs.go:overflow` 自动记入（整件分解走 `SetResolve`，只溢出一部分时只记材料、不动 OType——打成 `TypesResolve` 会让整条不落库）

### Handle interface

公开方法: `Get`, `Val`, `Data`, `IType`, `Select`, `Parser`。
私有生命周期方法: `increase`, `decrease`, `save`, `reset`, `reload`, `loading`, `release`, `destroy`, `submit`, `verify`。

`Insert(op, before...)` 不在 Handle 接口中，由 Values/Document/Collection 各自暴露，用于直接注入已封装的 Operator（Virtual 不支持）。

### Updater 公开方法

`Add`, `Sub`, `Get`, `Val`, `Select` — 通过 IID 路由到对应 Handle。`Add/Sub` 的 num 参数为 `any`，在 Updater 层统一通过 `dataset.ParseInt64` 转换为 `int64`。

类型访问器: `Values()`, `Document()`, `Collection()`, `Virtual()` — 通过 name 或 IType ID 获取具体 Handle 实例。

临时挂载: `Mount(MountModel)`, `Unmount(MountModel)`, `Mounted(MountModel)` — 见「临时挂载集合」。

### 方法排列约定

每个 Handle 实现文件（`handle_*.go`）按以下分组排列：
1. 构造函数 (`New*`)
2. Handle 接口公开方法 (`Get → Val → Data → IType → Select → Parser`)
3. Handle 接口私有方法 (`increase → decrease → save → reset → reload → loading → release → destroy → submit → verify`)
4. 类型特有公开方法 (`Add/Sub/Set/Insert/...`)
5. 类型特有私有方法 (`operator/val/format/...`)

## Language

Code comments, error messages, and documentation are in Chinese. Maintain this convention.