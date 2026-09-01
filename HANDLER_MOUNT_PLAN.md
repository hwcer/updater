# 临时句柄（Mount / MountCollection）设计实现方案

终版设计，一步到位。来源：bong 仓库代码质量审查中对 updater v1.5.1 的分析
（2026-09-01），经对照 updater 现有代码逐条核实后重写（二稿），并按实现中发现的问题
回写修正（三稿，已实现，代码见 mount.go / handle_mount.go / handle_mount_test.go）。
本文取代此前所有讨论稿。

## 一、背景与需求

bong 侧三类场景需要"Updater 之外的临时数据"：

| 场景 | 同批次写库 | 内存驻留 | operator 下发客户端 | IType 路由 |
|---|---|---|---|---|
| mail 领取标记 | ✅ | 请求内 | 可选（客户端重拉即可） | ❌ |
| 兑换码占用 | ✅ | 请求内 | ❌ | ❌ |
| 临时战斗副本 | ✅ | 跨请求 | ❌（客户端有自己的战斗同步） | ❌ |

三行共同点：**要"与玩家数据同批次原子写库 + 按需查库 + 可选内存驻留"，
不要 IType 路由、不要 operator 自动生成**。当前临时做法是 handler 手动
`u.BulkWrite().Update(...)`——批次原子性有了，但每次请求重查库、无内存态，
长命场景（战斗副本）完全无法表达。

结论：为临时数据做**专用 MountCollection**，不进 IType/operator 流水线。

## 二、现状问题（updater v1.5.1）

`handler.go` 的 `Handler map[string]Handle`（注释"用于存储临时Handle"）+
`GetOrCreate(u, name, parser, ram, model)` 是个未接线的预留口：

1. `Handles()` 只遍历 `modelsRank`（全局注册表），`Data()/Submit()` 基于它
   ——临时句柄不会被加载、不会被提交；
2. Handle 生命周期方法（`submit/save/loading/release`）未导出，外部无法驱动；
3. `GetOrCreate` 不校验 model，传错运行期才炸；
4. 命名三胞胎：`Handler`（挂载表）/ `Handles()`（全局列表）/ `Handle`（接口）
   只差大小写与单复数，且 `Handler` 在 Go 生态暗示 http.Handler 式"处理者"；
5. **三仓零调用**（已在 updater / yyds / bong 三处 grep 确认，搜到的
   `GetOrCreate` 全是业务 cache 层的同名方法，与本机制无关）。

由第 5 条：`Handler` 类型与 `Updater.Handler` 字段**直接删除，不留
Deprecated 别名**——零引用的东西留一版兼容，只是让那组命名三胞胎再碍眼一个版本。

## 三、复用边界：复用容器，不复用流水线

这是本方案最容易搞反的一条，先定死：

| 组件 | 复用？ | 理由 |
|---|---|---|
| `dataset.Collection` | ✅ **复用** | 纯数据容器：内存 map + dirty 跟踪 + `Save(CollectionWriter)`。临时数据要的正是这些 |
| `statement` | ❌ 不复用 | 算子流水线（operator/cache/verify/IType）。临时数据一条都不需要 |
| `Collection`（handle_coll.go） | ❌ 不复用 | 它 = statement + dataset，一半是不要的那半 |

⚠️ 一稿写的是"不复用 dataset.Collection，内存自管 `map[string]any` + 脏 key 集合"
——**排除错了对象**。`dataset.Collection` 里没有任何 operator 概念，自管一份等于
把 dirty 跟踪、Update 合并、Document 语义、`Save→Setter` 全部重写一遍，还要重新
趟一遍 `dataset.Update` 已经解决过的合并语义。

复用它顺带解决三件事：

- `Save(w CollectionWriter)` 直接把脏数据经 `model.Setter` 写进共享 bulkWrite
  （与 `CollectionBulkWrite` 同一套适配器，见 handle_coll.go:483）；
- `Release()` 的语义天然正确：**只清 dirty，保留 dataset**——长命句柄跨请求驻留
  由此白送，不需要额外设计；
- 文档访问走 `Document(key)` 直接拿 `*dataset.Document`，按字段读写不用自己拆。
  ⚠️ `Get(any) any` 的签名由 `Handle` 接口锁死，改不了，所以类型化访问只能另开
  `Document()` —— 与通用 `Collection` 同款（handle_coll.go:375）。

