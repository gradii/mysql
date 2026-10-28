# 显式模型映射：混淆构建约束与历史命名行为

本文是命名策略调查和设计说明，不是实现变更。对照对象为旧 Goravel v1.18.0 / GORM v1.31.2，以及当前 `/Users/dog/Gradii/goravel-framework`。完整 Schema Grammar、旧版本兼容、字符集与时区配置已经确定必须迁移；Docker 和安装工具不在范围内。

**已确认的设计约束：数据库映射不依赖 Go struct 名称。** 程序可能经过压缩或混淆，Go 类型名不是稳定的持久化契约。因此不恢复基于 `reflect.Type.Name()` 的自动表名推导。此前建议恢复自动表名 fallback 的结论撤回；下文旧 GORM 行为只用于说明历史差异，不构成必须恢复的兼容目标。

压缩、剥离调试符号与重命名混淆不是同一种处理，并非都会改变反射名称。设计上仍应将数据库名称与 Go 符号名称分离，最终使用实际发布构建链验证。

**命名策略负责什么**

命名策略决定 Go 模型和字段对应的数据库逻辑名称，例如 `UserProfile -> user_profiles`、`DisplayName -> display_name`。Grammar 负责把已确定的名称编译成数据库 SQL，例如 MySQL 的反引号、表前缀和 schema-qualified 标识符。Processor 负责处理执行结果和元数据，不应该再次改变列名。

在本项目中，framework 的模型映射层应读取显式、稳定的数据库名称。MySQL 驱动只负责配置和 SQL 标识符编译，不根据 Go 类型名或字段名猜测表列名称。是否保留某个默认名称生成规则，应看其输入是否为稳定的数据库元数据，而不是是否使用了字符串转换函数。

**旧驱动公开的四个参数：历史行为**

旧 `mysql/config.go:57-68` 读取这四项，旧 framework 的 `database/driver/gorm.go:63-68` 将它们传给 `gorm/schema.NamingStrategy`，所以它们以前确实参与执行。

| 配置 | 旧行为 | 例子与边界 |
| --- | --- | --- |
| `prefix` | 给自动推导的表名直接拼接前缀；不修改列名。 | `pre_` + `user_profiles` = `pre_user_profiles`。前缀本身不经过替换、不自动加下划线。显式表名有单独规则，见后文。 |
| `singular` | 为 true 时跳过自动表名的复数化。 | `UserProfile` 默认得到 `user_profiles`，true 时得到 `user_profile`。它不是强制把任何已有复数名称改成单数。 |
| `no_lower_case` | 跳过整个默认 snake_case/小写转换步骤。 | `DisplayName` 保留为 `DisplayName`，而不是 `display_name`。它不会关闭表名复数化，也不会关闭 NameReplacer。 |
| `name_replacer` | 在默认 snake_case 转换前调用 `Replace(string) string`。 | `strings.NewReplacer("User", "Account")` 将 `UserID` 映射成 `account_id`。它不是针对最终 SQL 字符串的替换。 |

默认字段名的旧处理顺序为：

```text
Go 字段名
  -> NameReplacer（如果配置）
  -> snake_case / 小写（no_lower_case=false 时）
  -> 数据库列名
```

自动表名在这个过程后，按 `singular` 决定是否复数化，再拼接 `prefix`。显式 `gorm:"column:..."` 不走字段名推导。旧 GORM 还有一个边界：替换器返回空串时，直接使用原始输入，不继续 snake_case。

**可复现的组合例子**

模型名为 `UserProfile`，其中有字段 `NameCID`。替换器为 `strings.NewReplacer("User", "Account", "CID", "Cid")`。以下结果已经用旧依赖的 GORM schema 解析验证：

| 设置 | 自动表名 | `NameCID` 的列名 |
| --- | --- | --- |
| 默认 | `user_profiles` | `name_c_id` |
| 设置替换器 | `account_profiles` | `name_cid` |
| 替换器 + `prefix=pre_` + `singular=true` | `pre_account_profile` | `name_cid` |
| 替换器 + `prefix=pre_` + `no_lower_case=true` | `pre_AccountProfiles` | `NameCid` |
| 替换器 + `prefix=pre_` + 两个 bool 都为 true | `pre_AccountProfile` | `NameCid` |

另一个更直观的字段例子：`UserID` 使用上述替换器得到 `account_id`，同时打开 `no_lower_case` 后得到 `AccountID`。`gorm:"column:legacy_user_id"` 始终采用 `legacy_user_id`。

**当前框架与旧行为的差异：不代表都应恢复**

