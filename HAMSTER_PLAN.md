# 核心版拆分（hamster 子包）设计方案

> 分支 `hamster` · 第三稿（三稿修订：句柄并入核心，根包仅是 IID/IType 包装 —— 见下）· 本文取代此前所有讨论稿

> **三稿修订（作者定调，破坏性重构解禁）**：①根包是对核心包的封装层 —— `Updater` 内嵌
> `*hamster.Store`、`Mount` 内嵌 `*hamster.Collection`（只加方法；仅 Loading/Destroy 两处
> 覆盖，store 内部从不自调，无虚派发陷阱），取代第二稿"持有+显式委托+错误同步纪律"方案；
> ②道具句柄类型删除（取代 D4 双类型），`u.Document(name)` 等直接返回核心句柄，道具语义经
> 注册期适配器注入核心可选接口（Keyer/OperatorDecorator/ParseDecorator/ModelReset）；
> ③`hamster.Handle` 生命周期方法改回**未导出**（与主干一致）—— 前提是 hamster 是接口的
> 唯一实现者集合；④核心包完整化：四种数据形状 + 字段级 Add/Sub 全部在核心。
> 一至六节的切割线分析、扩展点论证与历史教训仍然有效。

**一句话定位**：本仓库拆成两层 —— **hamster**（GET/SET/DEL + 脏标记 + 批量落库的文档/集合存储引擎，
颊囊预载、囤货入仓：数据预载进内存、攒一批原子入库、脏标记记得囤了什么）与 **updater**（其上的玩家道具扩展层，
IType 路由、Add/Sub、溢出分解都是扩展）。

核心版**不是无主数据的裸 CRUD**：hamster 同样有**主档 Document**（如公会主档）与**身份主体 Entity**，
其余数据以主档/主体为基础加载 —— 只是 `Player`/`Uid` 这两个词不再适用，终审替换为 `Entity`/`Id()`。

方法论已被本仓库验证过两次：

- **overflowHandle 收窄**（funcs.go:14-23）：IMax/IType 只被 overflow 一个调用点用到，从 Handle 接口摘除，
  谁参与溢出谁才实现 —— 本次拆分是同一场手术的放大版；
- **Mount 全尺度原型**（handle_mount.go）：不进 IType 体系、string key、Set/New/Del/Unset 四操作、
  独立 Receiver、共享 bulkWrite —— hamster.Collection 就是它的泛化。

🔴 **全文最高纪律，置顶并贯穿后文**：**Go 的方法提升没有虚派发。扩展层绝不内嵌 hamster 类型再覆盖方法。**
Mount 曾内嵌 Collection，"覆盖了却不生效""以为继承了其实语义不同"接连出事（CLAUDE.md 明文记录三稿教训）。
本文第七节的组合结构以此为一票否决项。

---

## 一、需求矩阵

| 场景 | GET/SET/DEL | 脏标记 | 批量落库 | 主档/Entity | iid 路由 | 溢出分解 | 前端下发 |
|---|---|---|---|---|---|---|---|
| 公会管理（主档+成员+申请+日志） | ✅ | ✅ | ✅ | ✅ | ❌ | ❌ | 不走通用更新 |
| 邮件领取 / 兑换码 / 充值订单 | ✅ | ✅ | ✅ | 可无主档 | ❌ | ❌ | 不走通用更新 |
| 玩家道具背包（现状 updater） | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| 公会资金 / 成员贡献等数值 | ✅（字段级 Add 即可） | ✅ | ✅ | — | ❌ | ❌ | 业务自组 |

归纳：前三列（GET/SET/DEL、脏标记、批量落库）+ 主档/Entity 是**所有场景的公约数**，归核心版；
后三列只有道具场景需要，归扩展层。

**定版结论**：hamster = 生命周期 + statement 流水线 + dataset 容器 + BulkWrite 三层 + 事件/中间件/缓存/灾难熔断
+ 主档 Document + Entity 主体；updater = 在此之上的 IType 注册路由、Add/Sub 便捷 API、溢出分解、Values/Virtual。

## 二、现状问题（通用骨架被道具概念渗透的位置）

逐条给证据，行号可点开核对：

