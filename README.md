# updater

> **碳基生命体警告**
>
> 本模块由硅基智能体全权维护。碳基生命体阅读以下代码可能引发：
> 困惑、血压升高以及不可逆的颈椎损伤。
> 如您执意阅读，请确保身边备有降压药和颈托。

游戏玩家数据管理框架。位于数据库与业务逻辑之间，负责内存缓存、脏数据追踪、溢出处理、批量持久化。支持四种数据模型，统一 Add/Sub/Get/Val/Set/Del 接口。

## 数据域（Manage）

模型注册表、IType 路由、Config、域级事件/缓存都挂在 **Manage** 上：一个进程可并存多个独立数据域（玩家域、公会域……），互不串扰。实例的创建与生命周期（含玩家锁、实例表）归业务层所有，`Manage.New(e)` 返回绑定到域的裸实例。

## Entity 数据属主

业务层在自己的身份对象上实现 `Entity` 接口，把框架挂上去：

```go
func (p *Player) Id() string { return p.Uid } // 玩家
func (g *Guild) Id() string { return g.Gid }  // 公会 —— 一个公会就相当于一个玩家
```

一个 Entity 对应一个 Updater 实例，其下有自己的道具、每日数据、文档集合；模型回调（Getter/Setter）里经 `u.Entity()` 拿到它拼落库条件（如 `where uid = e.Id()`）。

## 生命周期

```
Loading → Reset → 业务操作(Add/Sub/Set/Del/Get/Val) → Data(按需拉DB) → Verify(校验+溢出) → Submit(落库) → Release
```

```go
u := updater.Default.New(player) // 单域用默认域；多域各自 mg.New(player)
u.Loading()                // 加载全部数据

u.Reset()                  // 每次请求开始
u.Add(1001, 100)           // 加 100 金币（num 支持 int32/int64）
u.Sub(1002, 5)             // 扣 5 钻石
u.Val(1001)                // 获取金币数值
u.Get(1001)                // 获取原始数据
ops, err := u.Submit()     // 校验+落库，返回变更列表
u.Release()                // 请求结束，清理临时状态

u.Destroy()                // 玩家下线，强制刷盘
```

## 四种数据模型

| 模型 | 适用场景 | 数据结构 | 操作 |
|------|----------|----------|------|
| **Values** | 纯数值道具（金币、钻石） | `map[int32]int64` | Add/Sub/Set/Del/Resolve/Overflow |
| **Document** | 单文档（玩家信息） | struct 字段级读写 | Add/Sub/Set |
| **Collection** | 文档集合（背包物品） | `map[oid]*Document` | Add/Sub/Set/Del/New |
| **Virtual** | 虚拟层（日常任务） | 委托其他模块数据 | Add/Sub/Set |

## 注册模型

注册表是**数据域（Manage）实例级**的：玩家、公会各自 `NewManage` 一个，互不串扰 ——
同一个 IType ID 可以在两个域各指不同模型，公会域可以有独立的每日数据甚至落库目标。

```go
playerManage := updater.New()
playerManage.Config.IType = func(iid int32) int32 { ... }   // 玩家域的 iid 路由
playerManage.Config.BulkWrite = func(u *updater.Updater) updater.BulkWrite { ... }
playerManage.Register(updater.ParserTypeValues, updater.RAMTypeAlways, &ItemModel{}, itemIType)
playerManage.Register(updater.ParserTypeDocument, updater.RAMTypeMaybe, &PlayerModel{}, playerIType)
playerManage.Register(updater.ParserTypeCollection, updater.RAMTypeAlways, &BagModel{}, equipIType, gemIType)

guildManage := updater.New()                                // 公会域：一个公会就相当于一个玩家
guildManage.Config.BulkWrite = ...                          // 可指向不同的库
guildManage.Register(updater.ParserTypeCollection, updater.RAMTypeMaybe, &GuildDailyModel{}, guildDailyIType)
```

⚠️ 包级 `updater.Register / Config / RegisterGlobalCache / RegisterGlobalEvent /
RegisterGlobalMiddleware / ITypes / Models / NewHandle` 是**默认域 `updater.Default`**
的兼容入口，单域（只管玩家）用法零改动。多域业务别往 Default 里塞非玩家模型。

🔴 `updater.New()` 现在返回 `*Manage`（创建数据域）；实例经 `Default.New(player)`
或 `mg.New(player)` 创建（原包级 `updater.New(player)` 的替代写法）。

