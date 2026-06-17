# Fact Query 事实查询节点使用说明

`fact-query` 用来在 workflow 执行中补齐事件没有携带、但资格判断或奖励计算需要使用的业务事实。

典型事实包括：

- 用户资料：用户是否存在、用户等级、VIP 等级、注册时间、国家、渠道、标签。
- 订单/存款摘要：周期存款金额、首存状态、历史存款次数。
- 交易/流水摘要：有效投注、盈亏、结算状态。
- 邀请关系：邀请人、被邀请人、关系是否有效、邀请任务进度。
- 风控事实：设备/IP 风险、黑名单、KYC 状态、异常行为评分。

## 定位

`fact-query` 只负责“查询事实并输出 facts”，不负责判断是否通过。

推荐链路：

```text
event-in -> inbox -> normalize -> fact-query -> user-level-check / qualify -> quota-check -> reward...
```

职责边界：

| 节点 | 职责 |
| --- | --- |
| `fact-query` | 查询并输出 `facts` |
| `user-level-check` | 根据 `facts.USER_PROFILE` 判断用户是否存在、等级/VIP 是否满足 |
| `qualify` | 根据 facts、event attributes 和活动规则做业务资格判断 |
| `quota-check` | 做次数和频率限制 |
| `state-store` | 读写 workflow 状态、公共表、Redis 语义状态 |

不要把资格判断直接写进 `fact-query`。例如用户不存在时，`fact-query` 应输出：

```json
{
  "facts": {
    "USER_PROFILE": {
      "exists": false,
      "level": 0,
      "vipLevel": 0,
      "status": "MISSING"
    }
  }
}
```

然后由 `user-level-check` 或 `qualify` 输出 `BLOCK`。

## 资源和 Store 模型

推荐资源结构是两层：

```text
底层资源 Resource
  -> Store / Provider
       -> Fact
```

以默认用户资料为例：

```text
default_marketing_database
  -> default_user_profile_store
       -> marketing_user_profile
       -> USER_PROFILE
```

含义：

| 对象 | 类型 | 作用 |
| --- | --- | --- |
| `default_marketing_database` | `DATABASE` | 默认营销数据库资源，表示底层数据库连接和能力 |
| `default_user_profile_store` | `DATABASE_TABLE` | 基于默认数据库创建的用户资料 Store |
| `marketing_user_profile` | 物理表 | 存储用户等级、VIP、国家、渠道、状态等 |
| `USER_PROFILE` | Fact Code | workflow 中统一读取的事实 key |

节点应该引用 Store：

```json
{
  "resourceRef": "default_user_profile_store"
}
```

不建议节点直接引用底层数据库资源：

```json
{
  "resourceRef": "default_marketing_database"
}
```

底层数据库是基础设施能力，Store 才是业务可读写的数据集边界。

## 默认用户资料 Store

内置默认用户资料 Store：

```json
{
  "code": "default_user_profile_store",
  "type": "DATABASE_TABLE",
  "driver": "POSTGRES",
  "config": {
    "databaseRef": "default_marketing_database",
    "storeKind": "FACT_STORE",
    "factCode": "USER_PROFILE",
    "adapter": "USER_PROFILE",
    "physicalTable": "marketing_user_profile",
    "primaryKey": ["tenantId", "subjectType", "subjectId"],
    "keyTemplate": "${tenantId}:${subject.type}:${subject.id}"
  }
}
```

`marketing_user_profile` 查询条件：

```sql
WHERE tenant_id = :tenantId
  AND subject_type = :subject.type
  AND subject_id = :subject.id
```

标准事件：

```json
{
  "tenantId": "site_001",
  "subject": {
    "type": "USER",
    "id": "u_demo_001"
  }
}
```

实际查询：

```text
tenant_id = site_001
subject_type = USER
subject_id = u_demo_001
```

## 节点配置

每日签到用户资料查询推荐配置：

```json
{
  "factKeys": ["USER_PROFILE"],
  "resourceRef": "default_user_profile_store",
  "namespace": "facts",
  "table": "marketing_user_profile",
  "subjectPath": "subject.id",
  "keyTemplate": "${tenantId}:${subject.type}:${subject.id}",
  "timeoutSeconds": 3,
  "cacheTtlSeconds": 0,
  "failureAction": "BLOCK"
}
```

字段说明：