1. **DocumentModel.IType 是必需方法**（handle_doc.go:18-24，注释 handle_doc.go:16-17 解释缘由），
   唯一硬消费点是 `Document.operator()` 的 `it := this.IType(0); if it == nil { err }`（handle_doc.go:357-363）。
   公会主档这类纯文档被迫声明两个道具方法（IType/Field(iid)）才能注册；
2. **Collection.operator 硬编码 `Config.ParseId` + `mayChange`**（handle_coll.go:418-433, 439-458）：
   string 主键的非道具集合（`uid-code`、平台订单号）解析不出 iid 会打脏 `Updater.Error`、整个请求失败。
   Mount 不得不自建一整套 `operator()` 绕开（handle_mount.go:203-221，注释 :199-202）—— 绕开本身就是证据；
3. **`statement.result` 硬引用 `itypesDict`**（statement.go:92-102）：流水线基类直接查道具注册表填 ITypeResult。
   🔴 这是**拆包第一硬阻** —— 无论扩展层怎么组合，statement 与道具注册表都不得互相 import；
4. **Handle 接口带 `Count(iid)`/`increase`/`decrease`**（handle.go:23, 35-36）：不参与 iid 体系的句柄
   （Mount）被迫写恒空桩（handle_mount.go:483-484 注释"不可达"）；
5. **Config 四字段混装**（define.go:7-12）：IMax/IType/ParseId 是道具的，BulkWrite 是通用的；
6. **`New(p Player)` 的用词把框架绑死在玩家域**（updater.go:14-16, 37-39）：Player 接口只有 `Uid() string`
   一个方法，且只被 `Uid()` 打点消费（updater.go:52-54）—— 公会场景下"玩家/UID"语义错位；
7. **operator 协议本身是道具视角**：`IID`/`IType` 字段与 `json:"i"`/`json:"it"` tag（operator/operator.go:99-103）；
8. 反面证据（拆分的可行性）：五份生命周期实现（四个 handle_*.go + handle_mount.go）共享的只有
   statement 值内嵌 + dataset 容器 + CollectionBulkWrite 适配器（handle_coll.go:505-539，Mount 共用 :532）——
   **"句柄独立完整类型"的结构现状已经存在**，本次拆分只是把这条边界画清楚。

## 三、取舍与失败稿对照

| 稿 | 主张 | 为什么不对 |
|---|---|---|
| 一稿 | hamster 提供泛型 baseHandle，扩展层内嵌后覆盖差异方法 | 方法提升无虚派发，Mount 三稿的"覆盖了却不生效"全套教训原样复刻 |
| 二稿 | hamster 句柄全部接口化 + 四个扩展点机制全建（Parse 前置钩子、路由钩子等） | "句柄独立完整类型"下 Parse 钩子与路由钩子**没有消费者** —— 道具句柄的 verify 自己调自己的 overflow（parse_coll.go:23-26 原样），hamster 容器里根本没有溢出概念，机制空转是最贵的噪音 |
| 三稿 | dataset 包也拆进 hamster / dataset.Values 泛型化成 string 键 KV / 身份退化为裸 string | dataset 已是零依赖子包、被两包共享，搬动只添 churn；数值 KV 需求未实际出现（见"明确不做"）；裸 string 丢掉主档/主体语义，切断与 Player 的类型关联，公会/玩家/临时副本无法区分 |
| **定版** | **机制下沉（statement/骨架平移）+ 句柄分层（hamster 两份纯净实现、扩展层五份独立完整实现）+ 扩展点收缩为 2 个 + Entity/主档进核心** | 与 Mount 定版同一逻辑："挂载真正需要的只有四种操作，自己写清楚反而短" |

## 四、关键决策

**D1 实例类型 `hamster.Store`**
hamster 是"存储引擎"，Store 与 dataset.Document/Collection 的"库"语义连贯。
备选 `hamster.Engine`：将来 hamster 要承载非存储调度再改名，成本一个类型别名。

**D2 身份主体 `Entity`，API 冻结的唯一豁免**

```go
// hamster
type Entity interface {
    Id() string //主体唯一标识：玩家 uid / 公会 gid / 临时副本 id
}

func New(e Entity) *Store
func (s *Store) Id() string
```