## 四、终版 API

```go
// MountModel 临时数据模型。组合 schema.Tabler 是必需的:
// 挂载名取自 TableName(),而 CollectionModel 本身不含它——只声明 CollectionModel
// 就得在 Mount 内部做运行时类型断言,"参数类型即校验"这句话就不成立了。
type MountModel interface {
	CollectionModel
	schema.Tabler
}

// Mount 挂载/取回一个临时数据集合,keys 非空时顺带把这几条**当场查出来**
// (等价 Select(keys...) + Data(),不等框架的 Data 阶段)。
//
// 幂等:同模型重复 Mount 直接返回已挂句柄——长命场景的每个 handler 开头都是这一行,
// 首个请求创建、后续全是复用;已在内存的 key 被 Select 跳过,反复调不会重复查库。
//
// 名字取 model.TableName();与已注册的全局模型重名时直接报错(见第七节 2)。
//
// ⚠️ **挂载与取数是两码事**:查库失败时返回 (句柄, err) —— 句柄已经挂上且完全可用,
// 只是这几条没读回来,重试一次 Select + Data 即可。唯一返回 nil 的是重名,那时压根没挂上。
func (u *Updater) Mount(model MountModel, keys ...any) (*MountCollection, error)

// Unmount 与 Mount 同参对称,内部按 TableName() 摘除。
// ⚠️ 摘除前**必须先刷盘**,理由见第六节。
func (u *Updater) Unmount(model MountModel)

// Mounted 取回已挂载的集合(只取不挂、不取数),未挂载返回 nil。
func (u *Updater) Mounted(model MountModel) *MountCollection
```

⚠️ **`Mount` 的取名规则与 `Register` 并不一致**，这是有意的：`Register` 对非
`schema.Tabler` 的模型有 `schema.Kind(model).Name()` 兜底（model.go:139），
Mount 没有——`MountModel` 强制要求 `TableName()`，兜底路径不存在。
（一稿写"与 Register 取名规则一致"，那句是错的。）

## 五、MountCollection

```go
// MountCollection 临时数据集合:dataset 容器 + Getter 惰性加载 + Submit 时经
// model.Setter 写入共享 bulkWrite。不进 IType/operator 流水线。
type MountCollection struct {
	name    string
	keys    Keys //待拉取的 _id 集合,Data 阶段消费后清空
	model   MountModel
	dataset *dataset.Collection
	Updater *Updater
}
```

用法：

```go
coll, err := u.Mount(&model.Battle{}, battleId) //挂载 + 当场取数
if err != nil {
	return err
}
doc := coll.Document(battleId)             // *dataset.Document(Get 返回 any)
coll.Update(battleId, dataset.Update{...}) // 改内存 + 记脏
// 请求结束由框架 Submit:MountCollection.submit() 走 dataset.Save(writer)
// → model.Setter → 同一个 u.bulkWrite,与全局句柄同批次,一起成败。
```

设计要点：

- **statement / verify / IType 全部空跳过**：不产 operator、不参与条件校验；
  通用入口（`u.Set(oid)` / `u.Add(iid)` / `u.Select(key)`）对临时数据
  **明确不可用**——`handleWithKey` 走 `Config.IType` 路由，临时模型不在
  `modelsDict` 里，压根路由不到。文档直说，不伪装支持；
- **`Parser()` 返回 `ParserTypeMount`**，新增常量 `ParserTypeMount Parser = -1`。
  取负值是为了落在 iota 序列之外：`handles` 工厂表里没有它，
  `Register(ParserTypeMount, ...)` 会在第一道检查就报 `parser unknown`，
  不会误建出一个没有 statement 的全局句柄；
- **业务层要客户端感知时**（如 mail 领取状态）：`u.Dirty(opt)` 是现成公开出口，
  业务自行 `operator.New(...)` 构造后塞入。框架不产 operator，下发通道保留；
- **模型要求**：实现 `MountModel`（= CollectionModel + Tabler）。
  无 ITypeCollection 要求、无 ID 占坑、无 GetOID/Stacked 约定；
