# ExMarketing 服务、接口、提供者和资源设计

本文档定义 ExMarketing 中工作流节点如何使用服务、接口、提供者和资源。它补充 [公共资源管理设计](resource-management-design.zh-CN.md)，用于把“业务编排”和“技术能力治理”拆开。

核心原则：

```text
节点表达业务编排，不表达基础设施连接。
服务表达业务能力归属。
接口表达能力契约。
提供者表达能力实现。
资源表达底层状态和基础设施。
```

节点不直接配置 HTTP URL、Redis 地址、DB 连接、MQ broker、密钥、SQL 或物理 topic。节点只引用已注册的服务接口、提供者策略或资源，并声明输入、操作和输出。

## 总体分层

ExMarketing 有两条主线。

### 业务编排线

```text
Workspace 工作区
  -> Folder 文件夹
    -> Workflow 流程
      -> Node 节点
```

这条线回答：

| 层级 | 职责 |
| --- | --- |
| `Workspace` | 租户、团队或业务域隔离边界 |
| `Folder` | 活动分类、权限分组和资产归档 |
| `Workflow` | 活动流程、版本、发布、审核和回滚 |
| `Node` | 具体业务编排、条件判断、事实查询、状态读写和副作用边界 |

### 能力资产线

```text
Workspace 工作区
  -> Service 服务
    -> Interface 接口
      -> Provider 提供者
        -> Resource 资源
```

这条线回答：

| 层级 | 职责 |
| --- | --- |
| `Service` | 一个稳定业务或技术能力，例如用户画像、风控、发奖、签到状态 |
| `Interface` | 服务暴露的能力契约，例如查询用户画像、评估风险、发送发奖命令 |
| `Provider` | 接口的具体实现，例如 HTTP、RPC、MQ、Plugin、自定义 TCP |
| `Resource` | Provider 或节点依赖的底层状态和基础设施，例如 Redis、DB、表、MQ topic、对象存储 |

两条线在节点处汇合：

```text
Workflow Node
  -> call Service.Interface
  -> select Provider by policy
  -> operate Resource
```

## 概念定义

### Node

`Node` 是工作流里的业务编排单元。它应该表达“这个活动下一步做什么”，而不是“怎么连 Redis/HTTP/DB”。

节点可以：

- 调用服务接口；
- 操作受控资源；
- 使用 Lua 或表达式调用已授权的成熟服务和方法；
- 读取上游输出并生成下游输出；
- 决定 PASS、BLOCK、RISK、SKIP 等路由。

节点不应该：

- 直接保存 endpoint、token、password、broker、driver；
- 直接执行任意 SQL；
- 直接访问未注册的 HTTP/RPC/MQ/TCP 目标；
- 绕过资源权限、租户隔离和审计。

### Service

`Service` 是能力归属。它代表一个业务或技术服务，而不是某个具体协议。

示例：

```text
user-profile-service
risk-evaluation-service
reward-grant-service
checkin-state-service
marketing-fact-service
```

`Service` 负责说明：

- 这个能力属于谁；
- 可以暴露哪些接口；
- 能被哪些工作区、文件夹、流程或节点使用；
- 是否需要审批、审计、限流或环境隔离。

### Interface

`Interface` 是能力契约。它定义服务能做什么、输入是什么、输出是什么、错误怎么表达。

示例：

```text
query-user-profile
evaluate-risk
read-checkin-state
upsert-checkin-state
publish-reward-command
```

接口不等于 HTTP/RPC/MQ。HTTP/RPC/MQ 是接口的实现协议，属于 Provider。

接口需要定义：

- `operation`：`CALL`、`PUBLISH`、`REQUEST_REPLY` 等；
- `inputSchemaRef`；
- `outputSchemaRef`；
- `errorSchemaRef`；
- 幂等键策略；
- 超时、重试和降级语义；
- 是否允许被 Lua/表达式调用。

### Provider

`Provider` 是接口的具体实现。一个 Interface 可以有多个 Provider。

示例：

```text
query-user-profile-http-primary
query-user-profile-redis-cache
query-user-profile-mock
reward-command-kafka-provider
checkin-state-redis-provider
```

Provider 负责说明：