| 路径 | 当前行为 | 影响 |
| --- | --- | --- |
| ORM 字段映射，`database/gorm/model_schema.go:133` | 非空 `gorm` column 标签优先，否则直接调用 `str.Snake`。函数没有接收连接命名策略。 | `NameReplacer`、`NoLowerCase` 不必为了旧行为恢复；如果构建会重命名字段，应取消该默认字段名依赖，采用显式列映射。 |
| ORM 行扫描，`database/gorm/row.go` | 使用独立的默认 snake mapper。 | 只修复 SQL 写入名称还不够，读取映射也必须使用同一策略。 |
| DB 结构体映射，`database/db/builder.go:16`、`row.go:8` | 默认全局 `NameMapper` 为 snake_case；显式 `db` 标签优先。 | 与 ORM 的 `gorm` 标签是两套规则。不能为了 ORM 兼容而悄悄改变 DB facade 的既有标签语义。 |
| ORM 表名，`database/gorm/relation.go:566` | 使用声明的 `TableName()`；没有声明时返回 `OrmQueryTableNotSet`，显式 `Table(...)` 的路径除外。 | 符合本项目不依赖 Go 类型名的方向，应保留，不恢复旧自动表名 fallback。 |
| 模型迁移生成，`database/migration/model_mapper.go:145/189` | 表名使用 `TableName()`，否则退回结构体名的 snake_case；列名按显式标签或 snake_case。 | 未声明表名时可能生成 `user_profile`，而 ORM 又拒绝该模型；迁移生成与 ORM 规则不同。 |
| 关联推导，`database/gorm/relation.go:305/353` | 根据已声明表名做 singular 化，生成外键和中间表名；显式关系配置优先。 | 这是关联约定，与连接的 `singular` 开关不是一回事。不能简单用该开关替代关联命名逻辑。 |

即使没有配置替换器，默认 snake_case 算法也不完全相同。旧 GORM 把 `NameCID` 映射为 `name_c_id`；当前 `str.Snake` 的实际结果是 `name_cid`。这说明旧数据库迁移时需要核对并固定显式列名，而不是为本项目恢复依赖 Go 符号名称的推导算法。

旧 Goravel 的 `Create`、`Get`、`ToSql().Create/Get` 接受普通 struct，查询构造没有额外要求 `TableName()`。本项目有意保留显式表名要求，这是有设计依据的公开 API 差异。

**显式名称、关联和索引的边界**

1. **字段标签优先。** ORM 的非空 `gorm:"column:LegacyCode"` 应固定列名。DB facade 的 `db:"LegacyCode"` 固定它自己的映射。替换器不应改写显式数据库列名。
2. **表名有三条旧路径。** 自动表名使用完整策略；`TableName() == "LegacyProfiles"` 在旧 GORM 中直接采用，不自动附加前缀；旧 Goravel `Query.Table("audit_logs")` 自己附加配置前缀，再交给 GORM，绕过替换、snake 和复数化。当前框架将前缀放到 grammar/table 编译路径处理，迁移时必须逐入口确定前缀语义，避免重复添加或查询不同的表。
3. **查询字符串本来就是数据库名称。** `Where("user_id", ...)`、显式列列表、原始 SQL、别名和 schema-qualified 名称，不应该当作 Go 字段名再次送进 NameReplacer。
4. **关联访问名是 Go/API 名称。** `With("Customer")` 或当前声明式关系中的 `Customer` 不应被替换为别的名字。命名策略最多参与隐式数据库键名的推导；明确配置的 `ForeignKey`、`OwnerKey`、pivot table/key 应优先。新的声明式关系系统无需为了命名兼容重新引入 GORM 关系标签解析。
5. **Schema 索引命名另有现成规则。** Goravel Blueprint 使用 `<prefix><table>_<columns>_index` 等默认名称，显式索引名优先。不能因为 GORM 内部也有 `IndexName`、`RelationshipFKName`，就把它的 `idx_*`、`fk_*` 规则当作 Goravel Schema 的兼容目标。完整 Schema Grammar 应保留当前 Blueprint 的命名约定。
6. **模型迁移生成也必须读取显式映射。** 旧生成器使用空的 `schema.NamingStrategy{}`；当前生成器仍有结构体名/字段名 fallback。目标是与运行时共享稳定的表列描述，映射缺失时明确报错，不按混淆后的类型名生成迁移。

**建议的 framework 设计**

目标改为与 Go 符号名称解耦的显式模型映射。不要新增一个用于恢复 GORM 默认推导的 NamingStrategy。

