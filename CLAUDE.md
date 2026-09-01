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

临时挂载集合（`Mount`）不另设 Parser —— 它不在全局注册表里，`Parser()` 那个返回值也没有任何消费者，报 `ParserTypeCollection` 即可，见下节。

> `Collection`（全局注册句柄）也有一对**内存增删**：`Remove(oid...)` 从内存清掉、
> `Receive(oid, data)` 把已经在手上的文档塞进去。两个都**不碰数据库**，
> 用途是"别处已经查过/刚插入过，别让 Select+Data 照着 oid 再查一遍"。
> ⚠️ Receive 进来的必须是库里真实存在的那条，凭空造一条会让后续 Add/Set 更新到不存在的记录上。

### 临时挂载集合 (Mount)

给"Updater 之外、但要与玩家数据**同批次原子写库**"的数据用：邮件领取标记、兑换码占用、充值订单、临时战斗副本。全部实现在 `handle_mount.go`，设计取舍见 `HANDLER_MOUNT_PLAN.md`。

```go
coll, err := u.Mount(&model.Mail{}, ids...)   // 挂载 + 当场查库(= Select + Data)，幂等
defer u.Unmount(&model.Mail{})                // 标记卸载，真正摘除在 Release 阶段
coll.Update(id, dataset.Update{...})          // 产 operator：verify 写内存，submit 进 bulkWrite
```

`Mount` **不内嵌 `Collection`，整套接口自己实现**，但走的是同一条流水线：改动先变成 operator 入队 → verify 消费（写内存+记脏）→ submit 经 `model.Setter` 落进共享 bulkWrite。

> ⚠️ 曾经内嵌过一版，反复栽在同一件事上：**Go 的方法提升没有虚派发**，`Collection` 内部调到的永远是它自己的方法。于是"覆盖了却不生效"（IType/IMax）、"以为继承了其实语义不同"（Select 走全局 IType 换 oid）、"抄了一半"（submit 漏掉 remove）、"提升出一条必错的路"（`Collection.New` → `mayChange` → ErrITypeNotExist）接连出现。挂载真正需要的只有 **Set / New / Del / Unset** 四种操作，自己写清楚反而短。

与 `Collection` 的分界：

- **不进 IType 体系**：operator 的 `IID` 恒 0，没有溢出检查与自动分解；`u.Add/u.Sub/u.Set/u.Select` 路由不到，只能拿本对象操作。`IMax`/`IType` **压根不实现** —— 那两个已从 `Handle` 接口上摘掉（只服务于 `overflow` 一个调用点，收窄成 `funcs.go` 的 `overflowHandle`）；
- **没有 Add/Sub**：语义是"按 iid 增减持有量"，挂载没有 iid。不存在这两个方法，误用是编译错误；
- **key 只能是文档 `_id`（string）**：没有 iid→oid 的转换规则；
- **自己的 `operator()` 构造**：`Collection.operator` 对 string id 会调 `Config.ParseId` 解析 iid，而挂载的 `_id` 是业务主键（`uid-code`、平台订单号），解析失败会**打脏 `Updater.Error`、整个请求失败**；紧接着的 `mayChange` 还硬要 IType。

**下发客户端与否没有开关**：模型声明了 `ModelIType`（`IType(0)` 非 0）就走通用更新（operator 进 `Updater.dirty` → S2CUpdate），否则不下发 —— 客户端按 IType 分发变更，一条 `IType=0` 的 operator 到对面就是无主数据。不下发时业务用 `Operators()` 自己组协议（读的就是 `statement.cache`，**verify 之后、submit 之前**有效）。

其它要点：

- `Receive(id, data)` 把已经在手上的文档直接塞进内存，跳过查库 —— **挂载同时是这次会话里的一份缓存**（充值的下单 → 核销横跨多次请求就是典型）。`Collection` 也有一对：`Remove` 清掉 / `Receive` 塞进去，都不碰数据库；
- **两档生命周期**（框架零自动清理）：短命 `defer Unmount`；长命（充值、战斗副本）不 Unmount，留给玩家下线兜底。🔴 **三个"释放"不是一回事**：逐请求 `release()` 只清 dirty **保留内存**／`Unmount` 只打标记、Release 阶段才摘除／`destroy()` 是下线刷盘；
- 🔴 `Select` 必须置 `StatusChanged`（`Updater.data()` 开头有闸门），漏了的症状是"Select 了、Data 了、Get 拿到 nil、还不报错"；
- 🔴 `ram` 取 `RAMTypeMaybe`：`release` 保留内存数据，且 `statement.has` 里 `Always && loader` 那条短路**不能命中**（命中后 Select 会跳过每一个 key）。

回归测试在 `handle_mount_test.go`，钉的多数是静默失效。

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