- 使用什么协议：`HTTP`、`RPC`、`MQ`、`PLUGIN`、`TCP`、`LUA_ADAPTER`；
- 真实 endpoint、topic、method、method name、plugin action；
- 鉴权、密钥、header、序列化方式；
- 超时、重试、熔断、限流；
- fallback、灰度、环境绑定；
- 依赖哪些 Resource。

节点通常不直接绑定 Provider，而是绑定 `Service.Interface`，再由 `providerPolicy` 选择 Provider。只有调试、灰度或特殊活动才允许显式指定 Provider。

### Resource

`Resource` 是底层状态和基础设施。

示例：

```text
Redis
DB
DB Table
Mongo Collection
MQ Topic
Object Store
KV Namespace
Counter
Lock
```

Resource 负责说明：

- 类型和 adapter；
- 连接和密钥引用；
- namespace、key policy、schema；
- 支持的资源操作；
- 租户、工作区、文件夹、流程、节点权限；
- 审计和测试连接。

HTTP/RPC/TCP 不是 Resource 主类型，它们是 Provider 协议。MQ 比较特殊：MQ broker/topic 是 Resource，发送或消费消息是 Interface/Provider 行为。

## 推荐关系

标准关系：

```text
Node
  -> Service.Interface
  -> Provider
  -> Resource
```

直接资源操作关系：

```text
Node
  -> Resource
```

直接资源操作只适合状态、频控、锁、通用存储这类底层语义明确的节点，例如：

```text
activity-state
state-store
limit
quota
lock
lua-query
```

业务事实类能力优先走 `Service.Interface`，不要让普通业务节点直接读 DB 表。

## 对象示例

### Service

```json
{
  "code": "user-profile-service",
  "name": "用户画像服务",
  "serviceType": "EXTERNAL",
  "scope": "WORKSPACE",
  "workspaceId": "growth",
  "status": "ACTIVE",
  "owner": "user-platform",
  "tags": ["fact", "profile"]
}
```

### Interface

```json
{
  "serviceRef": "user-profile-service",
  "code": "query-user-profile",
  "name": "查询用户画像",
  "operation": "CALL",
  "inputSchemaRef": "user-profile-query-v1",
  "outputSchemaRef": "user-profile-v1",
  "idempotencyKeyTemplate": "${tenantId}:${subject.id}",
  "timeoutMs": 3000,
  "retryPolicy": "STANDARD",
  "luaCallable": true
}
```

### Provider

```json
{
  "serviceRef": "user-profile-service",
  "interfaceRef": "query-user-profile",
  "code": "query-user-profile-http-primary",
  "protocol": "HTTP",
  "endpointRef": "secret://user-profile/http-base-url",
  "method": "POST",
  "path": "/api/user/profile/query",
  "authRef": "user-profile-http-auth",
  "timeoutMs": 3000,
  "resources": [
    {
      "resourceRef": "user-profile-http-client",
      "usage": "CALL"
    }
  ]
}
```

### Resource

```json
{
  "code": "checkin-state-redis",
  "type": "REDIS",
  "mode": "KV",
  "scope": "WORKSPACE",
  "workspaceId": "growth",
  "status": "ACTIVE",
  "capabilities": ["READ", "WRITE", "UPSERT", "CAS"],
  "config": {
    "keyPrefix": "checkin:state"
  },
  "keyPolicy": {
    "allowedTemplates": ["${tenantId}:${activityId}:${subject.id}"]
  }
}
```

## 节点引用方式

### 调用服务接口

```json
{
  "providerChain": [
    {
      "targetType": "SERVICE_INTERFACE",
      "serviceRef": "user-profile-service",
      "interfaceRef": "query-user-profile",
      "operation": "CALL",
      "providerPolicy": "PRIMARY_WITH_FALLBACK",
      "inputMapping": {
        "tenantId": "${tenantId}",
        "userId": "${subject.id}"
      },
      "outputAs": "userProfile"
    }
  ]
}
```

### 操作资源

```json
{
  "providerChain": [
    {
      "targetType": "RESOURCE",
      "resourceRef": "checkin-state-redis",
      "operation": "READ",
      "namespace": "checkin",
      "keyTemplate": "${tenantId}:${activityId}:${subject.id}",
      "valuePath": "$.state",
      "outputAs": "checkinState",
      "onMiss": "BLOCK"
    }
  ]
}
```

### Lua 调用成熟服务和方法