- 🔴 **key 只能是文档 `_id`（string）**。不进 IType 路由 → 没有 iid→oid 的转换规则 →
  数字键在这里没有任何可靠含义，格式化成十进制只是凭空发明一套约定，
  与业务真实的 `_id` 对不上时是**静默查不到**。
  类型特有方法（`Document`/`Has`/`Update`/`Set`/`Delete`/`Remove`）直接收 `string`，
  编译期就挡住；`Get`/`Val`/`Select` 的 `any` 是 `Handle` 接口锁死的，
  非字符串键运行时告警并跳过。`Mount(model, keys ...string)` 同理；
- `Val` 取数值字段（字段名由 `Field()` 定，默认 `dataset.Fields.VAL`），
  `Count(iid)` 按 iid 统计、传 0 统计全部 —— ⚠️ **是不完全统计**，
  惰性加载下它数的是内存不是库。`IMax`/`IType` 是纯占位（不参与溢出检查）。

## 六、生命周期（两档，框架零自动清理）

**短命（请求内）**——挂载成功立即 defer：

```go
coll, err := u.Mount(&model.Mail{})
if err != nil {
	return err
}
defer u.Unmount(&model.Mail{})
```

**长命（跨请求，如战斗副本）**——首个请求 Mount，后续请求再次 Mount 幂等取回同一
句柄继续读写，业务在**全部结束路径**（结算/超时/中断）显式 Unmount。

### 三个 release 不是一回事

⚠️ 一稿把它们混成了一个词，是最容易实现错的地方：

| 动作 | 触发时机 | 做什么 | 对长命句柄 |
|---|---|---|---|
| `release()` | **每次请求结束**（`Updater.Release`） | `dataset.Release()`：只清 dirty，**保留内存数据**；清 keys | 安全，数据留着 |
| `Unmount` | 业务显式调用 | **只打标记**，摘除留到 Release | 这才是"卸载"，但推迟生效 |
| `destroy()` | 玩家下线（`Updater.Destroy`） | `save()` **刷盘**，不是 release | 未落库脏数据不丢 |

#### 🔴 Unmount 只打标记，真正摘除在 Release

**一二稿都错了**：短命场景的标准写法是

```go
coll, err := u.Mount(&model.Mail{}, ids...)
defer u.Unmount(&model.Mail{})
```

而 `defer` 在 **handler 返回时**执行，框架的 `Submit` 排在那之后
（`Updater.Verify` 的注释里写着"handle 返回后框架才 Submit"）。
当场摘除的话，这次改动就**永远写不出去，且一声不吭**。

三稿曾改成"Unmount 里自己 `save()` 再摘除"，**那也不对**：那是绕开 `submit` 另开一条
旁路，短流程与长流程走的路不一样，以后往 `submit` 上加的任何东西（监控、埋点、
错误分级）短流程都吃不到。

**定版：`Unmount` 只在句柄上打一个 `unmount` 标记**，句柄照旧留在 mounts 里正常参与
`Data` / `verify` / `submit`，直到 `Updater.Release()` 末尾（`EventTypeRelease` 之后、
各句柄 `release()` 之后）才从 mounts 摘除。长短两档由此走的是同一条路，
`Unmount` 自己不落任何一行库。

- 标记可撤销：同一请求内再次 `Mount` 同一模型会清掉它（业务改主意了）；
- 请求失败时不会有"半落库"：`Submit` 没跑到，脏数据本就不该写，句柄照样在 Release 摘掉。

- **不做任何自动清理**（不挂请求结束钩子）：对短命多余、对长命有害；
- **兜底**：`Updater.Destroy` 里对 mounts 走 `destroy()`（刷盘）而非 `release()`，
  然后 `u.mounts = nil`——泄漏上限钉死在"玩家在线期间"，且下线不丢数据。
  正常路径每次 Submit 都已刷过，这条是兜底；
- 长命句柄残留期间每次 Submit 空转扫过它（无脏数据即 no-op），可接受；
  空闲 TTL 自动卸载明确不做，实测有开销再议。

## 七、接线清单（框架侧全部改动）

1. **`Updater` 加私有字段 `mounts map[string]*MountCollection`**，`Mount` 里懒初始化
   （与 `u.handles` 同款模式）。删除 `Handler` 类型与 `Updater.Handler` 字段
   （handler.go 整个文件删掉）；