| 字段 | 必填 | 说明 | 示例 |
| --- | --- | --- | --- |
| `factKeys` | 是 | 要查询的事实 key，可一次查多个 | `["USER_PROFILE"]` |
| `resourceRef` | 推荐必填 | 引用资源管理中的 Store 或 Provider | `default_user_profile_store` |
| `namespace` | 否 | 资源审计和 key policy 的命名空间 | `facts` |
| `subjectPath` | 是 | 从输入上下文读取主体 ID 的路径 | `subject.id` |
| `keyTemplate` | 否 | 逻辑 key 模板，用于资源审计和缓存/Store 查询 | `${tenantId}:${subject.type}:${subject.id}` |
| `table` | 视 Store 而定 | 显式标识 Store 背后的业务表，方便 UI 和审计展示 | `marketing_user_profile` |
| `timeoutSeconds` | 否 | 查询超时时间 | `3` |
| `cacheTtlSeconds` | 否 | 查询成功后的缓存时间；`0` 表示不缓存 | `60` |
| `failureAction` | 是 | 查询失败时动作 | `BLOCK` |

## 数据提供者设计

数据提供者是 `fact-query` 的统一查询入口。上下文、运行缓存、Store、HTTP、Redis、gRPC、Plugin、自定义协议都应该作为“数据提供者 step”配置到同一个列表里，而不是分散在“上下文和缓存”“Provider 目标”“高级兼容字段”几块里。

底层字段暂时仍叫 `providerChain`，用于兼容已有 draft；产品语义统一叫“数据提供者”。

推荐结构：

```json
{
  "providerChain": [
    {
      "source": "WORKFLOW_CONTEXT",
      "sourcePath": "$.outputs.normalize.normalized_event.attributes.USER_PROFILE",
      "onMiss": "NEXT"
    },
    {
      "source": "LOCAL_CACHE",
      "cachePath": "$.vars.execution.facts.USER_PROFILE",
      "onMiss": "NEXT"
    },
    {
      "source": "FACT_SERVICE",
      "resourceRef": "default_user_profile_store",
      "namespace": "facts",
      "keyTemplate": "${tenantId}:${subject.type}:${subject.id}",
      "onMiss": "BLOCK"
    },
    {
      "source": "HTTP_PROVIDER",
      "resourceRef": "user_profile_fact_provider",
      "endpoint": "https://user-service/internal/facts/user-profile",
      "method": "POST",
      "authRef": "user-service-auth",
      "onMiss": "BLOCK"
    },
    {
      "source": "REDIS",
      "resourceRef": "user_profile_cache",
      "redisRef": "marketing-fact-cache",
      "keyTemplate": "marketing:fact:${tenantId}:${subject.id}:USER_PROFILE",
      "onMiss": "NEXT"
    }
  ]
}
```

每个 step 的含义：

| 字段 | 必填 | 说明 |
| --- | --- | --- |
| `source` | 是 | 查询来源，例如 `WORKFLOW_CONTEXT`、`LOCAL_CACHE`、`FACT_SERVICE`、`HTTP_PROVIDER`、`REDIS` |
| `resourceRef` | 视来源而定 | 需要访问资源管理时选择资源别名，例如 `default_user_profile_store` |
| `sourcePath` | 否 | `WORKFLOW_CONTEXT` 使用，从当前事件、上游输出或公共上下文读取 |
| `cachePath` | 否 | `LOCAL_CACHE` 使用，从 workflow 运行缓存读取 |
| `keyTemplate` | 否 | 资源、Redis、缓存查询 key 模板 |
| `namespace` | 否 | 资源审计和 key policy 命名空间 |
| `operation` | 否 | 资源操作，通常是 `READ` 或 `CALL` |
| `valuePath` | 否 | 从资源响应里提取 fact value 的 JSON Path |
| `endpoint` | 否 | `HTTP_PROVIDER` 的 HTTP 地址；推荐放资源管理，节点里只作为同一 step 的覆盖参数 |
| `method` | 否 | `HTTP_PROVIDER` 的请求方法 |
| `authRef` | 否 | `HTTP_PROVIDER` 的鉴权引用 |
| `redisRef` | 否 | `REDIS` 提供者的 Redis 连接或资源引用 |
| `grpcTarget` | 否 | `GRPC` 提供者的服务地址或服务发现名 |
| `pluginCode` | 否 | `PLUGIN` 提供者的插件编码 |
| `providerCode` | 否 | 已登记的业务 Fact Provider 编码 |
| `onMiss` | 否 | 当前 step 没查到时动作：`NEXT` 继续下一步，`BLOCK` 立即失败 |

