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

### 懒加载（Loading(true)，跨玩家补数据的轻量形态）

`Loading(lazy ...bool) (status Status, err error)`（无参 = 在线档），供 yyds 批量锁给不在内存的玩家
建 Updater（跨玩家补数据）。懒加载（`lazy` 私有字段，仅当 `Loading(true)` 且**尚未
StatusLoaded** 时进入——已加载数据的实例不存在懒加载）。
查询接口 `Status()` / `Loaded()` 取代旧 `Loader()`。Loading 不再带回调——组件创建等收尾由
调用方在 Loading 前自行处理（yyds 在 Loading 前建组件，createComponents 自身幂等）。

**机制：handle 零改动，全部 handle 按 RAMTypeNone 运行**——`statement.GetRAMType()` 在
懒加载时一律返回 None（**不改写 ram 注册值**）。由此自动获得：不预拉、release 丢 dataset
不驻留；请求内照常走完整内存流水线——写 Select 记账 → Submit 收敛的 Data **按需拉触碰的键**
→ Parse 读改写（校验/溢出/惰性重置/Listener/Result 全保留）→ save **绝对值 $set**（幂等
可重试，无 $inc 双记问题）。Document/Collection/Values/Virtual/Mount 全部自动生效。

**上线恢复原始形态（转正）**：无参 Loading 清掉 lazy → `GetRAMType()` 自动回注册值、
各 handle 的 loader 未置（None 期间按需拉不置）→ 幂等分支 `tryLoad` 重新全量预拉 →
`Set(StatusLoaded)` → **补发 `EventTypeLoaded`**。进出都是 Loading 一个入口，ram 字段全程未被改写。

- 🔴 **两个加载事件分工**（`EventTypeInit` 曾是 `EventTypeLoaded` 的兼容别名，已拆成独立值）：

  | 事件 | 何时发 | 挂什么 |
  |---|---|---|
  | `EventTypeInit` | 首次 Loading 建好句柄与域级缓存后，在线/懒加载**各发一次**，转正不重发 | 不依赖数据的准备工作 |
  | `EventTypeLoaded` | **从数据库完整初始化成功时抛出**：首次 Loading、懒加载转正（`tryLoad`）、`Reload()`。按需拉取的局部数据（`RAMTypeNone` / 懒加载下 `Select`+`Data`）与跨天 `ModelReset` 重载不算；懒加载实例不抛 | 依赖整档数据的派生状态：建索引、存量对账。**须能承受重复触发**：`Reload` 会再抛且换掉了整个 dataset，重建前先清空旧派生状态 |

  依赖数据的监听挂错在 Init 上，懒加载转正后派生状态（如项目的物品索引监视器）整个在线期为空。
  域级缓存创建器在 Init 前执行、转正不重建，**不得依赖数据**。
- 🔴 **`statement.loader` 与 dataset 同生死**：release 按运行态 None 丢 dataset 时同步置 false，
  否则「部分句柄已加载 → 进懒加载 → 丢数据 → 转正」不会重拉。
- **Loading 失败可重试**：建句柄与 loading 分两步、每次对全部句柄过一遍（loading 幂等）；失败时
  `u.Error` 清空（错误由返回值交出），否则错误常驻、重试永远失败。进来时已有 `u.Error`（请求中途转正、
  此前的业务错误）则原样返回、不加载也不清。回归用例见 `lazy_promote_test.go`。
- 🔴 **转正失败退回懒加载**：不能留成「非懒加载、未 Loaded」的半成品（扫描不再被拦、半份数据常驻）；
  已加载的句柄在下次 release 按 None 丢数据并复位 loader，之后可再次转正。
- 🔴 **懒加载读之前必须 `Select` + `Data`**：不预拉，读接口只看内存，没拉过的键读到零值。「读出来判定
  再写绝对值」的业务漏了这一步就会用零值覆盖库。框架不替业务记账补拉——要读什么、先拉什么由业务决定。
- 🔴 **扫描类接口懒加载报错**：`Collection.Range/Len/Cursor`、不可叠加道具的 `Collection.Count`、
  `Values.Len/All/Range`、`Document.Range` 靠 Select 拉不全，懒加载下写 `u.Error` 并返回空
  （`Cursor` 返回空数据集上的游标），不静默给半份——遍历里的发信 / 推送 / 直连写库这类管线外副作用，
  请求后续失败也拦不住。可叠加道具的 `Count` 是单文档点查、`Document.Any` 是读主入口，照常可用
  （读哪个先 Select 哪个）。回归用例见 `lazy_read_test.go`。

