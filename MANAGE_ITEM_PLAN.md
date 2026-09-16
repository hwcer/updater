# Manage 域级纯数据开关（PureData）设计方案

> 分支 `refactor` · 待实施 · 前置：Manage 数据域已落地（manage.go/default.go，见 git log c37ad0e、169e5a2）

## 背景与动机

Manage 数据域落地后，公会域可以独立注册模型。但当前**写操作全部要过道具层**：

- Collection：`mayChange` → `ITypeCollection(op.IID)` 解析失败 → 报 `ErrITypeNotExist` 并打脏 `u.Error`（string 键还要先过 `Options.ParseId`，不配会 panic）；
- Document：`operator` → `this.IType(0)` 为 nil → 打脏 Error；
- Values：写操作静默丢弃（仅 Debug 日志）。

即：**不注册 IType 的模型只有读和生命周期可用，写不进去**。公会这类"只管理数据、
没有道具系统"的域（每日数据、成员表、申请表）被迫注册"门牌 IType"才能写。

**定论（作者拍板）**：开关放在 **Manage 域级**，而不是逐模型判定 ——
**只有玩家数据才使用道具模型**，公会等纯数据域整个域就没有道具概念。

## 语义定义

`Manage` 新增导出开关 `PureData bool`（**纯数据域**，默认 `false`）：

- **PureData = false（默认，零值）**：道具模式，现状不变 —— iid 路由（u.Add/Sub/Get/Val/Select）、
  写操作道具层（IType 盖章/监听/OID 生成/ParseId/溢出）全部启用。玩家域（Default）零变化。
- **PureData = true（公会等纯数据域）**：域内没有道具概念 —— 注册不声明 IType，
  Collection/Document 写操作跳过道具层直接走数据路径；`Options.IType/IMax/ParseId`
  三个配置都不用配，只需 BulkWrite。
- **为什么是反向命名**：零值必须落在兼容侧 —— false（零值）= 道具模式，即使有人用
  `&Manage{}` 字面量绕过 New() 构造，语义也和旧行为一致；正向的 `Item bool`（默认 true）
  会让字面量构造静默翻转为纯数据域。
- **时序纪律**：PureData 必须在 Register/Loading 之前设置。
- **键语义**：纯数据 Collection 只支持文档 `_id`（string）显式键（数值键没有 OID 生成规则）；
  缺失文档报 `ErrItemNotExist`（没有 DocFactory，Upsert 改即建同样报此错，业务先 `New()` 建档）；
  **ModelReset 跨天重置照常可用**（公会每日数据靠它）。

```go
guildManage := updater.New()
guildManage.PureData = true                   // 公会域：纯数据，无道具系统
guildManage.Config.BulkWrite = ...            // 唯一必配项
guildManage.Register(updater.ParserTypeCollection, updater.RAMTypeMaybe, &GuildDailyModel{}) // 无 its
```

## 改动点

### 1. manage.go
- `Manage` 加导出字段 `PureData bool`（注释：纯数据域开关，false=道具模式（零值，向后兼容）；
  只有玩家数据才使用道具模型；须在 Register/Loading 前设置）；
- `New()` **不需要**初始化它 —— 零值即默认道具模式，这正是反向命名的目的。

### 2. manage.go（Register 校验，fail fast）
- `m.PureData && len(its) > 0` → 报错"纯数据域不支持注册 IType"
  （modelsDict/itypesDict 天然保持为空）；
- `m.PureData && parser == ParserTypeValues` → 报错"Values 依赖道具模式的 iid 寻址，
  纯数据域请用 Collection/Document/Virtual"。

### 3. handle_coll.go（Collection 三处门控）
- **`mayChange` 顶部**：`this.Updater.manage.PureData` 时跳过道具层 —— 有 OID 则 Select，恒放行
  （op.IType 恒 0）；
- **`operator()` string 分支**：纯数据域跳过 `Config.ParseId`（无 iid 语义，op.IID 恒 0；
  parse 阶段不消费它）；
- **`collectionHandleNewItem` 顶部**：纯数据域报 `ErrItemNotExist(op.OID)` —— 缺失文档的
  Add、Upsert 改即建两条路都汇于此；纯数据集合没有 DocFactory，准确报"文档不存在"而非
  误导性的 `ErrITypeNotExist`；
- 顺手：`Collection.Set` 里三处 `this.Updater.Error = ErrArgsIllegal(...)` 直赋改
  `this.Updater.Errorf(...)`（与全局 Errorf 纪律对齐，消除覆盖已挂起错误的旧模式）。

### 4. handle_doc.go（Document 一处门控）
- `operator()` 的 IType 块：`it == nil` 时仅**道具模式**报错；纯数据域放行
  （op.IType 恒 0，跳过 ITypeOID/ITypeListener）。

### 5. manage.go / updater.go / handle_val.go（iid 入口守卫，防 nil-func panic）
- `handleWithKey`：纯数据域直接返回 nil（u.Add/Sub/Get/Val/Select 惰性无副作用）；
- `Manage.IType`：纯数据域返回 nil（⚠️ IType/IMax 已从 Updater 迁到 Manage，以现状为准）；
  `Manage.IMax` **不需要**门控 —— 纯数据域不配 Options.IMax 时返回 0（无上限），
  且 overflow 对 IID==0 天然跳过，无害；
- `Updater.ParseId(string)`：纯数据域返回 `ErrUnableUseIIDOperation`（现成错误，语义正好）；
- `Values.operator`：纯数据域写操作打脏 `ErrUnableUseIIDOperation`（响亮失败防误用；
  读取不经 operator，不受影响）。

### 6. 文档
- README：公会域示例更新为 `guildManage.PureData = true`；注明 Values 在纯数据域不可注册、
  纯数据 Collection 键为 string `_id`、缺失文档报不存在、ModelReset 可用；
- CLAUDE.md：Manage 节补 `PureData` 开关一条（含门控位置：mayChange / operator /
  collectionHandleNewItem / Document.operator / iid 入口守卫）。

### 7. 测试（manage_test.go 增补，复用 mountModel/mountRow/mountBulk）
- **TestManagePureDataDomain**：`mg := New(); mg.PureData = true`，只配 BulkWrite；注册
  CollectionModel 不带 its：Loading+Reset → Receive 预置 → Set 改字段 → Submit 后
  BulkWrite/Setter 收到、u.Error 为 nil；Set 不存在的键 → Verify 报错（文档不存在）；
  Delete 计数生效；`u.Add(iid, n)` 无 panic 无副作用；`u.ParseId("x")` 返回
  ErrUnableUseIIDOperation；Register 带 its → 报错；Register Values → 报错；
  ModelReset 返回 true 的模型 Reset 后 getter 重新调用（跨天重置可用）；
- **TestPureDataDocument**：注册 DocumentModel（IType 返回 0）不带 its → 字段 Set/Add →
  Submit 落库、u.Error 为 nil；
- **回归**：现有全部测试不变即证明 PureData=false 路径零变化（New() 零值即兼容形态，
  现有测试天然覆盖）。

## 明确不动

- 带 its 注册的既有模型：行为零变化（PureData=false 路径向后兼容）；
- Virtual：本就 IType 可选，不动；
- Mount：本就免 IType，不动；
- Values：纯数据域不可注册（读也要 iid，离开道具模式没有存在意义）。

## 验证

1. `gofmt` / `go vet` / `go test -race -count=1 ./...` 全绿；
2. workspace 全部 11 个模块编译通过；
3. 完成后不自动提交，留工作区待作者确认。