现状 `Player` 接口只有 `Uid() string`（updater.go:14-16），只被日志打点消费（updater.go:52-54），证据充分。
**豁免的含义**：根包 `updater.New` 同步改为接受 Entity，`Player` 接口与 `Updater.Uid()` 退役 ——
用户的迁移动作 = 把自己结构体的 `Uid() string` 方法改名 `Id() string`，一处。
除这一项外，其余导出 API 全部冻结（见第十一节）。

**D3 hamster.Collection 以 Mount 为蓝本，而非现 Collection 收窄**
Mount 已是验证过的"无 IType Collection"完整实现（string key、无 ParseId、无 mayChange、Select 强校验
string、operator 不设 IID），照它泛化即可；反方向（现 Collection 摘钩子）要拆 GetOID/mayChange 调用链，
`Collection.operator` 会变成"道具开、核心关"的双态代码，长期最脏。
**配套**：hamster.Collection 增加**字段级 Add/Sub**（对 OID+field 的 TypesAdd/TypesSub，无溢出检查）——
成员贡献 `coll.Add(oid, "contribution", 50)`、公会资金在主档上 `doc.Add("funds", 100)`，核心版就够用。
Mount 没有 Add 只是当时的实现范围选择，不是概念禁止（字段级数值增减是 documentParseAdd 已验证的通用语义）。

**D4 Document 双类型，第一版不做单类型合并**
hamster.Document 纯净版 + 扩展层 Document 原样搬家。两者仅 operator() 构造差三行，但"单类型 + 钩子合并"
要在三个位置开洞（operator 填充 / verify 前置 overflow / Result 填充），将来 hamster.Document 演化时钩子点
可能选漏；扩展层独立类型零风险。
合并触发条件写死：迁移步 4 完成后，若扩展层 Document 与 hamster.Document 的生命周期方法逐字节相同度 >90%，
单独立项评估。

**D5 Parser 枚举保留，注册走工厂函数**
hamster 保留 `Parser` 类型（数值不变，作元数据/兼容锚点），但 hamster 内部注册走 `HandleFactory` 工厂函数，
不用 `handles map[Parser]handleFunc` 固定表（model.go:22-29）。工厂注册正是扩展点①的核心。

**D6 主档模式（使用约定，不是新机制）**
hamster.Document 的首要角色即**主档**（公会主档、玩家档案）。Loading 按 `TableOrder`（model.go:31-33 先例）
排序保证主档先于其他模型加载完成，其余模型的 Getter 以 `Entity.Id()` 为键基础派生查询（如
`where guild_id = s.Id()`）。主档先行靠 TableOrder + 事件顺序就成立，**不加任何新机制**；
邮件/兑换码等无主档场景不注册 Document 即可，主档不是强制的。

## 五、终版 API（签名草图）

### 5.1 hamster 包