2. **`Mount` 内部**：`TableName()` 取名 → 查 mounts 命中即返回 → **查 `modelsRank`
   是否已有同名全局模型，有则返回错误** → 构造 MountCollection → `reset()`。
   ⚠️ 重名检查不能省：撞名的话同一张表在一个 Updater 里有两个句柄各写各的，
   是静默的数据竞争；
   带 `keys` 时在挂载之后追加 `Select(keys...)` + `Data()`（当场查库）。
   ⚠️ 查库失败**不回滚挂载** —— 挂载与取数是两码事，句柄已经挂上且可用，
   业务重试一次 Select + Data 就好；把两件事绑在一起只会让"能不能重试"变得不明确；
   ⚠️ **Mount 不做 `loading()`**。`Collection.loading()` 走的是
   `model.Getter(u, dataset, nil)`——**keys 为 nil 的全量拉取**，战斗副本表全量拉
   不可接受。临时句柄一律惰性：要预热就显式 `Select` + `Data`；

3. **`MountCollection.Select` 必须置 `u.status.Set(StatusChanged)`**。
   ⚠️ 这条漏了机制表面通、实际静默失效：`Updater.data()` 开头
   `if !u.status.Has(StatusChanged) { return }`（updater.go:316），
   而全仓唯一设置该位的地方是 `statement.Select`（statement.go:143）——
   MountCollection 不用 statement，就得自己置。漏了的症状是
   "Select 了、Data 了、Get 返回 nil、不报错"，排查方向会全歪到"库里没这条"；

4. **`Handles()` 末尾追加 mounts**，并修掉开头的 nil 守卫：

   ```go
   func (u *Updater) Handles() (r []Handle) {
       r = make([]Handle, 0, len(modelsRank)+len(u.mounts))
       for _, model := range modelsRank {
           if h := u.handles[model.name]; h != nil { r = append(r, h) }
       }
       for _, h := range u.mounts { r = append(r, h) }
       return
   }
   ```

   ⚠️ 原来的 `if u.handles == nil { return nil }` 会把 mounts 一并吞掉
   （Loading 之前 Mount 属边缘场景，但守卫得跟着改）。
   这一步是机制从死到活的关键：`Data` / `converge` / `Submit` / `Reset` /
   `Release` / `Save` / `Reload` 全部基于 `Handles()`，改这一处即可全覆盖；

5. **顺序：临时句柄与全局句柄之间没有顺序契约。**
   ⚠️ 一稿写"Submit 时临时句柄排在全局之后"，那句**既做不到也不需要**：
   `Submit` / `verify` / `Release` 都是**倒序**遍历（updater.go:355/337/214），
   追加到尾部恰恰使 mounts 最先执行。
   而它本来就不需要有序——临时句柄不共享 IType 路由、不与全局句柄互相产生操作，
   **同批次原子性由共享 `u.bulkWrite` 保证，与遍历顺序无关**。
   所以：追加到尾部即可，并在此写明"倒序阶段里 mounts 先跑"这个事实，
   免得将来有人在上面建立依赖；

6. **`submit()`**：`dataset.Save(&mountBulkWrite{coll: this})`，适配器与
   `CollectionBulkWrite` 同形（Delete/Insert/Setter 三个方法转发到
   `u.BulkWrite()` 与 `model.Setter`）；

7. **`Unmount`**：只置 `MountCollection.unmount = true`，不落库、不摘除；
   真正的摘除在 `Updater.Release()` 末尾（各句柄 `release()` 之后）统一扫一遍 mounts 删掉。
   理由见第六节。重挂即全新内存、重新 Getter 加载，无脏数据复用；

8. **`Destroy`**：对 mounts 走 `destroy()`（= `save()` 刷盘），随后 `u.mounts = nil`
   ——现有 `Destroy` 只置 `u.handles = nil`，要补这一行；

9. **事件链**：参与 `EventTypeVerify / EventTypeSubmit / EventTypeRelease`
   （幂等空转），不参与 `EventTypeInit`（玩家加载期临时句柄不在场）。

## 八、战斗副本的三个边界

1. **同模型多实例并发**：mounts 以 TableName() 为 key，同模型只有一个句柄。
   同一玩家多场并发战斗 → 共用句柄、按文档 key（战斗ID）区分，Select 只拉
   当前战斗的 key；
   ⚠️ **由此 Unmount 是"整表卸载"而非"卸掉这一场"**：两场并发时先结束的那场
   Unmount 会把另一场的内存一起端掉。因为每次 Submit 都写穿了库，这不是数据丢失，
   但后续 `Get` 会返回 nil 直到重新 Select——业务侧看到的是"数据莫名消失"。
   **口径定死：并发多实例时，只有最后一场结束才 Unmount**；业务判断不了"是不是
   最后一场"的话，就别 Unmount，留给下线兜底；

