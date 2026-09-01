# 临时句柄（Mount / Mount）设计实现方案

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

结论：为临时数据做**专用 Mount**，不进 IType/operator 流水线。

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

## 三、复用边界：不复用，自己实现

⚠️ **这一节推翻了前三稿**，每一稿都是被同一件事教育出来的：

| 稿 | 主张 | 为什么不对 |
|---|---|---|
| 一稿 | 不复用 `dataset.Collection`，内存自管 | 排除错了对象 —— 该排除的是流水线，不是容器 |
| 二稿 | 复用容器、不复用 `statement` 流水线 | 不走 operator = `Update` 直接改内存，请求失败回滚不掉 |
| 三稿 | 内嵌 `Collection`，覆盖差异处 | **Go 的方法提升没有虚派发** |
| **定版** | **不内嵌，整套接口自己实现** | 挂载只需要四种操作，写清楚反而短 |

三稿栽的都是同一件事：`Collection` 内部调到的永远是它自己的方法。于是

- "覆盖了却不生效" —— `IType`/`IMax`（`Parse` 内部走 `Collection.IType`）；
- "以为继承了其实语义不同" —— `Select` 对非字符串键走全局 IType 换 oid；
- "抄了一半" —— `submit` 漏掉 `remove` 队列的处理；
- "提升出一条必错的路" —— `Collection.New` → `mayChange` → `ErrITypeNotExist`，
  而对内嵌 `Model` 的业务模型**能编译通过**，运行才炸。

每修一处都要重新推演一遍派发路径。现在每个方法都写在眼前，没有"到底走哪一份"这个问题。

**流水线仍与 `Collection` 相同**（这是要的）：

```
Update/Set/Unset/Delete/Insert
   → operator 入队 (statement.insert)
   → verify: 自己的 parse 消费 → 写内存 + 记脏
   → submit: model.Setter → 共享 bulkWrite
   → Updater.Submit 末尾一次提交
```

复用的只有 `statement`（算子队列）与 `dataset.Collection`（容器），两者都是纯机制、不带 IType 语义。

## 四、终版 API

```go
type MountModel interface {
	CollectionModel
	schema.Tabler
}

// Mount 挂载/取回，keys 非空时当场查库(= Select + Data)。幂等。
// 挂载与取数是两码事:查库失败返回 (句柄, err),句柄已经挂上且可用;
// 唯一返回 nil 的是与已注册全局模型重名。
func (u *Updater) Mount(model MountModel, keys ...string) (*Mount, error)
func (u *Updater) Mounted(model MountModel) *Mount   //只取不挂、不取数
func (u *Updater) Unmount(model MountModel)          //只打标记,Release 阶段才摘除
```

数据操作（**全部产 operator**，返回 `*operator.Operator`，nil 表示出错、原因在 `Updater.Error`）：
`Update` / `Set` / `Unset` / `Delete` / `Insert`。

| 其它 | |
|---|---|
| `Operators()` | 本次请求已 verify、未 submit 的 operator（读的就是 `statement.cache`）。**手动 `u.Verify()` 之后就能读**，submit 之后置空 |
| `Receive(id, data)` | 把已经在手上的文档塞进内存，跳过查库 —— 挂载同时是这次会话的缓存 |
| `Remove(id...)` | 只从内存移除，submit 落库之后才真正摘 |
| `Document` / `Has` / `Len` / `Range` / `Count` / `Field` / `Schema` | 读取 |

## 五、与 Collection 的四点分界

1. **不进 IType 体系**：operator 的 `IID` 恒 0，没有溢出检查与自动分解。
   `IMax`/`IType` **压根不实现** —— 那两个只服务于 `overflow` 一个调用点，
   已从 `Handle` 接口摘掉、收窄成 `funcs.go` 的 `overflowHandle`；
2. **没有 `Add`/`Sub`**：语义是"按 iid 增减持有量"。方法不存在，误用是**编译错误**；
3. **key 只能是文档 `_id`（string）**：没有 iid→oid 的转换规则；
4. 🔴 **自己的 `operator()` 构造**，不能用 `Collection.operator`：
   · 那边对 string id 调 `Config.ParseId` 解析 iid —— 挂载的 `_id` 是业务主键
     （`uid-code`、平台订单号…），解析失败会**打脏 `Updater.Error`、整个请求失败**；
   · 紧接着的 `mayChange` 拿不到 IType 直接返回 `ErrITypeNotExist`。
   仍然走 `format`（字段名统一成 JSName，与 Collection 同口径）。

**下发客户端与否没有开关**：模型声明了 `ModelIType`（`IType(0)` 非 0）就走通用更新，
否则不下发。客户端按 IType 分发变更，一条 `IType=0` 的 operator 到对面就是无主数据 ——
这不是可以随便拨的旋钮，而是"客户端认不认得出"的直接后果。