```go
package hamster

// ---------- 主体 ----------
type Entity interface{ Id() string }

type Store struct { /* now/last/status/handles/mounts/bulkWrite/Cache/Error/Events/Middleware/CreditAllowed
                      —— 自 updater.go:20-35 平移，player 字段换成 entity */ }
func New(e Entity) *Store
func (s *Store) Id() string
func (s *Store) Now() time.Time
func (s *Store) Errorf(format any, args ...any) error

// ---------- 生命周期（与 updater.Updater 同名同序） ----------
func (s *Store) Loading(cb ...Listener) error
func (s *Store) Reset(t ...time.Time) error
func (s *Store) Data() error
func (s *Store) Verify() error
func (s *Store) Submit() ([]*operator.Operator, error) //返回本次变更流水；下发与否由 Receiver 决定（第八节）
func (s *Store) Release()
func (s *Store) Destroy() error
func (s *Store) Save() error
func (s *Store) Reload() error

// ---------- 定位与类型访问器（仅 string 表名，类型系统上排除 iid 路由） ----------
func (s *Store) Document(name string) *Document
func (s *Store) Collection(name string) *Collection
func (s *Store) Mount(m MountModel, keys ...string) (*Collection, error) //照搬 handle_mount.go，幂等
func (s *Store) Unmount(m MountModel)
func (s *Store) Mounted(m MountModel) *Collection

// ---------- 注册 ----------
type HandleFactory func(s *Store, m *Model) Handle
func Register(name string, ram RAMType, model any, factory HandleFactory) error //重名报错
func RegisterDocument(name string, ram RAMType, m DocumentModel) error         //内部用默认工厂
func RegisterCollection(name string, ram RAMType, m CollectionModel) error

// ---------- 模型接口（收窄后） ----------
type DocumentModel interface {
    New(s *Store) any
    Getter(s *Store, data *dataset.Document, keys []string) error
    Setter(s *Store, bw BulkWrite, dirty dataset.Update, unset []string) error
    // IType / Field(iid) 整体摘除 —— 主档模型只需要上面三个方法
}
type CollectionModel interface { /* Upsert/Schema/Getter/Setter 原样，*Updater → *Store */ }
type MountModel interface { CollectionModel; schema.Tabler } //照搬 handle_mount.go:31-34

// ---------- Handle 接口 = 现 Handle（handle.go:19-37）摘掉 Count/increase/decrease ----------
// 🔴 Get/Val/Select 保留的理由（落地时复核）：核心包内部零消费，但消费方是扩展层的
// iid 路由 —— u.Get/u.Val/u.Select 拿到句柄后经接口直调（w.Get(iid) 等），是玩家数据版
// 冻结 API。留在接口上，句柄漏实现是编译错误；摘掉改断言则退化为"路由不到、静默返回 0"。
// 与 Count/increase/decrease 的摘除情形不同：那三个逼 Mount 写恒空桩，这三个所有真句柄都有真实现。
type Handle interface {
    Get(any) any
    Val(any) int64
    Data() error
    Select(keys ...any)
    Parser() Parser
    save() error; reset(); reload() error; loading() error
    release(); destroy() error; submit() error; verify() error
}

// ---------- 句柄公开操作（核心版全部能力面） ----------
// Document：Get/Val/Set/Unset/Add/Sub（字段语义，字段级 Add/Sub 无溢出检查）
// Collection：Get/Set/Unset/Del/New/Insert/Update/Add/Sub（oid 语义）/Select/Receive/Remove/Operators
```

整体平移进 hamster（零 IType 依赖，探索已确认）：`Status`、`RAMType`、`Keys`、`BulkWrite` 接口
（define.go:46-52）、`EventType`/`Listener`/`Events`、`Middlewares`、`Cache`、灾难熔断（errors.go）、
`TableOrder`；statement 改名 `Statement` 导出（`Updater` 字段改 `Store *Store`），加第三个函数字段
`handleResult`（第六节③）。

`operator` 子包**原地不动**，两包共用；`dataset` 子包**原地不动**，原样共享。

### 5.2 updater 扩展包（用户 API 冻结面）

```go
// 用户可见签名不变（唯一豁免见 D2）：Register/Add/Sub/Get/Val/Select/Values/Document/Collection/
// Virtual/Mount/Mounted/Unmount/On/Emit/Operators/Dirty/Testing/Develop/Loader/BulkWrite...
type Updater struct {
    store  *hamster.Store //🔴 持有，不内嵌（第七节）
    entity hamster.Entity //原 player 字段
    itypes map[int32]IType  //itypesDict 收进扩展层
    models map[int32]*Model //iid → 归属模型，按 name 与 hamster 注册表关联
}

// Add/Sub/Get/Val/Select + handleWithKey 路由（updater.go:417-456）整体留在扩展层
```

别名桥接（API 冻结的胶水）：`type Status = hamster.Status`、`type RAMType = hamster.RAMType`、
`type BulkWrite = hamster.BulkWrite`、`type EventType = hamster.EventType` 等一一对应；
`Config` 拆成 `hamster.Config{BulkWrite}` 与 `updater.Config{IMax, IType, ParseId}`，后者桥接转发前者，
用户注册代码不变。

## 六、四个候选扩展点，定版只建两个

先给诚实的总述：在"扩展层句柄独立完整类型"（定版组合）下，②④没有机制需求；③无论如何必须建；
①是注册分层的自然产物。

**① Handle 工厂 —— 必须建**
形态：`Register(name, ram, model, factory HandleFactory)`。扩展层 `updater.Register(ParserTypeCollection, ...)`
内部转 `hamster.Register(name, ram, model, newItemsCollection)`，并在自家 models 表记 IType 归属。
两层注册表的唯一合法接缝：hamster 的注册表只认 name，道具的 iid→model 路由表是扩展层私产。
备选（hamster.Model 加 `Extension any` 元数据槽）—— 拒绝，会让 hamster.Model 长出为道具服务的字段。

