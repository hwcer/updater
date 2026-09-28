# Operator.Field 移除蓝图 v2（Field 降级为派生方法）

> 状态：**待实施** · 前置：updater Manage 数据域已落地 · v1 评估 2026-09，v2 复审 2026-09
>
> 目标：删除 `Operator.Field string`（存储字段），字段名的唯一载体是 `Result` map 的
> key；对外以**派生方法 `Field()`** 提供单字段视图。线上协议逐字节不变
> （Field 本来就是 `json:"-"`）。
>
> v2 相对 v1 的实质改进：① Field 从"删掉"变为"降级为方法"——业务迁移近乎零成本；
> ② 构造期 map 的值语义收敛（消除与 op.Value 双源）；③ 补两个 v1 漏掉的实锤坑
> （Result 判空检测失效 ×2）；④ Clone/分配的取舍写明。

## 可行性依据（v1 结论，仍然成立）

1. **`operator.New` 库外零使用**（areyouok/yyds 全量 grep，2026-09）——签名可自由改。
2. **Add/Sub/Set/Unset 管线上恒为单字段操作**，字段名终局形态就是 Result map 的 key
   ——与其用独立字段中转，不如让 map 从构造期就是载体。
3. **Field 现有用途清单**：handle_doc 构造期 Select / parse_doc ×10 / parse_coll ×4 /
   operator New·Release·Clone / Values 恒空 / 序列化不上网。

## 目标形态

```go
// 现状（Field 是存储字段，parse 期才长出 map；Add/Sub 构造期 Result=nil）
fieldOperator: New(TypesSet, field, 0, v)          // Result = 裸值
parse:         dataset.Set(op.Field, r); op.Result = map{op.Field: r}

// v2（构造期即 map；Field 从存储降级为派生方法）
//  Set:      Result = dataset.Update{field: v}            // 值=目标值
//  Add/Sub:  Result = dataset.Update{field: nil}          // key-only,增量唯一真源仍是 op.Value
//  Unset:    Result = dataset.Update{field: nil}
//  parse:    m := op.Result.(dataset.Update); m[field] = r   // 原地改,分配 1v1
```

### 构造期 map 的值语义（v2 收敛）

pre-parse 的 map 只有一个含义：**目标字段清单**；值仅在 Set 时有意义（目标值）。
Add/Sub 把增量复制进 map 是双源真值，禁止——增量只住 `op.Value`。

## 核心设计：`Field()` 派生方法（v2 新增，替代 v1 的"业务遍历 map"）

```go
// operator.go —— 存储字段删除,方法重生:从 Result 单 key 派生
// Add/Sub/Set/Unset 管线上恒为单字段;非单字段形态(New 的 []any 等)返回 ""
func (op *Operator) Field() (name string, value any) {
	m, ok := op.Result.(dataset.Update)   // ⚠️ operator 不能 import dataset(会成环?)——见"开放问题"
	if !ok || len(m) != 1 {
		return "", nil
	}
	for k, v := range m {
		return k, v
	}
	return "", nil
}
```

- **业务监听器迁移近乎零成本**：`op.Field` → `name, _ := op.Field()`，API 不降级
  （v1 要求业务遍历 map， ergonomics 倒退，废弃）。
- 名字从"状态"变"视图"，单一真源。
- parse 内部同用此方法取 key（len!=1 属非法构造，返回 "" 走报错兜底）。

### 开放问题：import 环

`operator` 包目前零依赖 updater 内部包；`dataset.Update` 若为 `map[string]any`
的别名（`type Update = map[string]any` 或具名类型），Field() 可只按
`map[string]any` 断言避免 import；实施时按 dataset 的实际定义定夺，禁止引入环。

## 实施步骤

### ① Collection（handle_coll.go / parse_coll.go）

- Add/Sub 构造：变参 field 揉进 `Result = Update{field: nil}`（⚠️ 变参是 per-op
  数据，不能换 `coll.Field()` 兜底）；Set/Unset/Del 本就以 Update 承载，不动；
  `format()`（handle_coll.go:473）构造期 map→map 换规范化 map，天然兼容。
- 🔴 **坑一（v2 复审新发现）**：`collectionHandleNewEquip:170` 以
  `if op.Result != nil` 判断"装备是否已生成"——Add 构造期 Result 从 nil 变 map
  后**装备生成被静默跳过**。判空改为"字段值非 nil"
  （`m[field] != nil`）或按 OType 分流，并补测试钉住。
- parse_coll 4 处 `op.Field` → `op.Field()`。

### ② Document（handle_doc.go / parse_doc.go）

- `fieldOperator`：解析 field 后**前置 `statement.Select(field)`**，按目标形态构造
  Update map；`operator()` 里 `this.statement.Select(op.Field)` 删除。
- `parse_doc.go` 4 函数：`name, _ := op.Field()` 取 key；Add/Sub 原地改
  `m[name] = r`；Set 写 `m[name] = v`；Unset 置 nil。
- 🔴 **坑二（同源）**：任何"Result==nil 判操作形态"的代码同理失效——库内已排查
  （parse_coll:76/170 属 New 的 []any 语义，不受影响）；**业务侧实施时必须全量
  grep `op.Result`**，已知 role.go:50 以 `op.Result != nil` 判 Set，改按
  `op.OType == operator.TypesSet`。

### ③ operator 包（operator.go）

- `New(opt, field, value, result)` → `New(opt Types, value int64, result any)`。
- `Release()` / `Clone()`：删 Field 清理行。
- **Clone 对 Update 深拷贝**：全库唯一调用点是 parse_coll:175 装备工厂（冷路径），
  一张单 key 小 map 的拷贝代价可忽略，换来"克隆体 parse 原地改不串原体"的确定性；
  在 Clone 注释写明该契约。
- 新增 `Field()` 方法（见上）。
- 删除 `_ struct{}` 防误用字段；头部"各模式有效字段"表格同步（字段名一律表述为
  "Result map 的 key"）。

### ④ Values（handle_val.go）

Field 恒空串，New 调用删参即可，parse_val 不动（IID 语义）。

### ⑤ 测试同步

- 现有 doc/coll 测试矩阵兜底；
- 新增：构造期 `op.Result.(dataset.Update)` 断言成功且 len==1；
- 新增：NewEquip 判空改造的回归（Add 叠加装备正常生成）；
- 新增：`Field()` 派生方法的单测（Set 带值 / Add 无值 / 非 map 形态返回 ""）。

## 取舍记录（v2 明确写死）

| 决策 | 选择 | 放弃项与理由 |
|---|---|---|
| parse 写 map | 原地改 + Clone 深拷贝 | 每次换新 map：Add/Sub 热路径 2v1 分配，弃 |
| 增量真值 | 只住 op.Value | 复制进构造 map：双源，弃 |
| 对外字段名 API | `Field()` 派生方法 | 业务遍历 map：ergonomics 倒退，弃(v1 方案) |
| 保留 Field 字段 | 见本蓝图 | 16 字节池化字段 vs 心智负担，作者拍板移除 |

## 明确不动

- 线上协议（Field `json:"-"`，Result parse 后形态与现状逐字节一致）；客户端零改动；
- statement.Select 签名与语义；Values/Mount/Virtual 的 parse 路径。

## 验证

1. `gofmt` / `go vet` / `go test -count=1 ./...` 全绿（含 doc/coll 全矩阵 + 新增回归）；
2. workspace 全部模块编译通过；
3. 业务侧迁移（监听器 `op.Field()` 化 + `op.Result` 判空全量复查）后
   areyouok 编译 + 测试全绿；
4. 按惯例：完成后不自动提交，留工作区待作者确认。