只认四种 operator：`Set` / `Unset` / `Del` / `New`，其余报错。没有 `Drop`/`Resolve`
（那是溢出分解）。

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

1. **`Updater` 加私有字段 `mounts map[string]*Mount`**，`Mount()` 里懒初始化
   （与 `u.handles` 同款模式）。删除 `Handler` 类型与 `Updater.Handler` 字段
   （`handler.go` 整个文件删掉 —— 未接线的预留口，三仓零调用）；

2. **`Updater.Mount`**：`TableName()` 取名 → 查 mounts 命中即返回 → **查 `modelsRank`
   是否已有同名全局模型，有则返回错误** → `newMount` → `reset()`。
   ⚠️ 重名检查不能省：撞名的话同一张表在一个 Updater 里有两个句柄各写各的，
   是静默的数据竞争；
   带 `keys` 时追加 `Select(keys...)` + `Data()`（当场查库）。
   ⚠️ 查库失败**不回滚挂载** —— 挂载与取数是两码事，句柄已经挂上且可用；

3. 🔴 **`Select` 必须置 `StatusChanged`**：`Updater.data()` 开头有
   `if !u.status.Has(StatusChanged) { return }` 的闸门。挂载的 `Select` 委托给
   `statement.Select`，那里会置位 —— 但**别绕过它自己往 keys 里塞**。
   漏了的症状是"Select 了、Data 了、Get 返回 nil、不报错"，排查方向会全歪到"库里没这条"；

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

   ⚠️ 原来的 `if u.handles == nil { return nil }` 会把 mounts 一并吞掉。
   这一步是机制从死到活的关键：`Data` / `converge` / `Submit` / `Reset` /
   `Release` / `Save` / `Reload` 全部基于 `Handles()`，改这一处即可全覆盖；

5. **顺序：挂载与全局句柄之间没有顺序契约。**
   ⚠️ 一稿写"Submit 时挂载排在全局之后"，那句**既做不到也不需要**：
   `Submit` / `verify` / `Release` 都是**倒序**遍历，追加到尾部恰恰使 mounts 最先执行。
   而它本来就不需要有序 —— 同批次原子性由共享 `u.bulkWrite` 保证，与遍历顺序无关。
   追加到尾部即可，别在这个次序上建立依赖；

6. **`Handle` 接口摘掉 `IMax` / `IType`**：它们只服务于 `overflow` 一个调用点，
   而那三个调用点（Collection/Document/Values 的 `Parse`）传的都是具体类型。
   收窄成 `funcs.go` 的 `overflowHandle`，挂载就不必写两个恒空桩；

7. **`Unmount`**：只置 `Mount.unmount = true`，不落库、不摘除；
   真正的摘除在 `Updater.Release()` 末尾（各句柄 `release()` 之后）统一扫一遍 mounts 删掉。
   理由见第六节。重挂即全新内存、重新 Getter 加载，无脏数据复用；

8. **`Destroy`**：对 mounts 走 `destroy()`（= `save()` 刷盘），随后 `u.mounts = nil`
   ——原先 `Destroy` 只置 `u.handles = nil`，补这一行；

9. **事件链**：参与 `EventTypeVerify / EventTypeSubmit / EventTypeRelease`
   （幂等空转），不参与 `EventTypeInit`（玩家加载期挂载不在场）。

### 实现里几处容易写错的

- **`ram` 取 `RAMTypeMaybe`**：`release` 保留内存数据（只有 `None` 才丢 dataset），
  且 `statement.has` 里 `Always && loader` 那条短路**不能命中** ——
  命中后 `Select` 会认为"全都在内存里"而跳过每一个 key，`Data` 永不执行、`Get` 全返回 nil；
- **`submit` 里 `BulkWrite() == nil` 要单独返回**，不能跟 `save` 失败一起吞：
  前者是配置错误（`Config.BulkWrite` 没配），吞了的话挂载数据永远写不进库、
  现场只有一行 Alert；
- **`verify` 用下标遍历不用 range**：当前四个 parse 分支都不追加 operator，
  但那是实现的性质、不是接口保证；
- **`Remove` 延到 submit 之后再摘**：立即摘会把这条尚未保存的改动一起丢掉；
- **`Insert` 的 `_id` 从对象上取**，别另给一个 id 参数 —— 真正决定落库主键的是对象自己的
  `_id`，两者不一致时库里存一个键、operator 说另一个键，且不报错。

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
- **泛型化**（`Mount[M]`）：接收者级泛型 1.18 即可，但接口不能含泛型
  方法的限制迫使双入口，动主体系，单独评估勿与本次捆绑。bong 侧 typed wrapper
  （cache 层模式）已够用。