## 数据提供者目标规则

发布校验会按 `source` 检查目标字段，避免运行时才出现 `ADAPTER_REQUIRED`。

| Source | 当前目标要求 | 说明 |
| --- | --- | --- |
| `WORKFLOW_CONTEXT` | 不需要资源 | 从当前事件、上游输出、变量、facts 读取 |
| `LOCAL_CACHE` | 不需要资源 | 从本次 workflow 变量缓存读取 |
| `FACT_SERVICE` | `resourceRef` / `providerCode` / `endpoint` 任一 | 推荐 `resourceRef=default_user_profile_store` |
| `HTTP_PROVIDER` | `resourceRef` / `providerCode` / `endpoint` 任一 | 推荐把 endpoint/auth 放到资源管理，节点只引用资源 |
| `REDIS` | 必须 `resourceRef` | `resourceRef` 应指向资源管理中的 Redis 或 Redis 语义 Store |
| `GRPC` | 必须 `resourceRef` | 裸 `grpcTarget` 只做展示，真实执行需要受控资源 |
| `PLUGIN` | 必须 `resourceRef` | 插件执行也要通过资源授权和审计边界 |
| `CUSTOM` | 必须 `resourceRef` | 自定义协议必须先登记为资源 |
| `LUA_QUERY` | 必须 `resourceRef` | Lua 查询可调用资源 API，但节点目标仍需可审计 |

因此下面配置会被发布校验拦截：

```json
{
  "providerChain": [
    {
      "source": "REDIS",
      "redisRef": "marketing-fact-cache",
      "keyTemplate": "marketing:fact:${tenantId}:${subject.id}:USER_PROFILE"
    }
  ]
}
```

应改成：

```json
{
  "providerChain": [
    {
      "source": "REDIS",
      "resourceRef": "user_profile_cache",
      "namespace": "facts",
      "keyTemplate": "marketing:fact:${tenantId}:${subject.id}:USER_PROFILE",
      "onMiss": "NEXT"
    },
    {
      "source": "FACT_SERVICE",
      "resourceRef": "default_user_profile_store",
      "namespace": "facts",
      "keyTemplate": "${tenantId}:${subject.type}:${subject.id}",
      "onMiss": "BLOCK"
    }
  ]
}
```

## 运行 Trace

`fact-query` 输出里会包含：

| 字段 | 说明 |
| --- | --- |
| `facts` | 已解析出的事实对象，供下游 `user-level-check`、`qualify`、`reward` 使用 |
| `factKeys` | 本次请求的事实 key 列表 |
| `providerChain` | 每个数据提供者 step 的执行摘要 |
| `providerChain[].status` | `DONE` 命中或完成、`MISS` 未命中、`FAILED_NEXT` 失败后继续、`ADAPTER_REQUIRED` 未配置真实执行目标 |
| `providerChain[].remaining` | 进入该 step 前仍未解析的 fact key |
| `providerChain[].resourceRef` | 当前 step 使用的资源引用 |
| `responses` | provider 原始响应摘要，用于调试和审计 |

每日签到成功时，`USER_PROFILE` 的关键结构类似：

```json
{
  "facts": {
    "USER_PROFILE": {
      "exists": true,
      "subjectId": "u_demo_001",
      "level": 1,
      "vipLevel": 1,
      "status": "ACTIVE",
      "country": "CN",
      "channel": "APP"
    }
  }
}
```

用户不存在时，默认用户资料 Store 会输出 `exists=false`，下游 `user-level-check` 或 `qualify` 决定是否 `BLOCK`。

UI 推荐：

- `数据提供者` 显示为步骤列表，不显示原始 JSON。
- 每一步用下拉选择 `source`。
- 当 `source` 需要资源时显示 `resourceRef` 下拉，只能选择当前 workspace/folder/activity 有权限的资源。
- 当 `source=HTTP_PROVIDER` 时，在同一个 step 内显示 endpoint、method、authRef。
- 当 `source=REDIS` 时，在同一个 step 内显示 redisRef、keyTemplate。
- 当 `source=GRPC` 时，在同一个 step 内显示 grpcTarget。
- 当 `source=PLUGIN` 时，在同一个 step 内显示 pluginCode。
- 支持上移、下移、删除；复制可作为后续快捷操作补充。
- 缺少 `resourceRef`、key 模板不合法、资源无权限时在当前 step 标红。
- 调试运行时 `trace` 需要显示每个 step 是否命中、miss、失败或被跳过。