**② Parse 前置钩子 —— 不需要（失败稿二稿）**
形态本来是 `WithBeforeParse(f)` 挂 Model、verify 循环消费前调用。但定版下道具 Collection/Values 的
verify 自己调 overflow，hamster 容器没有溢出概念，钩子无调用者。记录于此防止将来再提。

**③ statement Result 填充 —— 必须建，第一硬阻的解法**
现状 statement.go:92-102 硬引用 itypesDict。解法是**第三个函数字段**，与已有的
`handleExist`/`handleReceiver`（statement.go:15-16, 27-28）完全同构：

```go
// hamster/statement.go
type stmHandleResult func(s *Store, op *operator.Operator)

type Statement struct {
    // ...平移
    handleExist    stmHandleExist
    handleReceiver stmHandleReceiver
    handleResult   stmHandleResult //verify 搬运时填充 op.Result；nil 则跳过
}
```

扩展层在句柄构造时注入 `itypesDict[op.IType] → ITypeResult.Result` 的原逻辑。
这是唯一一处"机制必须回调扩展层"的点，函数字段是本仓库已制度化两次的模式。
备选（机制对句柄做接口断言 `if r, ok := stmt.owner.(ResultHook)`）—— 拒绝，机制不得感知句柄具体类型。

**④ 路由便捷方法 —— 无机制，扩展层自有方法**
`Add/Sub/Get/Val/Select` + `handleWithKey`（updater.go:417-456）整体留在扩展层，路由表是扩展层私产。
**核心版没有等价物，也不提供顶层 `s.Set(name, id, field, v)` 糖** —— 顶层按 key 自动路由正是拆分要剥离
的东西，加糖等于从后门请回。真要糖，业务自己写一行 `s.Collection(name).Set(...)`。

## 七、🔴 组合结构（全文核心章）

定版结构 + 三条制度：

1. **updater.Updater 持有 `store *hamster.Store` 字段，不内嵌**。
   内嵌会把 hamster.Store 的 Loading/Reset/Submit 提升到 updater.Updater 上，形成"透传裸核心版"的
   隐式 API；将来任何"想让 Submit 多做一点"的改动都会变成同名覆盖 —— 正是 Mount 三稿的雷区模式。
   显式委托方法一次写清（Loading/Reset/Data/Verify/Submit/Release/Destroy/Save/Reload/On/Develop/Testing，
   约 12 个）。备选"内嵌 + 只加不覆盖"—— 拒绝，那是没有编译器保证的口头纪律。
2. **扩展层句柄独立完整实现 hamster.Handle，不内嵌 hamster.Document/Collection**。
   现状五份生命周期实现的结构照旧（第二节 8），共享的只有 hamster.Statement（值内嵌，与现状一致）+
   dataset 容器 + CollectionBulkWrite 适配器（下沉 hamster 共用）。每个句柄带编译期断言
   `var _ hamster.Handle = (*Collection)(nil)`，杜绝"靠方法提升凑接口、覆盖时静默换语义"。
3. **机制回 call 句柄一律走构造期注入的函数字段**（handleExist/handleReceiver/handleResult 三件套）。
   制度化表述：**结构内嵌（数据 + 纯机制）安全，行为覆盖（方法覆盖）不安全；机制不得断言/调用句柄的其他方法。**

## 八、下发协议

- `operator.Operator` 原样保留（含 IID/IType 字段与 `json:"i"`/`json:"it"` tag）—— operator 是独立子包、
  两层共用，协议数值与序列化**零变更**；
- **IType=0 合法化**：hamster 产出的 operator `IType` 恒 0。语义沿用 Mount 钉死的物理事实
  （handle_mount.go:154-167）：客户端按 IType 分发，0 即无主数据 —— 所以**核心版默认不进通用更新**：
  hamster 句柄默认 Receiver 用 mountDiscard 模式（handle_mount.go:171-175），提升为导出的
  `hamster.DiscardReceiver`；需要变更记录的业务用 `Operators()` / `Submit()` 返回值自组协议；
  扩展层句柄构造时恢复默认收集 Receiver，现有下发行为不变；
