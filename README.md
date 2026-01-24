# updater 模块

updater 是一个游戏数据缓存层模块，介于数据库和业务逻辑之间，主要负责缓存数据和持久化数据。

## 功能特点

- **数据缓存**：内存优先，数据库同步，提高数据访问效率
- **玩家级数据淘汰**：登录加载，退出清理，优化内存使用
- **数据库批量操作**：基于 cosmo/mongodb，减少数据库压力
- **错误处理机制**：持久化失败时依赖下次同步，无需即时重试
- **多种数据模式**：支持 Values（数字型键值对）、Document（文档存储）和 Collection（文档集合）三种模式
- **操作类型**：支持 ADD、SUB、SET、DEL、NEW 等多种操作类型

## 模块结构

- **dataset**：数据集模块，包含 Collection、Document、Update 等数据结构
- **operator**：操作对象模块，定义了对数据的各种操作类型和结构
- **handle_***：各种数据模式的处理器，如 handle_val.go、handle_doc.go、handle_coll.go
- **parse_***：各种数据模式的解析器，如 parse_val.go、parse_doc.go、parse_coll.go
- **updater.go**：核心模块，管理玩家数据的生命周期
- **model.go**：模型注册和管理

## 数据模式

### 1. ParserTypeValues（数字型键值对）
- 适用于简单的键值对数据，如玩家的金币、经验等
- 支持 ADD、SUB、SET、DEL 操作

### 2. ParserTypeDocument（文档存储）
- 适用于复杂的文档数据，如玩家的装备、技能等
- 支持 ADD、SUB、SET 操作
- 支持一次更新多个字段

### 3. ParserTypeCollection（文档集合）
- 适用于多个相同类型的文档集合，如玩家的背包、任务等
- 支持 ADD、SUB、SET、DEL、NEW 操作

## 操作对象（Operator）

操作对象用于描述对数据的各种操作，包含以下字段：

- **OID**：object id，用于标识集合中的单个对象
- **IID**：item id，用于标识道具或物品的唯一ID
- **Mod**：物品类型 model ID，用于标识数据模型
- **Type**：操作类型，如 ADD、SUB、SET、DEL、NEW 等
- **Value**：增量值，ADD、SUB、NEW 时有效
- **Result**：最终结果，根据操作类型和数据模型不同而不同

## 使用示例

### 1. 注册模型

```go
// 注册一个 Values 模式的模型
updater.Register(updater.ParserTypeValues, updater.RAMTypeAlways, &PlayerGoldModel{})

// 注册一个 Document 模式的模型
updater.Register(updater.ParserTypeDocument, updater.RAMTypeMaybe, &PlayerEquipModel{})

// 注册一个 Collection 模式的模型
updater.Register(updater.ParserTypeCollection, updater.RAMTypeNone, &PlayerBagModel{})
```

### 2. 创建 Updater

```go
// 创建一个 Updater 实例
u := updater.New(1001) // 1001 是玩家 ID

// 加载玩家数据
u.Load()
```

### 3. 操作数据

#### Values 模式

```go
// 添加金币
u.Add("gold", 100)

// 扣除金币
u.Sub("gold", 50)

// 设置金币
u.Set("gold", 1000)
```

#### Document 模式

```go
// 更新装备
u.Update("equip", dataset.Update{
    "lv": 10,
    "atk": 100,
    "def": 50,
})
```

#### Collection 模式

```go
// 添加道具
u.Add("bag", "item_123", 1)

// 扣除道具
u.Sub("bag", "item_123", 1)

// 删除道具
u.Del("bag", "item_123")
```

### 4. 保存数据

```go
// 保存数据到数据库
u.Save()

// 销毁 Updater，清理内存
u.Destroy()
```

## 配置

### 1. 数据库配置

```go
updater.Config = &updater.Configuration{
    IType: func(iid int32) int32 {
        // 根据物品 ID 获取物品类型
        return itemTypeMap[iid]
    },
    ParseId: func(u *updater.Updater, id string) (int32, error) {
        // 解析物品 ID
        return parseItemId(id)
    },
}
```

### 2. 模型接口

```go
// 物品类型接口
type modelIType interface {
    IType(iid int32) int32
}

// 字段接口
type modelField interface {
    Field(u *updater.Updater, iid int32) (string, error)
}
```

## 错误处理

- **ErrITypeNotExist**：物品类型不存在
- **ErrObjectIdEmpty**：对象 ID 为空
- **ErrUnableUseIIDOperation**：无法使用 IID 操作

## 性能优化

- **内存管理**：根据数据的使用频率，选择合适的 RAM 类型（RAMTypeNone、RAMTypeMaybe、RAMTypeAlways）
- **批量操作**：减少数据库操作次数，提高性能
- **索引优化**：为常用字段添加索引，提高查询速度

## 注意事项

- **数据一致性**：内存数据优先，数据库同步可能有延迟，业务逻辑需要考虑这一点
- **错误处理**：持久化失败时，系统会在下次同步时重试，无需即时处理
- **内存使用**：合理设置 RAM 类型，避免内存溢出
- **操作顺序**：批量操作的顺序可能影响最终结果，需要注意操作顺序
