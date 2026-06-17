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

`dataset.Monitor` 接口定义 `Insert(doc)` / `Delete(doc)` 两个回调，在 `Save()` 持久化时触发。`dataset.Monitors`（`map[string]Monitor`）支持多种监控共存，按 key 注册/移除。通过 `Collection.SetMonitor(key, v)` 注册，`RemoveMonitor(key)` 移除。`Monitors` 自身实现 `Monitor` 接口，遍历调用所有注册的监控。

### Cursor 游标 (dataset package)

`dataset.Cursor` 提供 Collection 的内存分页能力。创建时快照当前 dataset 中所有 doc 指针到 `[]*Document` 切片。

- `Collection.Cursor(key)` — 创建或获取已有游标，key 为使用方标记
- `Cursor.Range(offset, size, func(*Document) bool)` — 按偏移量分页遍历
- `Cursor.Paging(page, size, func(*Document) bool)` — 按页码分页遍历（page 从 1 开始）
- `Cursor.Len()` — 快照中的元素总数
- `Cursor.Close(key)` — 移除使用方，所有使用方关闭后释放资源

多个使用方可共享同一 Cursor 实例（引用计数）。Cursor 通过未导出的 `cursorMonitor` 适配器注册到 Monitor，新元素插入时自动追加到快照尾部。全部使用方 Close 后自动从 Monitor 注销。Release/Reset 不清除 Cursor，Cursor 仅通过 Close 释放。

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

### Handle interface

公开方法: `Get`, `Val`, `Data`, `IType`, `Select`, `Parser`。
私有生命周期方法: `increase`, `decrease`, `save`, `reset`, `reload`, `loading`, `release`, `destroy`, `submit`, `verify`。

`Insert(op, before...)` 不在 Handle 接口中，由 Values/Document/Collection 各自暴露，用于直接注入已封装的 Operator（Virtual 不支持）。

### Updater 公开方法

`Add`, `Sub`, `Get`, `Val`, `Select` — 通过 IID 路由到对应 Handle。`Add/Sub` 的 num 参数为 `any`，在 Updater 层统一通过 `dataset.ParseInt64` 转换为 `int64`。

类型访问器: `Values()`, `Document()`, `Collection()`, `Virtual()` — 通过 name 或 IType ID 获取具体 Handle 实例。

### 方法排列约定

每个 Handle 实现文件（`handle_*.go`）按以下分组排列：
1. 构造函数 (`New*`)
2. Handle 接口公开方法 (`Get → Val → Data → IType → Select → Parser`)
3. Handle 接口私有方法 (`increase → decrease → save → reset → reload → loading → release → destroy → submit → verify`)
4. 类型特有公开方法 (`Add/Sub/Set/Insert/...`)
5. 类型特有私有方法 (`operator/val/format/...`)

## Language

Code comments, error messages, and documentation are in Chinese. Maintain this convention.