- `TypesResolve`/`TypesOverflow`/`TypesDrop` 枚举保留在 operator 包（协议数值稳定性），hamster 的 parse
  分发表不识别它们（报 unknown type，Mount.parse 同款行为，handle_mount.go:622-628 四分支先例）；
- `FlagDisplay` 语义不变。

## 九、生命周期对比

| 阶段 | updater.Updater（现状） | hamster.Store（核心版） | 差异 |
|---|---|---|---|
| Loading | modelsRank 顺序建 Handle，预装 globalCache，Emit Init | 同左；**主档（TableOrder 最前）先行**；无 itypesDict 预热 | 去 IType 预热 |
| Reset | 设时钟、置 StatusSubmit、逐 Handle reset、灾难熔断检查 | 同左 | 无 |
| Data | 闸门 StatusChanged，按 statement.keys 查库 | 同左 | 无 |
| Verify | converge 循环（100 轮上限），溢出检查产生新操作 | 同左；**无溢出时天然单轮收敛** | 多轮机制的动机归道具层，机制保留 |
| Submit | 倒序 h.submit → 共享 bulkWrite.Submit → Emit Success → 返回 dirty | 同左 | 无 |
| Release | Emit、还池、清 dirty、保留内存、摘除标记 unmount 的 mounts | 同左 | 无 |
| Destroy | 倒序 h.destroy 刷盘 → 最后一次 bulkWrite.Submit → 清表 | 同左 | 无 |

🔴 扩展层的每个委托方法 = `u.store.Xxx()` + 自有钩子，**不得偷改时序**（如先 Emit 再调 store.Reset），
一切以现状时序为准。

## 十、接线清单（框架侧全部改动，按序执行）

1. 建 `hamster/` 目录，平移零 IType 文件：statement.go（改名 Statement、`Store` 字段、加 handleResult）、
   events.go、middleware.go、cache.go、errors.go 灾难部分、define.go 的 Status/RAMType/Keys/BulkWrite；
2. Config 拆分：`hamster.Config{BulkWrite}`；`updater.Config{IMax, IType, ParseId}` 保留并桥接转发；
3. Handle 接口在 hamster 定版（摘 Count/increase/decrease，与 funcs.go:14-23 的 overflowHandle 收窄同款手术）；
   `overflowHandle` 与 `overflow()` 留扩展层原样；
4. Entity/主档落位：`hamster.Entity` + `New(e Entity)`；根包 `updater.New` 改收 Entity（Player 退役，
   **唯一 API 豁免**，见 D2）；
5. hamster.Document / hamster.Collection（Mount 蓝本 + 字段级 Add/Sub）/ Mount 入口按第五节落地；
6. 根包四句柄（Values/Document/Collection/Virtual）+ Mount 改为基于 hamster.Statement 的完整实现；
   CollectionBulkWrite 下沉 hamster 共用；
7. `statement.result` 的 itypesDict 查询改为 handleResult 注入（第六节③）；
8. 根包 Register 桥接 hamster.Register + 工厂；modelsDict/itypesDict 收进扩展层私有；
9. `Handles()`/mounts 表逻辑随 Store 平移；根包 Mount API 签名不变、内部委托 store；
10. 事件桥接：`updater.On` → `store.On`（Listener 签名内部适配 `*Updater ⇄ *hamster.Store`）；
11. 编译期断言补齐（第七节制度 2）。

## 十一、迁移路径

**方案甲：API 冻结 + 内部搬迁（v1，推荐 ✅）**
根包全部对外签名不变，**唯一例外 = 身份接口**（Player/Uid → Entity/Id 直接变更，D2 豁免）——
用户的全部迁移动作 = 把 `Uid() string` 方法改名 `Id() string`。
hamster 以别名桥接（`type Status = hamster.Status` 等），导出面文档标注"实验性、v2 前可能微调"。
优点：存量项目一处改名即可升级；hamster 结构正确性先被现有测试钉住。
缺点：根包留一层桥接噪音。

**方案乙：允许破坏性重排（v2.0）**
Mount 返回 `*hamster.Collection`、Register 正式拆分、hamster 一等公民、别名层删除。终态干净，代价是
存量项目全量过一遍。