兼容要求：

- 旧配置 `["WORKFLOW_CONTEXT", "LOCAL_CACHE", "HTTP_PROVIDER"]` 仍然有效。
- 新配置使用对象数组，executor 会在运行前解析为 resolved provider steps。
- 如果 `providerChain` 非空，executor 优先按数据提供者链执行；只有没有数据提供者链时，顶层 `resourceRef` 才作为单 Provider 快捷配置执行。

### subjectPath

默认 `subjectPath` 是：

```text
subject.id
```

它会读取标准事件里的：

```json
{
  "subject": {
    "type": "USER",
    "id": "u_demo_001"
  }
}
```

如果业务事件暂时不是标准结构，也可以改成：

```json
{
  "subjectPath": "attributes.userId"
}
```

但生产推荐统一使用标准事件 `subject.type + subject.id`，这样频控、参与记录、发奖和审计能共享同一个主体。

## 输出结构

成功查询用户：

```json
{
  "source": "RESOURCE",
  "resourceRef": "default_user_profile_store",
  "facts": {
    "USER_PROFILE": {
      "exists": true,
      "subjectType": "USER",
      "subjectId": "u_demo_001",
      "level": 1,
      "vipLevel": 1,
      "status": "ACTIVE",
      "country": "CN",
      "channel": "APP",
      "registeredAt": "2026-01-01T00:00:00Z"
    }
  }
}
```

用户不存在：

```json
{
  "source": "RESOURCE",
  "resourceRef": "default_user_profile_store",
  "facts": {
    "USER_PROFILE": {
      "exists": false,
      "subjectType": "USER",
      "subjectId": "u_demo_336",
      "level": 0,
      "vipLevel": 0,
      "status": "MISSING"
    }
  }
}
```

注意：用户不存在不是 `fact-query` 查询失败。它是成功查询后的事实结果，后续节点根据 `exists=false` 阻断。

## 后续节点怎么使用

### 用户等级判断

`user-level-check` 读取：

```json
{
  "requireExistingUser": true,
  "minLevel": 1,
  "levelPaths": [
    "$.facts.USER_PROFILE.level",
    "$.facts.USER_PROFILE.vipLevel"
  ],
  "existsPath": "exists",
  "failDecision": "BLOCK"
}
```

推荐只信任 `facts.USER_PROFILE`，不要从事件 payload 读取等级，避免调用方伪造：

```json
{
  "levelPaths": [
    "$.facts.USER_PROFILE.level",
    "$.facts.USER_PROFILE.vipLevel"
  ]
}
```

### 资格校验

`qualify` 规则可以读取：

```json
{
  "logic": "AND",
  "conditions": [
    {
      "field": "facts.USER_PROFILE.exists",
      "op": "=",
      "value": true
    },
    {
      "field": "facts.USER_PROFILE.vipLevel",
      "op": ">=",
      "value": 1
    }
  ]
}
```

## failureAction

`failureAction` 只处理“查询失败”，不处理“查到用户不存在”。

| 值 | 行为 | 适用场景 |
| --- | --- | --- |
| `BLOCK` | 查询失败时节点失败，整体流程失败或阻断 | 生产资格、发奖前强依赖事实 |
| `USE_EMPTY` | 查询失败时输出空 facts，继续后续节点 | 非关键画像、辅助推荐 |
| `SKIP` | 查询失败时跳过节点 | 低风险可选增强 |

生产奖励链路推荐：

```json
{
  "failureAction": "BLOCK"
}
```

因为用户等级、资格、存款摘要等事实一旦查询失败，继续发奖风险较高。

## 常见资源类型

`fact-query` 可以引用不同类型的 Store 或 Provider。

| Store / Provider | 典型 resource type | 说明 |
| --- | --- | --- |
| 数据库 Store | `DATABASE_TABLE` | 读取营销域公共表，例如 `marketing_user_profile` |
| Redis Store | `REDIS_KV` / `REDIS_HASH` | 读取预计算事实缓存 |
| HTTP Provider | `HTTP_PROVIDER` | 调业务服务实时查询事实 |
| gRPC Provider | `GRPC_PROVIDER` | 调内部服务查询事实 |
| Plugin Provider | `PLUGIN_PROVIDER` | 通过插件扩展事实查询 |
| Lua Query | `LUA_QUERY` | 用受控 Lua 生成事实；适合加工，不适合绕过资源权限 |

