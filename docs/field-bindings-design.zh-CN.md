# 字段级表达式与输入解析设计

本文定义 ExMarketing Workflow 节点的 `fieldBindings` 能力。目标是让每个节点的普通配置字段或输入字段既可以填写固定值，也可以在运行时通过表达式或有返回值的 Lua 脚本动态解析。

## 目标

- 保持现有节点配置兼容，旧配置无需迁移。
- 节点 handler 不感知字段值来源，只读取解析后的配置。
- 字段值可以来自固定值、模板、上下文、上游节点输出、资源或 Lua。
- 资源访问必须经过资源管理授权、限额和审计，不能直接访问 Redis、HTTP、DB 或文件系统。
- 调试和运行历史中必须能看到字段解析来源、结果、错误和耗时。

## 配置结构

`fieldBindings` 放在节点 `config` 下，与原字段并存。原字段作为默认值或兜底值，绑定解析成功后覆盖同名字段。

```json
{
  "scope": "USER",
  "scopeKeyTemplate": "${subject.id}:${activityId}",
  "window": "DAY",
  "maxCount": 1,
  "consumeMode": "ATOMIC",
  "exceedAction": "BLOCK",
  "resourceBindings": {
    "activityConfig": "activity_config_kv",
    "userProfile": "redis_user_profile"
  },
  "fieldBindings": {
    "maxCount": {
      "mode": "EXPRESSION",
      "language": "LUA",
      "source": "Resource.activityConfig.get('daily_checkin_max_count')"
    },
    "scopeKeyTemplate": {
      "mode": "EXPRESSION",
      "language": "LUA",
      "source": "Context.get('event.subject.id') .. ':' .. Context.get('activityId')"
    }
  }
}
```

节点也可以使用整段 Lua 配置脚本生成配置值。该模式适合一个节点内多个字段有关联，需要共享临时变量、资源读取或上下文计算的场景。

```json
{
  "configMode": "LUA",
  "configScript": "local userId = Context.get('event.subject.id')\nscope = 'USER'\nscopeKeyTemplate = userId .. ':' .. Context.get('activityId')\nwindow = 'DAY'\nmaxCount = Resource.activityConfig.get('daily_checkin_max_count')\nconsumeMode = 'ATOMIC'\nexceedAction = 'BLOCK'"
}
```

节点级脚本规则：

- 每个节点可赋值的变量名由该节点 schema 固定定义。
- 脚本里直接给固定变量赋值，例如 `maxCount = 3`。
- 临时变量必须使用 `local`，例如 `local userId = Context.get(...)`。
- resolver 只收集 schema 中存在且是合法 Lua 标识符的字段。
- 未被 schema 声明的全局变量不会进入 `resolvedConfig`。
- 节点级 Lua 配置在字段级 `fieldBindings` 之后执行，因此优先级最高。

## Binding 字段

| 字段 | 说明 |
| --- | --- |
| `mode` | `EXPRESSION` 或 `SCRIPT`。表达式模式自动包装为返回值；脚本模式要求 Lua 脚本显式 `return`。 |
| `language` | 当前支持 `LUA`。后续可扩展 `JSONPATH`、`CEL`。 |
| `source` | 表达式或脚本文本。 |
| `fallback` | 可选，解析失败或空值时使用的兜底值。 |
| `onError` | 错误策略：`FAIL`、`USE_DEFAULT`、`USE_FALLBACK`、`NULL`。默认 `FAIL`。 |
| `valueType` | 可选类型提示：`STRING`、`NUMBER`、`BOOLEAN`、`OBJECT`、`ARRAY`、`ANY`。 |

## 表达式模式

表达式模式只写一个表达式，不需要 `return`。

```lua
Resource.activityConfig.get("daily_checkin_max_count")
```

系统内部等价于：

```lua
return Resource.activityConfig.get("daily_checkin_max_count")
```

适合 `maxCount`、`amount`、`scopeKeyTemplate`、`quotaCode`、`endpoint` 等单值字段。

## 脚本模式

脚本模式用于多行逻辑，必须有返回值。