2. **跨玩家共享数据**（多人副本，队友也要读写）：Mount 绑定的是**单个玩家的
   Updater**，本机制不存在"挂玩家/挂房间"的挂载点——A 挂的句柄 B 天生看不见。
   跨玩家数据属于"战斗房间"维度，走独立数据结构，与 Mount 无关；

3. **异常路径残留**：战斗中断没走到 Unmount → 驻留到玩家下线（有兜底、无永久
   泄漏，且下线走 `destroy()` 会刷盘）。业务应在超时分支也保证 Unmount。

## 九、验收用例（bong 侧迁移）

**mail.Submit（短命）**——领取附件 + 标记已领，同批次提交一起成败
（当前用 `u.BulkWrite().Update` 的升级版，附件走全局 Items 句柄不变）：

```go
u := c.Player.Updater
coll, err := u.Mount(&model.Mail{}, ids...) // model.Mail 需补 Getter/Setter(仿 record.go)
if err != nil {
	return err
}
defer u.Unmount(&model.Mail{})

upsert := dataset.Update{}
upsert.Set("submit", 0)
upsert.Set("status", model.MailStatusRead)
for _, id := range ids {
	coll.Update(id, upsert)
}
```

**battle（长命）**——开战请求 Mount，回合/结算请求 Mount 复用，结束路径
（结算、超时、中断）Unmount，并遵守第八节 1 的"最后一场才卸"口径。

bong 在 updater 发版后升依赖再迁移；`exchange` 同 mail 形态（Insert 语义更简）。

## 十、验收标准

实现完成后这四条要有测试钉住，它们对应的都是**静默失效**、跑一遍看不出来：

1. `Select` → `Data` → `Get` 能取到库里的数据（钉第七节 3 的 StatusChanged）；
2. 一次请求内同时改全局句柄与临时句柄，**任一方报错则两方都不落库**
   （钉同批次原子性）；
3. 长命句柄跨两次请求（中间经过完整 `Release` → `Reset`）内存数据仍在
   （钉第六节的 release 语义）；
4. Mount 一个与已注册全局模型重名的临时模型，**返回错误**（钉第七节 2）；
5. `Update` 后直接 `Unmount`（不等框架 Submit），句柄仍在、Unmount 自己不落库，
   随后的 `Submit` 照常把脏数据写进 bulkWrite，`Release` 之后才摘除（钉第六节）；
6. `u.Error` 非空时 Submit 返回错误且 bulkWrite **未提交**（钉失败不落库）；
7. `Destroy` 走刷盘而非 release，且清空挂载表（钉第七节 8）；
8. `Mount(model, keys...)` 当场查库、重复调不重复查、不带 key 不预加载；
9. `Mount` 取数失败时句柄仍挂着且可用，重试 Select + Data 能取到；
10. `Unmount` 后同一请求内再 `Mount`，卸载标记被撤销。

✅ 十条均已实现于 `handle_mount_test.go`。

## 十一、明确不做的事（防止将来重新踩）

- **IType 集成**（Mountable 接口、实例级路由表、ID 占坑、OID 约定）：
  需求矩阵（第一节）三行全都不需要 IType 路由，为它改 `handleWithKey`
  两级查找、要求临时模型实现完整 ITypeCollection，成本倒挂；
- **作用域函数**（`WithCollection` 闭包）：`defer u.Unmount(...)` 原生够用，
  且闭包怎么收具体句柄类型本身就是个别扭问题；
- **自动清理 / 请求结束钩子 / 空闲 TTL**：见第六节；
- **RAMType 入口**：Mount 一律"惰性加载 + 驻留到 Unmount"，不给三档 ram 选择。
  `statement.loading()` 那套（Maybe/Always 才真加载）是为全局句柄的开服预热设计的，
  临时数据没有预热需求；
- **泛型化**（`MountCollection[M]`）：接收者级泛型 1.18 即可，但接口不能含泛型
  方法的限制迫使双入口，动主体系，单独评估勿与本次捆绑。bong 侧 typed wrapper
  （cache 层模式）已够用。