**推荐：甲进 v1（本分支完成接线清单 1-11），乙留 v2。**
拆分的风险全在"行为等价"而非"签名好看"，先用冻结面把等价性验干净，破坏性整理就变成纯机械操作。

分步落地（每步可编译、现有测试全绿为过关线）：

1. 骨架平移 + 别名桥接（零行为变化）；
2. Handle 接口收窄 + 注册工厂化（hamster 注册表只认 name，dict/itypes 留扩展层）；
3. hamster.Store/Document/Collection 落地 + hamster 侧新测试（把 handle_mount_test.go 的静态失效
   测试点对 hamster 版复钉）；
4. 🔴 最高风险步：扩展层五份句柄回插改造 —— 逐句柄搬迁、逐测试等价
   （五个 *_test.go 除 Player→Entity 适配外不改一行全绿）；
5. 文档回写：README 增"核心版 hamster"章、CLAUDE.md 架构节改两层描述、目录结构树更新。

## 十二、边界场景

1. 同一 name 在 hamster 与扩展层重复注册：hamster.Register 报错（Mount 重名检查同款）；
2. 核心版用户误调扩展层方法：**编译错误** —— hamster.Store 上没有 Add/Sub/ParseId，方法不存在即最响的
   报警（Mount"没有 Add/Sub，误用是编译错误"同款先例）；
3. 扩展层句柄构造时 hamster.Statement.handleResult 未注入：Result 不填充、operator 带原始 Result 落库
   —— 道具句柄工厂内加 Loading 期自检（nil 即 panic 提示接线遗漏，钉静默失效）；
4. Store 与 Updater 生命周期双驱动：扩展层 Updater.Reset 必须转调 store.Reset 且**不得重复 Emit**
   （事件只经一条路，验收标准 8 钉此）；
5. 公会场景多 Store 并发（一个公会一个实例）：与玩家 Updater 同款约束 —— 外部持锁，框架不做并发兜底；
6. IType=0 的 operator 进 dirty（业务显式装了收集 Receiver）后由 Submit 返回：客户端消费方必须判 0
   （第八节口径重申，主数据不分发 0）。

## 十三、验收标准（钉静默失效）

跑一遍看不出来、线上表现为数据莫名其妙的行为，逐条钉住：

1. 根包现有全部 *_test.go **除 Player→Entity 适配外不改一行全绿**（API 冻结的机器证明，步 4 过关线）；
2. hamster.Collection：Select → Data → Get 取到库数据（钉 StatusChanged 闸门 —— 漏置闸门的症状是
   "Select 了、Data 了、Get 拿到 nil、还不报错"，Mount 十条同款）；
3. **主档先行**：TableOrder=0 的主档 Document 在成员 Collection 之前 Loading 完成，成员 Getter 内
   能读到主档内容（钉 D6 使用约定）；
4. 同批次原子性：hamster.Store 上 Document 与 Collection 同请求改动、任一 Getter/Setter 报错时
   两方都不落库（钉共享 bulkWrite）；
5. 扩展层 Add 经路由 → 溢出截断/分解行为与迁移前一致，ITypeResult 填充经 handleResult 钩子不丢
   （钉扩展点③接线）；
6. hamster.Collection 对超限 Add **不做任何溢出处理**（钉"hamster 无溢出"这一负向约束）；
7. hamster 产 operator 的 `IType == 0` 且默认不进 dirty；显式装收集 Receiver 后 Submit 返回非空
   （钉第八节）；
8. 每个扩展层句柄存在 `var _ hamster.Handle = (*Xxx)(nil)` 编译期断言（钉第七节制度 2，断言缺失 CI 失败）；
9. `updater.On(EventTypeVerify, ...)` 经 store.Emit 触发且**只触发一次**（钉边界 4 双驱动）；
10. hamster.Store.Destroy 刷盘 + 清表，重新 New 后无残留状态（钉生命周期兜底）；
11. 最小 DocumentModel（仅实现 New/Getter/Setter，无 IType/Field）注册成功且 Set/Get 往返正确
    （钉接口收窄本身 —— 这一条过了，公会主档就真的不需要道具方法了）。

## 十四、明确不做的事（防止将来重新踩）