Lua 只用于编排成熟服务、调用已注册方法、轻量数据转换、fallback 选择和输出组装。Lua 不直接访问 HTTP、Redis、DB、MQ 或密钥。

推荐暴露受控函数：

```lua
local profile = service_call("user-profile-service", "query-user-profile", {
  tenantId = ctx.tenantId,
  userId = ctx.subject.id
})

local state = resource_read(
  "checkin-state-redis",
  "checkin",
  ctx.tenantId .. ":" .. ctx.activityId .. ":" .. ctx.subject.id
)

if profile.level >= 3 and state.signedToday ~= true then
  emit({
    qualified = true,
    userProfile = profile,
    checkinState = state
  })
else
  block("NOT_QUALIFIED")
end
```

禁止暴露这类底层函数：

```lua
http.post("https://example.com/api", payload)
redis.get("raw-key")
db.query("select * from users")
mq.publish("raw-topic", payload)
```

## 操作类型

服务接口操作和资源操作应分开治理。

### 服务接口操作

| 操作 | 说明 |
| --- | --- |
| `CALL` | 同步调用 HTTP/RPC/Plugin/自定义协议 |
| `PUBLISH` | 发布消息，不等待业务结果 |
| `REQUEST_REPLY` | MQ 或异步通道上的请求响应 |
| `SUBSCRIBE_TRIGGER` | 订阅事件并触发标准事件 |

### 资源操作

| 操作 | 说明 |
| --- | --- |
| `READ` | 读取资源 |
| `WRITE` | 写入资源 |
| `UPSERT` | 不存在则创建，存在则更新 |
| `DELETE` | 删除资源项 |
| `APPEND` | 追加数组、列表或日志 |
| `INCREMENT` | 计数递增 |
| `CHECK_AND_INCREMENT` | 检查上限并原子递增 |
| `CAS` | 带版本或条件的比较更新 |
| `LOCK` | 获取锁 |
| `UNLOCK` | 释放锁 |

UI 上应根据 `targetType` 展示不同操作集合，避免把服务调用和资源读写混成一个无限扩张的 enum。

## providerChain Step 统一模型

`fact-query`、`activity-state`、`state-store`、`lua-query` 等节点可以逐步统一到同一种 step 结构。

```json
{
  "targetType": "SERVICE_INTERFACE | RESOURCE | WORKFLOW_CONTEXT | LOCAL_CACHE",
  "serviceRef": "",
  "interfaceRef": "",
  "providerRef": "",
  "providerPolicy": "PRIMARY",
  "resourceRef": "",
  "operation": "CALL",
  "namespace": "",
  "keyTemplate": "",
  "inputMapping": {},
  "valuePath": "",
  "outputAs": "",
  "onMiss": "NEXT",
  "timeoutMs": 3000
}
```

字段含义：

| 字段 | 说明 |
| --- | --- |
| `targetType` | 目标类型，决定调用服务接口还是操作资源 |
| `serviceRef` | 服务编码，`SERVICE_INTERFACE` 时必填 |
| `interfaceRef` | 接口编码，`SERVICE_INTERFACE` 时必填 |
| `providerRef` | 可选，指定具体提供者 |
| `providerPolicy` | 可选，未指定 provider 时的选择策略 |
| `resourceRef` | 资源编码，`RESOURCE` 时必填 |
| `operation` | 操作类型 |
| `namespace` | 资源命名空间或审计命名空间 |
| `keyTemplate` | 资源 key 模板 |
| `inputMapping` | 服务调用输入映射 |
| `valuePath` | 从响应或上下文读取的值路径 |
| `outputAs` | 写入节点上下文的输出名 |
| `onMiss` | 未命中时 `NEXT`、`BLOCK`、`EMPTY` |
| `timeoutMs` | 单步超时 |

## Provider 选择策略

| 策略 | 说明 |
| --- | --- |
| `PRIMARY` | 只使用主 Provider |
| `PRIMARY_WITH_FALLBACK` | 主 Provider 失败后按顺序 fallback |
| `CACHE_ASIDE` | 先读缓存 Provider，未命中再读主 Provider |
| `WEIGHTED` | 按权重灰度选择 Provider |
| `ENVIRONMENT` | 按 DEV/STAGING/PROD 绑定 Provider |
| `MOCK` | 测试或预览时使用 Mock Provider |