🔴 IType ID 仅域内有意义：operator 上只有裸 ID，跨域流转（或发往客户端）时
接收方必须先定位域、再按 IType 分发。

## 内存策略

| RAMType | 说明 |
|---------|------|
| `RAMTypeNone` | 实时读写，每次请求从 DB 拉取，Release 后丢弃 |
| `RAMTypeMaybe` | 按需加载，Loading 时预载，长期驻留内存 |
| `RAMTypeAlways` | 全量内存，Loading 时全量加载，永不丢弃 |

## 溢出处理

道具数量超过 `IMax` 上限时自动触发：

```
Add(金币, 1000) → 当前 9500, 上限 10000
  → 实际增加 500, 溢出 500
  → 如果 IType 实现了 ITypeResolve → Resolve(溢出部分)
  → 否则生成 TypesOverflow 操作（可用于邮件等替代发放）
```

## 灾难熔断

数据库持久化失败时的三级保护：

| 级别 | 行为 |
|------|------|
| `SaveErrorTypeNone` | 忽略，等待下次同步 |
| `SaveErrorTypeNetwork` | 启动数据库监控协程，30s 未恢复升级为灾难 |
| `SaveErrorTypeProgram` | 程序级错误，立即标记灾难 |
| `SaveErrorTypeDisaster` | 拒绝所有写操作，直到 DB 恢复 |

## 事件系统

```go
u.On(updater.EventTypeVerify, func(u *updater.Updater) bool {
    // Verify 前执行，返回 false 移除监听
    return true
})
```

| 事件 | 触发时机 |
|------|----------|
| `EventTypeInit` | Loading 完成后 |
| `EventTypeReset` | 每次 Reset |
| `EventTypeData` | Data 拉取前 |
| `EventTypeVerify` | Verify 校验前（可能多次） |
| `EventTypeSubmit` | Submit 提交前（可能多次） |
| `EventTypeSuccess` | 全部操作成功后 |
| `EventTypeRelease` | Release 释放前 |

## Virtual 前端转发

Virtual 默认不产生 Operator 记录。开启 `Forward(true)` 后，Add/Sub/Set 操作会生成 Operator 并在 Submit 时返回给前端，但 Virtual 本身不做任何数据持久化。

```go
v := u.Virtual("daily")
v.Forward(true)
v.Add(2001, 1)  // 委托给其他模块处理，同时生成 Operator 返回前端
```

## 依赖

| 包 | 版本 |
|----|------|
| `github.com/hwcer/cosgo` | v1.8.1 |
| `github.com/hwcer/logger` | v0.2.8 |
| `go.mongodb.org/mongo-driver/v2` | v2.6.0 |

## 目录结构

```
updater/
├── updater.go          Updater 核心生命周期（Reset/Submit/Release/Destroy）
├── manage.go           Manage 数据域（注册表/Config/域级事件缓存）
├── default.go          默认域 Default + 包级兼容入口
├── define.go           IType 接口 + Status/Keys 工具类
├── model.go            模型元数据（Model/verify）+ Parser 类型
├── statement.go        语句基类（Select/insert/verify/submit）
├── handle.go           Handle 接口定义
├── handle_val.go       Values 实现
├── handle_doc.go       Document 实现
├── handle_coll.go      Collection 实现
├── handle_virtual.go   Virtual 实现（委托模式 + 可选前端转发）
├── parse_val.go        Values 操作解析（Add/Sub/Set/Del）
├── parse_doc.go        Document 操作解析
├── parse_coll.go       Collection 操作解析（含 New/叠加/不叠加）
├── funcs.go            溢出处理（overflow → Resolve）
├── errors.go           错误定义 + 灾难熔断机制
├── events.go           事件系统（Listener/Middleware）
├── process.go          Process 注册表
├── dataset/
│   ├── document.go     Document 数据封装（Get/Set/Save/Clone）
│   ├── collection.go   Collection 数据集（Insert/Update/Delete/BulkWrite）
│   ├── dirty.go        脏数据追踪（Insert/Update/Delete 三态标记）
│   ├── values.go       Values 数据封装（map[int32]int64）
│   ├── update.go       Update map 封装
│   ├── define.go       Model/BulkWrite 接口定义
│   └── utils.go        类型转换工具
└── operator/
    ├── operator.go     Operator 结构体（OType/IID/OID/IType/Value/Result）
    └── types.go        操作类型枚举（Add/Sub/Set/Del/New/Drop/Resolve/Overflow）
```
