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

> `Collection`（全局注册句柄）也有一对**内存增删**：`Remove(oid...)` 从内存清掉、
> `Receive(oid, data)` 把已经在手上的文档塞进去。两个都**不碰数据库**，
> 用途是"别处已经查过/刚插入过，别让 Select+Data 照着 oid 再查一遍"。
> ⚠️ Receive 进来的必须是库里真实存在的那条，凭空造一条会让后续 Add/Set 更新到不存在的记录上。

### 临时挂载集合 (Mount / MountCollection)

给"Updater 之外、但要与玩家数据**同批次原子写库**"的数据用：邮件领取标记、兑换码占用、充值订单、临时战斗副本。设计方案见 `HANDLER_MOUNT_PLAN.md`。

```go
coll, err := u.Mount(&model.Mail{}, ids...)   // 挂载 + 当场查库(= Select + Data)
defer u.Unmount(&model.Mail{})                // 标记卸载，真正摘除在 Release 阶段
coll.Update(id, dataset.Update{...})          // 产 operator，verify 写内存，submit 进 bulkWrite
```

🔴 **它内嵌 `Collection`，走的是与全局句柄完全相同的那条流水线** —— 改动先变成 operator 入队，verify 阶段经 `Collection.Parse` 消费、写内存并记脏，submit 阶段经 `model.Setter` 落进共享 bulkWrite。不是"像"，就是同一套。

- 由此**改数据一律经 operator**，请求失败时 `release` 自动丢弃，内存不会留下库里没有的状态（这正是「取到的指针一律只读」在挂载上的落点）；
- `Forward(bool)` 决定产出的 operator 要不要像普通道具变更一样下发客户端（**默认 false**：临时数据客户端有自己的协议）；`Operators()` 随时手动取（只在本次请求内有效）；
- `Receive(id, data)` 把已经在手上的文档直接塞进内存，跳过查库 —— **挂载同时是这次会话里的一份缓存**。`Collection` 也有一对：`Remove` 清掉 / `Receive` 塞进去，都不碰数据库。

与通用 `Collection` 的四点差别：

1. **不进 IType 路由**：`IType`/`IMax` 恒空、operator 的 `IID` 恒 0；`u.Add/u.Sub/u.Set/u.Select` 一律路由不到，只能拿本对象操作；
2. **不支持 `Add`/`Sub`**：那两条要靠 `ITypeCollection.New` 建对象。调用会记告警返回 nil，不静默；
3. **key 只能是文档 `_id`（string）**：没有 iid→oid 的转换规则；
4. **自己的 `operator()` 构造**：不能复用 `Collection.operator` —— 它对 string id 会调 `Config.ParseId` 解析 iid，而挂载的 `_id` 是业务主键（`uid-code`、平台订单号…），解析失败会**把 `Updater.Error` 打脏、整个请求失败**；紧接着的 `mayChange` 也要求 IType。

模型要实现 `MountModel` = `CollectionModel` + `schema.Tabler`（挂载名取 `TableName()`，与已注册全局模型重名直接报错）。

**两档生命周期**（框架零自动清理）：短命 `defer Unmount`；长命（充值、战斗副本）不 Unmount，留给玩家下线兜底。🔴 **三个"释放"不是一回事**：逐请求 `release()` 只清 dirty **保留内存**／`Unmount` 只打标记、Release 阶段才摘除／`destroy()` 是下线刷盘。

🔴 **`Select` 必须置 `StatusChanged`**（`Updater.data()` 开头有闸门），漏了的症状是"Select 了、Data 了、Get 拿到 nil、还不报错"。回归测试在 `handle_mount_test.go`。

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