- **什么时候需要自己实现 writer（Setter）**：判据是「miss 时能不能用**整文档 Insert** 兜底」。
  - **不需要**（默认即可）：Collection 形态——写入口 `Select(OID)` 记账 → Submit 收敛 Data
    按键拉取（写之前一定拉数据）→ Parse 时 miss 且 `model.Upsert=true` →
    `collectionHandleNewItem` 用 itype creator 造整文档 **Insert**（身份字段 iid/uid/OID 由
    creator 填）；Setter 只在 Update 分支被调（行必已存在）。默认 Setter 就是键级 `$set`，
    **无需自拼 `$setOnInsert`**。
  - **需要**：落库形态不走「键级 $set on 已存在文档」的模型——典型是 **Values/Record 形态**
    （一键一行、行身份由业务规则定位如复合 Unique id、用 UpdateOne upsert 落库）。这类模型
    miss 时整文档 Insert 不适用，必须自己实现 Setter 并自带 `$setOnInsert` 身份字段
    （项目 record.go 是范例；cosmo 见载荷含 `$setOnInsert` 自动置 upsert）。
- 懒加载的已知边界（`StatusLoaded` 恒 false）：跨玩家补数据以「Select+Data 读触碰键 + 写绝对值」为限，
  需要复杂判定（溢出分解等）的业务先读够再写；依赖 `EventTypeLoaded` 建的派生状态（如项目的物品索引）
  懒加载下不存在，项目侧自行拦截。

## Architecture

### Core Lifecycle

Every request follows this strict sequence:

```
Loading → Reset → Business ops (Add/Sub/Set/Del) → Data (lazy DB fetch) → Verify (validation + overflow) → Submit (persist) → Release
```

- `Loading` initializes handles based on registered models
- `Reset` starts each request cycle, sets the clock, checks disaster state
- `Submit` runs a convergence loop: `data → verify → submit` repeating until no more changes (capped at 100 iterations to prevent infinite loops)
- `Release` clears per-request state; `Unload` flushes everything to DB on player logout

### 错误状态（Updater.Error）

`Updater.Error` 的类型是 **`*values.Message`**（Code/Data/Args 三元组，实现 `error` 接口）——错误不止有文案，业务码与参数（Args）可直接随回包下发客户端，不必解字符串。

- 写入统一走 `Errorf(...)`（**唯一入口**，返回该错误便于直接抛给上层）：直接委托 `values.Errorf`——传 `*values.Message` 时写时复制收存（Code/Args 保留），传普通 error/字符串则以文案形式收进 Data（错误码归 values 默认码）；
- 五个语义化 helper（`ErrArgsIllegal/ErrItemNotExist/ErrItemNotEnough/ErrITypeNotExist/ErrObjectIdEmpty`）直接返回 `*values.Message`，文案固定、参数只进 Args；
- 🔴 `Error` 是具体指针类型，**不得把 `u.Error` 赋给 `error` 变量再判 nil**——nil 指针装进接口就是非 nil（typed-nil），判空一律直接写 `if u.Error != nil`；
- 业务错误为哨兵形态（9000~9005，`ErrItemNotEnough.Clone(iid, need, has)` 带参）；业务自定义错误码用 `values.Errorf(myCode, updater.ErrXxx)` 换码副本，不改哨兵本体。

### Manage 数据域（实例级注册表）

原包级全局 `modelsRank/modelsDict/itypesDict/Config/globalCache/globalEvents/globalMiddlewares` 全部收进 `Manage`（manage.go）：**一个进程可并存多个独立数据域**（玩家域、公会域……），各域独立注册模型与 IType 路由空间。一个公会就相当于一个玩家 —— 公会域可以有自己的每日数据、IType 编号、BulkWrite 落库目标。

- `Options`（原包级 Config 匿名 struct）：`IMax/IType/ParseId/BulkWrite` 四函数字段，域内持有；`modelIMax/modelIType` 的 Config 回落经 `u.manage.Config`；
- Handle 链的域入口是 `statement.Updater`（`statement.result`、四个 handle 的 `IMax/IType` 都经它反查域表）；
- 🔴 **IType ID 仅域内有意义**：operator 上只有裸 ID，跨域流转（或发往客户端）时接收方必须先定位域再按 IType 分发；
- 🔴 **Manage 只管"域"不管"实例"**：实例（Updater）的创建与生命周期（含玩家锁、实例表）归业务层（yyds/players）所有；`New(e)` 返回绑定到域的裸实例，`updater.New()` 是造域（包级 `Default` 就是它造的默认域，**没有**包级 `New(player)` 了）；
- 域级事件/缓存（`RegisterGlobalEvent/RegisterGlobalCache/RegisterGlobalMiddleware`）只对该域实例生效，emit 顺序"域级 → 实例级"；
- **兼容层**（default.go）：包级 `Register/Config/RegisterGlobalXxx/ITypes/Models/NewHandle` 一行委托 `Default`（包级 `var Config = Default.Config` 是 `*Options` 别名，字段赋值兼容）；进程级 `disaster` 熔断**留全局**（共享库时一个库挂了两个域都该拒服务）。

### Four Data Models (Parser Types)

Each model type has a matching trio: `handle_*.go` (Handle implementation), `parse_*.go` (operator dispatch table), and a `dataset/*` backing store. Virtual is the exception — it has no parse file and no backing store; it delegates all operations to other modules.