推荐优先级：

```text
事件 attributes 已有可信数据
  -> 不需要 fact-query

营销域已有公共 Store
  -> 使用 DATABASE_TABLE / REDIS_* Store

必须实时查业务系统
  -> 使用 HTTP/gRPC Provider

复杂组合加工
  -> fact-query 后接 combine/code/lua-query
```

## 每日签到示例

目标：用户签到成功后，必须存在且 VIP >= 1，才发放奖励和通知。

标准事件：

```json
{
  "eventId": "evt_checkin_20260616_0001",
  "eventType": "CHECKIN_SUCCESS",
  "sourceSystem": "app",
  "tenantId": "site_001",
  "subject": {
    "type": "USER",
    "id": "u_demo_001"
  },
  "biz": {
    "bizType": "CHECKIN",
    "bizId": "daily_20260616_u_demo_001"
  },
  "occurredAt": "2026-06-16T13:10:00Z",
  "attributes": {
    "channel": "APP",
    "checkin": true,
    "timezone": "Asia/Shanghai",
    "streakDays": 1
  }
}
```

事实查询节点：

```json
{
  "factKeys": ["USER_PROFILE"],
  "resourceRef": "default_user_profile_store",
  "namespace": "facts",
  "table": "marketing_user_profile",
  "subjectPath": "subject.id",
  "failureAction": "BLOCK"
}
```

用户等级判断：

```json
{
  "requireExistingUser": true,
  "minLevel": 1,
  "levelPaths": [
    "$.facts.USER_PROFILE.vipLevel"
  ],
  "existsPath": "exists",
  "failDecision": "BLOCK"
}
```

期望结果：

| 用户 | `USER_PROFILE.exists` | `vipLevel` | 用户等级判断 | 整体结果 |
| --- | --- | --- | --- | --- |
| `u_demo_001` | `true` | `1` | `PASS` | 继续发奖 |
| `u_demo_336` | `false` | `0` | `BLOCK / USER_NOT_FOUND` | `FAILED` 或 `BUSINESS_REJECTED` |

## 调试检查

调试运行后重点看 `fact-query` 节点输出：

```json
{
  "status": "SUCCESS",
  "source": "RESOURCE",
  "resourceRef": "default_user_profile_store",
  "facts": {
    "USER_PROFILE": {
      "exists": true,
      "vipLevel": 1
    }
  }
}
```

如果用户不存在，正确输出应该是：

```json
{
  "status": "SUCCESS",
  "facts": {
    "USER_PROFILE": {
      "exists": false,
      "status": "MISSING"
    }
  }
}
```

后续 `user-level-check` 应输出：

```json
{
  "decision": "BLOCK",
  "reasonCode": "USER_NOT_FOUND"
}
```

## 常见问题

### 为什么不是 fact-query 直接失败？

因为“用户不存在”是业务事实，不是查询失败。查询成功返回 `exists=false`，后续资格节点负责阻断。

### resourceRef 应该填数据库还是 Store？

填 Store。

```json
{
  "resourceRef": "default_user_profile_store"
}
```

数据库资源只作为底层连接：

```text
default_marketing_database -> default_user_profile_store
```

### 可以把用户等级放在事件 attributes 里吗？

技术上可以，但生产不推荐。等级、VIP、黑名单、KYC、存款历史等关键资格事实应该来自可信 Store 或 Provider。

### factKeys 可以多个吗？

可以，例如：

```json
{
  "factKeys": ["USER_PROFILE", "ORDER_SUMMARY"]
}
```

但每个 factKey 都需要有对应 Store/Provider 支持。对于复杂链路，推荐拆成多个 `fact-query`，让每个节点的资源、失败策略和输出更清楚。

## 设计原则

1. 标准事件只携带本次事件事实。
2. 用户等级、历史存款、邀请关系等跨事件事实通过 Store/Provider 查询。
3. 节点引用 Store，不直接写数据库连接、Redis 地址、HTTP 密钥。
4. 查询事实和判断资格分开。
5. 所有资源访问必须经过资源管理权限校验和审计。
6. 生产发奖链路中，关键事实查询失败应 `BLOCK`。
