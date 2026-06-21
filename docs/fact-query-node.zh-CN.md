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
  -> default_kyc_profile_store
       -> marketing_kyc_profile
       -> KYC_PROFILE
  -> default_risk_profile_store
       -> marketing_risk_profile
       -> RISK_PROFILE
```

含义：

| 对象 | 类型 | 作用 |
| --- | --- | --- |
| `default_marketing_database` | `DATABASE` | 默认营销数据库资源，表示底层数据库连接和能力 |
| `default_user_profile_store` | `TABLE` | 基于默认数据库创建的用户资料 Store |
| `marketing_user_profile` | 物理表 | 存储用户等级、VIP、国家、渠道、状态等 |
| `USER_PROFILE` | Fact Code | workflow 中统一读取的事实 key |
| `default_kyc_profile_store` | `TABLE` | 基于默认数据库创建的 KYC 用户资源 Store |
| `marketing_kyc_profile` | 物理表 | 存储实名状态、KYC 等级、认证来源等 |
| `KYC_PROFILE` | Fact Code | 资格校验中读取的实名 / 合规事实 key |
| `default_risk_profile_store` | `TABLE` | 基于默认数据库创建的风控用户资源 Store |
| `marketing_risk_profile` | 物理表 | 存储风险分、风险等级、黑名单状态等 |
| `RISK_PROFILE` | Fact Code | 资格校验中读取的风控事实 key |

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
  "type": "TABLE",
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

## 默认 KYC Store

内置默认 KYC 用户资源：

```json
{
  "code": "default_kyc_profile_store",
  "type": "TABLE",
  "driver": "POSTGRES",
  "config": {
    "databaseRef": "default_marketing_database",
    "storeKind": "FACT_STORE",
    "factCode": "KYC_PROFILE",
    "adapter": "KYC_PROFILE",
    "physicalTable": "marketing_kyc_profile",
    "primaryKey": ["tenantId", "subjectType", "subjectId"],
    "keyTemplate": "${tenantId}:${subject.type}:${subject.id}"
  }
}
```

`marketing_kyc_profile` 由迁移 `010_kyc_profile_fact_provider.sql` 创建。迁移会根据已有 `marketing_user_profile` 初始化数据：

- `status = ACTIVE` 的用户初始化为 `KYC_PROFILE.status = VERIFIED`、`verified = true`。
- 其它用户初始化为 `UNVERIFIED`。
- `country` 会从用户资料复制。
- `vip_level >= 3` 初始化为 `level = STANDARD`，其它为 `BASIC`。

Fact Query 查询 KYC 时推荐配置：

```json
{
  "factKeys": ["KYC_PROFILE"],
  "resourceRef": "default_kyc_profile_store",
  "table": "marketing_kyc_profile",
  "keyTemplate": "${tenantId}:${subject.type}:${subject.id}"
}
```

## 默认风控 Store

内置默认风控用户资源：

```json
{
  "code": "default_risk_profile_store",
  "type": "TABLE",
  "driver": "POSTGRES",
  "config": {
    "databaseRef": "default_marketing_database",
    "storeKind": "FACT_STORE",
    "factCode": "RISK_PROFILE",
    "adapter": "RISK_PROFILE",
    "physicalTable": "marketing_risk_profile",
    "primaryKey": ["tenantId", "subjectType", "subjectId"],
    "keyTemplate": "${tenantId}:${subject.type}:${subject.id}"
  }
}
```

`marketing_risk_profile` 由迁移 `011_risk_profile_fact_provider.sql` 创建。迁移会根据已有 `marketing_user_profile` 初始化数据：

- `status = ACTIVE` 的用户初始化为 `score = 0`、`level = LOW`、`status = CLEAR`、`blacklisted = false`。
- 其它用户初始化为 `score = 80`、`level = HIGH`、`status = REVIEW`。
- `country` 会从用户资料复制。

Fact Query 查询风控时推荐配置：

```json
{
  "factKeys": ["RISK_PROFILE"],
  "resourceRef": "default_risk_profile_store",
  "table": "marketing_risk_profile",
  "keyTemplate": "${tenantId}:${subject.type}:${subject.id}"
}
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
  "subjectPath": "subject.id",
  "providerChain": [
    {
      "source": "PROVIDER",
      "resourceRef": "user-profile-provider",
      "namespace": "facts",
      "keyTemplate": "${tenantId}:${subject.type}:${subject.id}",
      "valuePath": "$.facts.USER_PROFILE",
      "timeoutSeconds": 3,
      "cacheTtlSeconds": 0,
      "onMiss": "BLOCK"
    }
  ]
}
```

UI 上只分三块：

| 分组 | 内容 | 说明 |
| --- | --- | --- |
| 配置参数 | `subjectPath` | 当前 fact-query 的公共主体路径，例如用户 ID、订单 ID 或业务主体 ID |
| 事实 Key | `factKeys` | 本节点要产出的事实字段，例如 `USER_PROFILE` |
| 数据提供者 | `providerChain` | 选择 Provider Ref、Resource Ref、上下文或本地缓存；底层 HTTP/Redis/gRPC/Plugin/Lua 实现放在资源管理里 |

字段说明：

| 字段 | 必填 | 说明 | 示例 |
| --- | --- | --- | --- |
| `factKeys` | 是 | 要查询的事实 key，可一次查多个 | `["USER_PROFILE"]` |
| `subjectPath` | 是 | 从输入上下文读取主体 ID 的路径 | `subject.id` |
| `providerChain` | 是 | 数据提供者列表，所有取数参数都放在 step 内 | 见下方 |

不再推荐在节点里配置 `endpoint`、`redisRef`、`grpcTarget`、`pluginCode`、Lua 脚本或底层操作类型。节点只表达业务查询意图；技术实现放到 `PROVIDER` 或 `RESOURCE` 资源定义里。旧 draft 仍兼容，但新 UI 不再把这些字段作为产品化配置入口。

## 数据提供者设计

数据提供者是 `fact-query` 的统一查询入口。产品层只允许选择：

- `PROVIDER`：公共事实 Provider，例如 `user-profile-provider`、`kyc-provider`、`risk-provider`。
- `RESOURCE`：受控资源 Ref，适合读取明确的 Store。
- `WORKFLOW_CONTEXT`：从事件、上游输出或公共上下文读取。
- `LOCAL_CACHE`：从本次 workflow 运行缓存读取。
- `FACT_SERVICE`：旧兼容入口，新配置优先使用 `PROVIDER`。

HTTP、Redis、gRPC、Plugin、Custom、Lua 不再作为 fact-query 节点来源暴露。需要这些实现时，先在资源管理里定义资源或 Provider，再由节点选择 Ref。

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
      "source": "PROVIDER",
      "resourceRef": "user-profile-provider",
      "namespace": "facts",
      "keyTemplate": "${tenantId}:${subject.type}:${subject.id}",
      "onMiss": "BLOCK"
    }
  ]
}
```