| Parser | Handle | Key Type | Backing Store | DB Model Interface |
|--------|--------|----------|---------------|--------------------|
| `ParserTypeValues` | `Values` | `int32` (IID) | `dataset.Values` (`map[int32]int64`) | `valuesModel` |
| `ParserTypeDocument` | `Document` | `string` (field name) | `dataset.Document` (struct wrapper) | `documentModel` |
| `ParserTypeCollection` | `Collection` | `string` (OID) | `dataset.Collection` (map of Documents) | `collectionModel` |
| `ParserTypeVirtual` | `Virtual` | `any` | delegates to another module | `virtualModel` |

临时挂载集合（`Mount`）不另设 Parser —— 它不在所属域的注册表里，`Parser()` 那个返回值也没有任何消费者，报 `ParserTypeCollection` 即可，见下节。

> `Collection`（全局注册句柄）也有一对**内存增删**：`Remove(oid...)` 从内存清掉、
> `Receive(oid, data)` 把已经在手上的文档塞进去。两个都**不碰数据库**，
> 用途是"别处已经查过/刚插入过，别让 Select+Data 照着 oid 再查一遍"。
> ⚠️ Receive 进来的必须是库里真实存在的那条，凭空造一条会让后续 Add/Set 更新到不存在的记录上。

### 临时挂载集合 (Mount)

给"Updater 之外、但要与玩家数据**同批次原子写库**"的数据用：邮件领取标记、兑换码占用、充值订单、临时战斗副本。全部实现在 `handle_mount.go`，设计取舍见 `HANDLER_MOUNT_PLAN.md`。

```go
coll, err := u.Mounts.Load(&model.Mail{}, ids...)    // 挂载 + 当场查库(= Select + Data)，幂等
defer u.Mounts.Remove(&model.Mail{})              // 标记卸载，真正摘除在 Release 阶段
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
- **两档生命周期**（框架零自动清理）：短命 `defer Mounts.Remove`；长命（充值、战斗副本）不 Remove，留给玩家下线兜底。🔴 **三个"释放"不是一回事**：逐请求 `release()` 只清 dirty **保留内存**／`Mounts.Remove` 只打标记、Release 阶段才摘除／`unload()` 是下线刷盘；
- 🔴 `Select` 必须置 `StatusChanged`（`Updater.data()` 开头有闸门），漏了的症状是"Select 了、Data 了、Get 拿到 nil、还不报错"；
- 🔴 `ram` 取 `RAMTypeMaybe`：`release` 保留内存数据，且 `statement.has` 里 `Always && loader` 那条短路**不能命中**（命中后 Select 会跳过每一个 key）。

回归测试在 `handle_mount_test.go`，钉的多数是静默失效。

### Handle interface

公开方法: `Get`, `Val`, `Data`, `IType`, `Select`, `Parser`。
私有生命周期方法: `increase`, `decrease`, `save`, `reset`, `reload`, `loading`, `release`, `unload`, `submit`, `verify`。

`Insert(op, before...)` 不在 Handle 接口中，由 Values/Document/Collection 各自暴露，用于直接注入已封装的 Operator（Virtual 不支持）。

### Updater 公开方法

`Add`, `Sub`, `Get`, `Val`, `Select` — 通过 IID 路由到对应 Handle。`Add/Sub` 的 num 参数为 `any`，在 Updater 层统一通过 `dataset.ParseInt64` 转换为 `int64`。**`Add`/`Sub` 不报错不置 `u.Error`：`iid<=0`、`num<=0` 及路由失败（未注册模型等）一律打 DEBUG 日志（附 uid 与 `callerChain` 完整调用链）后跳过**——策划表数值列大量留空属正常态（如 bong 主线重打 0/0 奖励），上抛会中断整次结算。生产 INFO 级零开销（未开 DEBUG 不捕栈）。没有 `AddErr`/`SubErr` 变体（曾存在，2026-10-01 撤回）。

类型访问器: `Values()`, `Document()`, `Collection()`, `Virtual()` — 通过 name 或 IType ID 获取具体 Handle 实例。

临时挂载: `Mounts` 字段上的 `Load`(取或建+取数) / `Get`(只取) / `Remove`(卸载标记) — 挂载方法不直接挂在 Updater（与 Cache/Events 同款），见「临时挂载集合」。

### 方法排列约定

每个 Handle 实现文件（`handle_*.go`）按以下分组排列：
1. 构造函数 (`New*`)
2. Handle 接口公开方法 (`Get → Val → Data → IType → Select → Parser`)
3. Handle 接口私有方法 (`increase → decrease → save → reset → reload → loading → release → unload → submit → verify`)
4. 类型特有公开方法 (`Add/Sub/Set/Insert/...`)
5. 类型特有私有方法 (`operator/val/format/...`)

## Language

Code comments, error messages, and documentation are in Chinese. Maintain this convention.