```lua
local vip = Resource.userProfile.get(Context.get("event.subject.id"))
if vip.level >= 5 then
  return 5
end
return 1
```

脚本不是 Lua 节点，不会改变流程结构；它只为当前字段返回一个值。

## 内置对象

### Context

`Context` 读取当前 execution 的上下文。

```lua
Context.get("activityId")
Context.get("event.subject.id")
Context.get("event.attributes.amount")
Context.get("input.resolvedInputs.facts")
Context.set("vars.debugLimit", 3)
```

`Context.set` 默认只写当前运行上下文变量；如需持久化，必须写资源。

### Resource

`Resource.<alias>` 来自节点 `resourceBindings`：

```json
{
  "resourceBindings": {
    "activityConfig": "activity_config_kv",
    "userProfile": "redis_user_profile"
  }
}
```

表达式中使用：

```lua
Resource.activityConfig.get("daily_checkin_max_count")
Resource.userProfile.get(Context.get("event.subject.id"))
Resource.userProfile.set("u_001", {vipLevel = 3})
Resource.userProfile.exists("u_001")
Resource.userProfile.incr("checkin:u_001", 1)
Resource.userProfile.delete("u_001")
```

底层根据资源类型分发到 Redis、数据库、HTTP Provider、gRPC 或插件，但都必须经过资源权限、调用次数和响应大小限制。

也支持显式资源引用：

```lua
ResourceRef("activity_config_kv").get("daily_checkin_max_count")
```

### Node

读取上游节点输出：

```lua
Node.get("normalize", "normalized_event", "$.attributes.channel")
Node.get("fact-query", "facts", "$.user.vipLevel")
```

### Template 和 Time

```lua
Template.render("checkin:${subject.id}:${date}")
Time.now()
Time.date("Asia/Shanghai")
```

## 解析顺序

1. 读取节点原始 config。
2. 应用 schema 默认值。
3. 解析 `fieldBindings`。
4. 执行节点级 `configScript`，收集 schema 固定字段变量。
5. 生成 `resolvedConfig`。
6. handler 使用 `resolvedConfig` 执行。
7. 节点运行 input/trace 写入字段解析记录。

## 错误策略

| onError | 行为 |
| --- | --- |
| `FAIL` | 字段解析失败时节点失败。默认策略。 |
| `USE_DEFAULT` | 使用原字段值或 schema 默认值继续执行。 |
| `USE_FALLBACK` | 使用 binding.fallback。 |
| `NULL` | 字段值置为 null。 |

所有错误都会写入 trace。即使使用兜底策略，也要能在运行历史中看到解析失败原因。

## 审计输出

节点执行 input 中写入：

```json
{
  "resolvedConfig": {
    "maxCount": 3
  },
  "fieldBindingTrace": [
    {
      "field": "maxCount",
      "mode": "EXPRESSION",
      "language": "LUA",
      "source": "Resource.activityConfig.get('daily_checkin_max_count')",
      "status": "SUCCESS",
      "value": 3,
      "durationMs": 2
    },
    {
      "field": "configScript",
      "mode": "SCRIPT",
      "language": "LUA",
      "status": "SUCCESS",
      "assignedFields": ["scope", "scopeKeyTemplate", "window", "maxCount"],
      "ignoredFields": [],
      "durationMs": 3
    }
  ]
}
```

资源调用仍由 Resource API 记录资源审计，字段 trace 只记录字段解析视角。

## UI 要求

- 每个 schema 字段右侧显示 `fx` 按钮。
- `fx` 未启用时显示固定值输入控件。
- `fx` 启用后可选择：
  - 表达式
  - Lua 脚本
- 支持填写 source、错误策略、fallback。
- 字段行展示 `fx` 状态和简短预览。
- 配置区域提供节点级 Lua 配置编辑器。
- Lua 编辑器使用轻量级代码编辑器，支持 Lua 语法高亮。
- 编辑器聚焦或按空格时提示当前节点固定变量名、默认值和字段说明。
- 调试面板展示 `resolvedConfig` 和 `fieldBindingTrace`。