每个 step 的含义：

| 字段 | 必填 | 说明 |
| --- | --- | --- |
| `source` | 是 | 查询来源：`PROVIDER`、`RESOURCE`、`WORKFLOW_CONTEXT`、`LOCAL_CACHE`；`FACT_SERVICE` 仅保留兼容 |
| `resourceRef` | 视来源而定 | 选择资源别名；`PROVIDER` 只能选择 `type=PROVIDER` 的公共 Provider |
| `sourcePath` | 否 | `WORKFLOW_CONTEXT` 使用，从当前事件、上游输出或公共上下文读取 |
| `cachePath` | 否 | `LOCAL_CACHE` 使用，从 workflow 运行缓存读取 |
| `keyTemplate` | 否 | 资源或缓存查询 key 模板；普通 Provider 通常不需要用户手填 |
| `namespace` | 否 | 资源审计和 key policy 命名空间 |
| `valuePath` | 否 | 从资源响应里提取 fact value 的 JSON Path |
| `providerCode` | 否 | 旧 `FACT_SERVICE` 兼容字段；新配置优先选择 `PROVIDER resourceRef` |
| `timeoutSeconds` | 否 | 当前 provider step 的查询超时 |
| `cacheTtlSeconds` | 否 | 当前 provider step 的缓存时间；`0` 表示不缓存 |
| `onMiss` | 否 | 当前 step 没查到时动作：`NEXT` 继续下一步，`BLOCK` 立即失败 |

## 数据提供者目标规则

发布校验会按 `source` 检查目标字段，避免运行时才出现 `ADAPTER_REQUIRED`。

| Source | 当前目标要求 | 说明 |
| --- | --- | --- |
| `PROVIDER` | 必须选择 `resourceRef` | `resourceRef` 应指向 `type=PROVIDER` 的公共事实 Provider |
| `RESOURCE` | 必须选择 `resourceRef` | `resourceRef` 应指向资源管理中的受控 Store |
| `WORKFLOW_CONTEXT` | 不需要资源 | 从当前事件、上游输出、变量、facts 读取 |
| `LOCAL_CACHE` | 不需要资源 | 从本次 workflow 变量缓存读取 |
| `FACT_SERVICE` | `resourceRef` / `providerCode` / `endpoint` 任一 | 旧兼容入口；新配置不建议使用 |

因此下面这种新配置不再推荐：