Provider 策略属于服务治理，不应该由节点脚本硬编码实现。

## 权限和治理

每个节点在运行前都必须通过访问校验。

服务接口访问校验：

```text
1. service 是否存在且 ACTIVE
2. interface 是否存在且 ACTIVE
3. 当前 workspace/folder/workflow 是否有调用权限
4. 当前 nodeType/nodeId 是否被允许
5. operation 是否被 interface 支持
6. input schema 是否通过
7. providerPolicy 是否能选出可用 Provider
8. 环境、限流、审批策略是否允许
```

资源访问校验：

```text
1. resource 是否存在且 ACTIVE
2. resource scope 是否覆盖当前 workflow
3. operation 是否在 resource.capabilities 内
4. namespace 是否被允许
5. keyTemplate 是否符合 key policy
6. 当前 nodeType/nodeId 是否被允许
7. 当前 actor/role 是否有执行权限
8. 环境、风险、审批策略是否允许
```

Lua/表达式访问额外限制：

- 只能调用 `luaCallable=true` 的接口；
- 只能访问节点声明过的服务接口和资源；
- 必须有超时和最大执行步数；
- 禁止网络、文件系统、系统命令；
- 每次 `service_call`、`resource_read/write` 都写审计；
- 返回值按 schema 或大小限制裁剪。

## 与核心节点的关系

推荐核心链路保持业务语义清晰：

```text
event-in -> inbox -> normalize -> match -> fact-query -> activity-state -> qualify -> reward -> limit -> quota -> participation -> grant-order -> outbox -> audit
```

节点分工：

| 节点 | 推荐使用方式 |
| --- | --- |
| `fact-query` | 调用事实类 `Service.Interface`，必要时读受控资源 |
| `activity-state` | 读写活动状态，本质是业务化的资源操作链 |
| `qualify` | 只做资格判断，不直接访问底层资源 |
| `reward` | 奖励计算和奖励项生成，复杂来源可调用奖励服务接口 |
| `limit` | 次数和频率限制，使用计数器资源或限制服务接口 |
| `quota` | 预算、库存、额度扣减，保留原子副作用边界 |
| `participation` | 参与事实写入 |
| `grant-order` | 发奖订单状态机 |
| `outbox` | HTTP/MQ/Webhook 投递边界 |
| `code/lua-query` | 编排已授权成熟能力，不实现底层连接 |

应弱化或隐藏的节点：

| 节点 | 原因 |
| --- | --- |
| `quota-check` | 被 `limit` 覆盖 |
| `candidate-reward` | 被 `reward` 覆盖 |
| `budget-reserve` | 被 `quota` 覆盖 |
| `decimal` | 更适合作为表达式/函数能力，不作为默认业务节点 |

## 第一阶段落地建议

第一阶段不要做成大而全连接器平台，只实现最小可用治理闭环。

建议支持：

1. `Service`、`Interface`、`Provider`、`Resource` 四类资产建模。
2. `HTTP` Provider。
3. `MQ PUBLISH` Provider。
4. `REDIS` Resource。
5. `TABLE` 或 `DATABASE` Resource。
6. `PLUGIN` Provider。
7. `fact-query` 和 `activity-state` 使用统一 providerChain step。
8. Lua 只通过 `service_call` 和 `resource_*` 受控函数访问能力。
9. 所有调用有权限、超时、审计和 schema 校验。

## 后续改造范围

本节只定义后续代码改造要求，不代表一次性全部实现。改造目标是先把能力资产模型稳定下来，再逐步替换节点里的底层连接配置。

### P0 文档和术语收敛

先统一产品、后端、前端和文档里的命名。

| 概念 | 推荐名称 | 不推荐混用 |
| --- | --- | --- |
| 服务 | `Service` | 把服务叫资源 |
| 接口 | `Interface` | 把 HTTP/RPC/MQ 直接叫资源 |
| 提供者 | `Provider` | 把 Provider 当成业务能力本身 |
| 资源 | `Resource` | 把 HTTP endpoint 当成资源主类型 |
| 节点步骤 | `ProviderChainStep` | 每个节点自定义一套 step 字段 |

文档需要同步：

- 资源管理文档只描述 `Resource` 治理；
- 节点目录文档说明节点如何引用服务接口和资源；
- fact-query、activity-state、lua-query 文档说明统一 providerChain；
- core node contract 明确主链路节点不直接暴露底层连接。