| 项目 | 目标规则 |
| --- | --- |
| 表名 | 使用 `TableName()` 返回的稳定字符串、明确的 `Table(...)`，或显式模型描述；禁止类型名 fallback。接口方法应通过接口调用，不通过字符串 `MethodByName` 寻找。 |
| 列名 | 混淆兼容路径使用明确的 `gorm:"column:..."`、DB facade 的 `db:"..."` 或等价字段描述。现有标签名称不意味着需要 GORM 库。 |
| `singular`、`name_replacer`、`no_lower_case` | 不恢复依赖 Go 符号的用途；从新运行时契约去除，或在过渡期明确弃用，不能保留成无效配置。 |
| `prefix` | 保留。输入是稳定数据库表名，是连接配置；只在一个边界附加一次。 |
| 索引/外键名称 | 可以根据显式数据库表列名生成默认名称，也支持显式名称；这种规则不依赖 Go struct 名称。 |
| 关联默认键 | 从稳定的显式表名推导的规则可以保留；明确的 ForeignKey/OwnerKey/pivot 配置优先。不得把 Go 字段名作为未声明的持久化标识。 |

例如，下面的模型即使 Go 类型名和字段名改变，数据库映射也有明确来源：

```go
type UserRecord struct {
    ID   uint   `gorm:"column:id;primaryKey" db:"id"`
    Name string `gorm:"column:display_name" db:"display_name"`
}

func (UserRecord) TableName() string { return "users" }
```

这个例子要求构建链保留所需标签和接口语义；它仅说明表列映射，不证明整个 ORM 已经支持任意混淆工具。

- **扫描与写入一致。** ORM 的列解析、SQL 写入、主键查找、事件属性映射、软删除列和结果扫描共享显式列描述。需要审计当前对 `ID`、`CreatedAt` 等 Go 字段名称的特殊判断，优先使用明确标签/语义标记。
- **关联名仍需审计。** 当前 `eager_loader.go:1100` 确实使用 `FieldByName(fieldName)` 回填关联，`:1206/:1263` 也按字符串查找 Pivot 字段。因此 `Relations()` 的 map key 虽然是显式字符串，仍依赖 Go 字段名。未来应使用稳定关系标识与字段访问描述/编译期 accessor 的绑定，或由实际混淆配置明确保留这些被反射访问的字段。不能只增加列标签就宣称全部关联可混淆。
- **类型身份与类型名称分开。** 用 `reflect.Type` 标识同一构建中的模型、缓存反射字段路径可以保留；用 `Type.Name()`/包路径作为数据库表名、持久化 morph type 或跨构建的模型标识不可以。诊断错误中展示类型名不影响数据库映射。
- **缓存按描述隔离。** 缓存键用类型身份和映射/连接上下文；连接切换、clone、transaction 不能复用另一套显式描述。不要以类型名称字符串拼接稳定标识。
- **Schema 编排保持通用。** Blueprint 的明确表列名和约束名直接进入完整 Schema Grammar。模型迁移生成也读取相同显式描述，缺失映射时明确失败；根据 Go 基础类型决定数据库类型与根据 Go 类型名决定表名是两件不同的事。
- **驱动内部查询也检查映射。** 旧 MySQL `getVersion()` 扫描到未标注的 `Value` 字段。新版本查询优先用 `QueryRowContext(...).Scan(&version)` 标量扫描，避免把驱动内部的 Go 字段名称当作查询结果列名。

这些是设计建议，尚未实施。它们不改变已经确认的 MySQL Schema Grammar、版本兼容、字符集与时区迁移范围。

**建议的验收用例**

| 维度 | 应验证内容 |
| --- | --- |
| 符号重命名 | 两套不同 Go 类型名/字段名使用相同显式描述，生成完全相同的表列 SQL 和映射结果。 |
| 映射缺失 | 没有表名或所需列描述时明确失败，不按类型名/字段名生成备用名称。 |
| 表名入口 | `TableName()`、`Table(...)`、显式模型描述；prefix 只附加一次；带 schema 和别名的表。 |
| 执行一致性 | 同一模型 Create/Update/Get/Scan/ToSql 使用同一列名；主键、软删除、自动时间戳和事件属性路径一致。 |
| 多连接 | 相同 Go 类型在不同 prefix/显式描述上下文使用；交替执行、事务和并发时不串映射。 |
| 关联 | 验证字符串关系标识到字段/accessor 的绑定；显式数据库关系键保持不变；实际混淆构建能完成 eager load 和关系写入。 |
| 模型迁移 | 读取显式表列描述，输出与运行时一致；没有描述时明确报错。 |
| Schema 名称 | Blueprint 的显式索引/外键名称和默认名称保持现有约定，不意外换成 GORM 的命名格式。 |
| 发布构建 | 使用实际采用的压缩/混淆命令执行 Create/Read/Update/Delete、关系、事件、主键、软删除和迁移映射回归。普通构建或手动改名的测试不能替代此项。 |

调查依据：旧配置装配与 GORM NamingStrategy 源码、旧 Goravel 查询入口、当前 ORM/扫描/迁移实现，以及仅在临时目录运行的名称转换探针。没有修改运行时代码。