```json
{
  "providerChain": [
    {
      "source": "HTTP",
      "endpoint": "https://user-service/internal/facts/user-profile"
    }
  ]
}
```

应改成：

```json
{
  "providerChain": [
    {
      "source": "PROVIDER",
      "resourceRef": "user-profile-provider",
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
- 每一步用下拉选择 `source`；新建配置只暴露 `PROVIDER`、`RESOURCE`、`WORKFLOW_CONTEXT`、`LOCAL_CACHE` 和兼容用 `FACT_SERVICE`。
- 当 `source=PROVIDER` 时显示 Provider 资源下拉，只能选择 `type=PROVIDER` 且当前 workspace/folder/activity 有权限的资源。
- 当 `source=RESOURCE` 时显示普通资源下拉，底层 HTTP/Redis/gRPC/Plugin/Lua 实现不在节点里展开。
- `HTTP`、`REDIS`、`GRPC`、`PLUGIN`、`CUSTOM`、`LUA_QUERY` 只作为旧 draft 兼容值显示，不作为新配置入口。
- 支持上移、下移、删除；复制可作为后续快捷操作补充。
- 缺少 `resourceRef`、key 模板不合法、资源无权限时在当前 step 标红。
- 调试运行时 `trace` 需要显示每个 step 是否命中、miss、失败或被跳过。

兼容要求：

- 旧配置 `["WORKFLOW_CONTEXT", "LOCAL_CACHE", "HTTP"]` 仍然由后端兼容解析。
- 新配置使用对象数组，executor 会在运行前解析为 resolved provider steps。
- 新 UI 只推荐使用 `providerChain` 中的 Provider Ref / Resource Ref。旧 draft 中的顶层 Provider 字段仍由后端兼容读取，但不再作为产品化配置入口。

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

## onMiss 和失败策略

`onMiss` 是数据提供者 step 级别的缺失处理策略，只处理“当前 provider 没查到结果或没有解析出需要的 fact”，不处理“查到用户不存在”。

| 值 | 行为 | 适用场景 |
| --- | --- | --- |
| `BLOCK` | 当前 provider 未命中时立即阻断 fact-query | 生产资格、发奖前强依赖事实 |
| `NEXT` | 当前 provider 未命中时继续下一个 provider | 缓存优先、Store 兜底、HTTP 兜底 |

生产奖励链路推荐：

```json
{
  "providerChain": [
    {
      "source": "PROVIDER",
      "resourceRef": "user-profile-provider",
      "onMiss": "BLOCK"
    }
  ]
}
```

因为用户等级、资格、存款摘要等事实一旦无法解析，继续发奖风险较高。

## 常见资源类型

`fact-query` 在节点层只引用公共 Provider 或受控资源。不同技术实现由资源管理承接。

| Store / Provider | 典型 resource type | 说明 |
| --- | --- | --- |
| 公共事实 Provider | `PROVIDER` | 节点首选入口，例如 `user-profile-provider`、`kyc-provider`、`risk-provider` |
| 数据库 Store | `TABLE` | 读取营销域公共表，例如 `marketing_user_profile` |
| Redis Store | `REDIS mode=KV/HASH` | 读取预计算事实缓存 |
| HTTP / gRPC / Plugin / Lua 实现 | `HTTP` / `GRPC_PROVIDER` / `PLUGIN_PROVIDER` / `CUSTOM_PROVIDER` | 不直接出现在 fact-query 节点下拉中，应封装为 `PROVIDER` 或 `RESOURCE` 后再引用 |

推荐优先级：

```text
事件 attributes 已有可信数据
  -> 不需要 fact-query

营销域已有公共 Store
  -> 建立 PROVIDER，targetResourceRef 指向 TABLE / REDIS Store

必须实时查业务系统
  -> 建立 PROVIDER，底层 target 指向 HTTP/gRPC/Plugin 资源

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
  "subjectPath": "subject.id",
  "providerChain": [
    {
      "source": "PROVIDER",
      "resourceRef": "user-profile-provider",
      "namespace": "facts",
      "keyTemplate": "${tenantId}:${subject.type}:${subject.id}",
      "timeoutSeconds": 3,
      "cacheTtlSeconds": 0,
      "onMiss": "BLOCK"
    }
  ]
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

### 数据提供者里的 resourceRef 应该填什么？

新配置优先填公共 Provider，例如 `user-profile-provider`。Provider 再通过 `targetResourceRef` 指向实际 Store 或外部实现。

```json
{
  "providerChain": [
    {
      "source": "PROVIDER",
      "resourceRef": "user-profile-provider"
    }
  ]
}
```

数据库资源只作为底层连接，不直接放到 fact-query 节点里：

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
6. 生产发奖链路中，关键事实数据提供者的 `onMiss` 应设置为 `BLOCK`。