### P1 数据模型改造

后端需要新增或扩展四类资产模型。

```text
ServiceDefinition
ServiceInterface
ServiceProvider
ResourceDefinition
```

建议最小字段：

| 模型 | 必要字段 |
| --- | --- |
| `ServiceDefinition` | `code`、`name`、`serviceType`、`scope`、`workspaceId`、`folderId`、`status`、`owner` |
| `ServiceInterface` | `serviceRef`、`code`、`operation`、`inputSchemaRef`、`outputSchemaRef`、`luaCallable`、`timeoutMs` |
| `ServiceProvider` | `serviceRef`、`interfaceRef`、`code`、`protocol`、`endpoint/topic/method/path`、`authRef`、`resourceRefs`、`status` |
| `ResourceDefinition` | `code`、`type`、`mode`、`scope`、`capabilities`、`config`、`keyPolicy`、`status` |

兼容要求：

- 现有 `resourceRef` 不迁移删除；
- 现有资源 API 可以继续服务 Resource；
- 旧节点里的 `source`、`endpoint`、`providerCode`、`redisRef` 只作为兼容字段读取；
- 新 draft 和新模板不再主动生成底层连接字段。

### P2 Admin API 改造

新增能力资产管理 API。

```text
GET    /admin/v1/services
POST   /admin/v1/services
PATCH  /admin/v1/services/{serviceCode}

GET    /admin/v1/services/{serviceCode}/interfaces
POST   /admin/v1/services/{serviceCode}/interfaces
PATCH  /admin/v1/services/{serviceCode}/interfaces/{interfaceCode}

GET    /admin/v1/services/{serviceCode}/interfaces/{interfaceCode}/providers
POST   /admin/v1/services/{serviceCode}/interfaces/{interfaceCode}/providers
PATCH  /admin/v1/services/{serviceCode}/interfaces/{interfaceCode}/providers/{providerCode}

POST   /admin/v1/services/{serviceCode}/interfaces/{interfaceCode}/test
POST   /admin/v1/resources/{resourceCode}/test
```

API 返回需要满足 Studio 联动选择：

```text
选择 Service -> 可选 Interface
选择 Interface -> 可选 ProviderPolicy / Provider
选择 Resource -> 可选 operation / namespace / key policy
```

### P3 providerChain Schema 统一

节点配置统一使用 `ProviderChainStep`。

```json
{
  "targetType": "SERVICE_INTERFACE",
  "serviceRef": "user-profile-service",
  "interfaceRef": "query-user-profile",
  "providerPolicy": "PRIMARY_WITH_FALLBACK",
  "operation": "CALL",
  "inputMapping": {},
  "outputAs": "userProfile"
}
```

```json
{
  "targetType": "RESOURCE",
  "resourceRef": "checkin-state-redis",
  "operation": "READ",
  "namespace": "checkin",
  "keyTemplate": "${tenantId}:${activityId}:${subject.id}",
  "valuePath": "$.state",
  "outputAs": "checkinState"
}
```

需要优先改造：

| 节点 | 改造要求 |
| --- | --- |
| `fact-query` | 支持 `SERVICE_INTERFACE` 和 `RESOURCE` step |
| `activity-state` | 使用 `RESOURCE` step 表达状态读写 |
| `lua-query` / `code` | 只能调用已授权服务接口和资源 |
| `state-store` | 保留底层资源操作，但新配置优先 `resourceRef` |

暂不要求改造：

- `qualify`、`participation`、`grant-order`、`outbox` 的核心事务语义；
- 已隐藏但兼容的 `quota-check`、`candidate-reward`、`budget-reserve`；
- 旧 workflow definition 的执行兼容。

### P4 Executor 改造

新增统一执行链，不要让每个节点各自实现一套服务/资源调用。

```text
ProviderChainExecutor
  -> ServiceInterfaceInvoker
  -> ProviderResolver
  -> ProviderAdapter
  -> ResourceOperator
  -> AuditWriter
```

执行要求：

