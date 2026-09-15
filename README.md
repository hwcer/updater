# updater

> **碳基生命体警告**
>
> 本模块由硅基智能体全权维护。碳基生命体阅读以下代码可能引发：
> 困惑、血压升高以及不可逆的颈椎损伤。
> 如您执意阅读，请确保身边备有降压药和颈托。

游戏玩家数据管理框架。位于数据库与业务逻辑之间，负责内存缓存、脏数据追踪、溢出处理、批量持久化。支持四种数据模型，统一 Add/Sub/Get/Val/Set/Del 接口。

本仓库现在是**两层结构**：

- **hamster**（`updater/hamster`）：核心版 —— GET/SET/DEL + 脏标记 + 批量落库的文档/集合存储引擎，带主档 Document 与 Entity 主体。颊囊预载（内存缓存）→ 囤货入仓（批量落库）→ 记得囤了什么（脏标记）。不依赖任何道具概念，适合公会管理、邮件、兑换码等场景；
- **updater**（根包）：其上的玩家道具扩展层 —— IType 路由、Add/Sub、溢出分解、Values/Virtual。

拆分设计与取舍见 [HAMSTER_PLAN.md](HAMSTER_PLAN.md)。

## 核心版 hamster 快速上手

```go
// 进程启动：注册模型（主档用 Document，TableOrder 最大先加载）
hamster.RegisterDocument("guild", hamster.RAMTypeAlways, &GuildProfileModel{})      // 公会主档
hamster.RegisterCollection("guild_member", hamster.RAMTypeMaybe, &GuildMemberModel{})

// 一次公会请求
s := hamster.New(guild)            // guild 实现 Id() string
defer s.Release()
s.Loading()                        // 主档先行，成员随后
s.Reset()

prof := s.Document("guild")        // 主档：改公告、加资金（字段级 Add，无溢出）
prof.Set("notice", "今晚8点攻城战")
prof.Add("funds", 100)

members := s.Collection("guild_member")
members.Select(oid)
s.Data()                           // 按需拉取
if mem := members.Document(oid); mem != nil {
    mem.Set("role", 2)
}
members.Add(oid, "contribution", 50)

changes, err := s.Submit()         // 共享 BulkWrite 一次原子入库
```

核心版产出的 operator `IType` 恒 0，默认不进通用更新通道（客户端按 IType 分发，0 即无主数据）；需要变更记录时装自己的接收器或读 `Submit()` 返回值。

## 生命周期

```
Loading → Reset → 业务操作(Add/Sub/Set/Del/Get/Val) → Data(按需拉DB) → Verify(校验+溢出) → Submit(落库) → Release
```

```go
u := updater.New(player)
u.Loading(true)            // 加载全部数据

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

```go
updater.Register(updater.ParserTypeValues, updater.RAMTypeAlways, &ItemModel{}, itemIType)
updater.Register(updater.ParserTypeDocument, updater.RAMTypeMaybe, &PlayerModel{}, playerIType)
updater.Register(updater.ParserTypeCollection, updater.RAMTypeAlways, &BagModel{}, equipIType, gemIType)
```

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
├── updater.go          Updater 扩展层（IType 路由/Add/Sub/委托 hamster.Store）
├── define.go           IType 接口 + Config + 核心版类型别名
├── model.go            模型注册（Register 桥接 hamster）+ 道具路由表
├── statement.go        扩展层 statement 构造（默认接收器 + handleResult 注入）
├── handle.go           Handle 别名 + 道具句柄小接口
├── handle_val.go       Values 实现
├── handle_doc.go       Document 实现
├── handle_coll.go      Collection 实现
├── handle_virtual.go   Virtual 实现（委托模式 + 可选前端转发）
├── handle_mount.go     Mount 薄包装（委托 hamster.Collection）
├── parse_val.go        Values 操作解析（含溢出）
├── parse_doc.go        Document 操作解析（含溢出）
├── parse_coll.go       Collection 操作解析（含 New/叠加/不叠加）
├── funcs.go            溢出处理（overflow → Resolve）
├── errors.go           错误定义 + 灾难熔断机制
├── events.go           事件系统（Listener/Middleware）
├── cache.go / middleware.go  全局缓存/中间件
├── HAMSTER_PLAN.md     核心版拆分设计方案
├── HANDLER_MOUNT_PLAN.md / UNSET_PLAN.md  历史设计文档
├── hamster/            核心版（存储引擎，无道具概念）
│   ├── store.go        Store 生命周期 + Mount
│   ├── statement.go    语句基类（含 handleResult 钩子）
│   ├── handle.go       Handle 接口
│   ├── model.go        注册表（工厂函数 + TableOrder）
│   ├── document.go     核心版 Document（主档承载者）
│   ├── collection.go   核心版 Collection（Mount 泛化 + 字段级 Add/Sub）
│   ├── bulkwrite.go    CollectionBulkWrite 适配器
│   └── errors.go / events.go / cache.go / middleware.go / define.go
├── dataset/
│   ├── document.go     Document 数据封装（Get/Set/Save/Clone）
│   ├── collection.go   Collection 数据集（Insert/Update/Delete/BulkWrite）
│   ├── dirty.go        脏数据追踪（Insert/Update/Delete 三态标记）
│   ├── values.go       Values 数据封装（map[int32]int64）
│   ├── cursor.go       快照分页游标
│   ├── update.go       Update map 封装
│   ├── define.go       Model/BulkWrite 接口定义
│   └── utils.go        类型转换工具
└── operator/
    ├── operator.go     Operator 结构体（OType/IID/OID/IType/Value/Result）
    └── types.go        操作类型枚举（Add/Sub/Set/Del/New/Drop/Resolve/Overflow）
```