- **hamster 不做 iid 路由 / IMax / ParseId / 溢出分解**：Config 三件套与 overflow 是扩展层私产，
  hamster 的代码里出现这些词即设计漂移；
- **不做"内嵌 hamster 类型再覆盖方法"**：永久禁令（第七节）；
- **hamster 不做 Values / Virtual / 数值 KV**：Collection + 字段级 Add/Sub 已覆盖公会资金/成员贡献；
  双 KV 并存的维护成本大于未发生的需求，需求真出现时单独立项（含 dataset.Values 键型问题的整体评估，
  勿与本次捆绑 —— 同 HANDLER_MOUNT_PLAN 对泛型化的处理口径）;
  ✅ **二稿修订（推翻本条）**：Values/Virtual 已以**纯数据结构**形态下沉核心
  （hamster.Values = int32→int64 纯 KV，去掉扩展层"IType 查不到静默丢弃"路径；
  hamster.Virtual = 纯 string 键委托视图 + 请求内中间态缓存，iid→Field 映射留在扩展层）。
  下沉的边界依旧：**道具语义一律不过核心** —— 无 IType 盖键、无溢出、无 iid 路由。
  RegisterValues/RegisterVirtual 入口与 Document/Collection 同款。
- **不动 operator 协议**：字段、json tag、枚举数值一概不改；
- **不动 dataset 包**：原样共享，dataset.Values 的 int32 键耦合留在原地，不为此改名/泛型化；
- **不做顶层 `Store.Set(name, id, field, v)` 语法糖**：路由糖是扩展层身份的一部分（第六节④）；
- **不做泛型化 `Mount[M]`**：单独评估，勿与本次捆绑；
- **第一版不做 Document 单类型 + 钩子合并**：双类型先行，合并触发条件写进 D4（相同度 >90% 再评估）；
- **主档不做强制注册**：无主档场景（邮件、兑换码）合法，D6 只是使用约定；
- **不用 Player/Uid 字样**：Entity/Id 是终审词，hamster 与文档中 Player 仅允许在"现状证据/迁移"语境出现。

## 十五、公会场景示例（验收用例）

业务侧完整用法（注册 → 加载 → 操作 → 提交）：

```go
// ---------- 模型注册（进程启动时） ----------
// 主档：TableOrder 最前，保证先于成员/申请加载
hamster.RegisterDocument("guild", hamster.RAMTypeAlways, &GuildProfileModel{})     // TableOrder() 0
hamster.RegisterCollection("guild_member", hamster.RAMTypeMaybe, &GuildMemberModel{}) // TableOrder() 1
hamster.RegisterCollection("guild_apply", hamster.RAMTypeNone, &GuildApplyModel{})    // TableOrder() 2

// ---------- 会话（一次公会请求） ----------
s := hamster.New(guild) // guild 实现 Id() string
defer s.Release()

if err := s.Loading(); err != nil { return err }
// Loading 内部：主档(TableOrder 0)先到 → 成员/申请的 Getter 以 s.Id() 为键基础
//              （如 where guild_id = ?），可安全引用主档内容

s.Reset(time.Now())

// 主档：改公告、加资金（字段级 Add，无溢出概念）
prof := s.Document("guild")
prof.Set("notice", "今晚8点攻城战")
prof.Add("funds", 100)

// 成员：提名副会（oid 定位）、加贡献
members := s.Collection("guild_member")
if err := members.Select(oid); err != nil { return err }
if err := s.Data(); err != nil { return err } //按需拉取（RAMTypeMaybe）
if mem := members.Get(oid); mem != nil {
    mem.Set("role", RoleOfficer)
}
members.Add(oid, "contribution", 50)

// 申请：未 Select 的数据走 Mount 或 RAMTypeAlways，按场景取舍

// 提交：脏数据经共享 bulkWrite 一次原子入库；返回变更流水（IType 恒 0，默认不进下发通道）
changes, err := s.Submit()
```

对照需求矩阵：主档/Entity ✅、字段级数值 ✅、同批次原子落库 ✅；整段代码没有任何道具概念。
同样的三条模型在扩展层照常工作（注册到 updater、走 IType 路由），两层互不干扰。

---

✅ 本文为第一稿。落地实现与现状冲突时，以第三节取舍表为基准更新本文，而不是绕过它。