1. 解析 step。
2. 根据 `targetType` 分流。
3. `SERVICE_INTERFACE` 校验服务和接口权限。
4. `ProviderResolver` 根据策略选择 Provider。
5. `ProviderAdapter` 执行 HTTP、MQ、Plugin 或其他协议。
6. `RESOURCE` 校验资源权限和 capability。
7. `ResourceOperator` 执行 Redis、DB、Table、KV 等操作。
8. 标准化输出并写入 `outputAs`。
9. 每一步写调用审计和运行 trace。

第一阶段 Adapter 只需要：

| Adapter | 范围 |
| --- | --- |
| `HTTP Provider` | 同步 CALL |
| `MQ Publish Provider` | PUBLISH，不等待最终业务结果 |
| `Redis Resource` | KV、Counter、Lock |
| `DB/Table Resource` | READ、UPSERT |
| `Plugin Provider` | 调用已注册插件 action |

### P5 Lua 和表达式改造

Lua 的定位是调用成熟服务和方法，不实现底层连接。

允许函数：

```lua
service_call(serviceRef, interfaceRef, input)
resource_read(resourceRef, namespace, key)
resource_write(resourceRef, namespace, key, value)
resource_upsert(resourceRef, namespace, key, value)
mq_publish(serviceRef, interfaceRef, payload)
```

禁止函数：

```lua
http.post(...)
redis.get(...)
db.query(...)
mq.raw_publish(...)
os.execute(...)
io.open(...)
```

运行时要求：

- 节点必须声明 `allowedServices` 和 `allowedResources`；
- `interface.luaCallable` 必须为 true；
- resource operation 必须在 capability 内；
- Lua 有超时、最大执行步数、最大输出大小；
- 所有调用进入审计和 run history；
- 表达式只用于字段映射、条件判断、keyTemplate 和 valuePath，不承担复杂副作用。

### P6 Studio 前端改造

资源管理页升级为能力资产中心。

```text
能力资产中心
  - Services
  - Interfaces
  - Providers
  - Resources
```

节点配置面板改造：

| 功能 | 要求 |
| --- | --- |
| providerChain 编辑器 | 支持 `targetType` 切换 |
| Service 选择 | 选择 Service 后联动 Interface |
| Interface 选择 | 展示 operation、schema、Provider 策略 |
| Resource 选择 | 展示 capability、namespace、key policy |
| Lua 面板 | 展示当前节点可调用服务和资源 |
| 校验提示 | 未授权、schema 不匹配、operation 不支持时直接提示 |

Studio 不应该让用户在节点里手填：

```text
HTTP URL
Redis 地址
DB table
MQ broker
secret
```

### P7 迁移和兼容

迁移策略必须保守。

| 类型 | 策略 |
| --- | --- |
| 旧 draft | 继续可打开、可保存、可执行 |
| 旧字段 | 执行器兼容读取，UI 不再主推 |
| 旧节点 | 隐藏新建入口，保留 schema Get 和 handler |
| 新模板 | 只生成新 providerChain 和资产引用 |
| 数据迁移 | 先双读兼容，再提供显式迁移工具 |

推荐迁移顺序：

```text
1. 新增模型和 API
2. 新 UI 支持选择资产
3. 新模板使用新模型
4. Executor 双读旧字段和新 step
5. 提供 draft migration preview
6. 迁移完成后再考虑移除旧 UI 字段
```

### P8 验收标准

改造完成后至少满足：

1. 新建流程不需要手填 endpoint、Redis 地址、DB 表或 MQ broker。
2. `fact-query` 可以通过 Service.Interface 查询用户事实。
3. `activity-state` 可以通过 Resource step 读取和写入签到状态。
4. Lua 只能调用已授权服务接口和资源。
5. 所有服务接口调用和资源操作都有审计。
6. 未授权资源、未授权接口、operation 不支持会在校验阶段失败。
7. 老 workflow 仍可打开和执行。
8. Studio 中服务、接口、提供者、资源的选择链路清晰。
9. 核心链路不因能力资产改造改变事务边界。

## 设计结论

这个模型符合 ExMarketing 的定位：

```text
Workspace / Folder / Workflow / Node = 业务组织和流程编排
Service / Interface / Provider / Resource = 技术能力资产和治理
```

最终目标不是让运营在节点里配置技术连接，而是让运营编排成熟能力：

```text
节点负责编排
服务承载能力
接口定义契约
提供者负责实现
资源承载状态
执行器负责权限、沙箱、审计和运行时治理
```
