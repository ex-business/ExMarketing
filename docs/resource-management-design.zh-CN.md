# ExMarketing 公共资源管理设计

本文档设计 ExMarketing 的公共资源管理能力，用于统一管理工作流节点需要访问的内存、数据库、公共表、Redis、HTTP/gRPC Provider、MQ、对象存储和插件资源。

核心原则：

```text
节点表达业务编排，不表达基础设施连接。
资源管理表达资源定义、连接、权限、schema、密钥、审计和 adapter。
```

节点里不应该出现数据库表名、Redis 地址、HTTP endpoint、token、headers、driver、secret、物理 topic 等基础设施配置。节点只引用 `resourceRef`，并声明要执行的业务操作。

## 目标

1. 统一资源入口：所有节点通过 `resourceRef` 使用资源。
2. 统一权限控制：资源可限制到租户、工作区、文件夹、流程、节点类型和操作。
3. 统一 schema：公共表、KV、对象、数组、消息 payload 都有结构定义和版本。
4. 统一密钥管理：endpoint token、Redis 密码、数据库凭据不进入 workflow definition。
5. 统一审计：每次资源调用记录谁、在哪个流程、哪个节点、做了什么操作、结果如何。
6. 统一 adapter：不同后端由 adapter 执行，节点不关心 Postgres、Redis、HTTP、gRPC 的细节。
7. 支持环境切换：开发、测试、生产使用相同逻辑资源名，绑定不同真实资源。

## 当前问题

`state-store` 当前已支持多种存储类型，但部分资源细节仍可出现在节点配置里，例如：

```json
{
  "storageType": "HTTP",
  "providerEndpoint": "https://internal/storage",
  "requestHeaders": {},
  "tableSchema": {}
}
```

这只是过渡形态。最终应改成：

```json
{
  "resourceRef": "workflow_state_store",
  "operation": "UPSERT",
  "namespace": "checkin",
  "keyTemplate": "${activityId}:${subject.id}",
  "valueType": "OBJECT",
  "valuePath": "$.input.value"
}
```

资源真实定义放到资源管理：

```json
{
  "code": "workflow_state_store",
  "type": "DATABASE_KV",
  "driver": "POSTGRES",
  "status": "ACTIVE",
  "capabilities": ["READ", "WRITE", "UPSERT", "APPEND", "DELETE", "CAS"],
  "config": {
    "table": "marketing_storage_state"
  }
}
```

## 总体架构

```text
Workflow Node
  -> resourceRef
  -> Resource Registry
  -> Permission Policy
  -> Schema Contract
  -> Secret Resolver
  -> Resource Adapter
  -> Real Backend
  -> Resource Audit
```

执行器步骤：

1. 节点读取 `resourceRef`。
2. 查询资源定义。
3. 校验资源状态、能力、schema、权限。
4. 解析密钥引用。
5. 选择 adapter。
6. 执行资源操作。
7. 标准化输出。
8. 写资源调用审计。

## 资源类型

| 类型 | 说明 | 典型用途 |
| --- | --- | --- |
| `MEMORY` | 当前 execution 内存资源 | 临时变量、调试中间结果 |
| `WORKFLOW_CONTEXT` | 工作流上下文资源 | 流程公共参数、节点间共享数据 |
| `DATABASE_KV` | 数据库 KV/JSON 存储 | `marketing_storage_state`、活动状态 |
| `DATABASE_TABLE` | 数据库公共表 | 签到用户表、奖励记录表、白名单表 |
| `REDIS_KV` | Redis KV | 用户状态缓存、活动配置缓存 |
| `REDIS_HASH` | Redis Hash | 用户资料、活动进度 |
| `REDIS_COUNTER` | Redis 计数器 | 频控、次数限制 |
| `REDIS_LOCK` | Redis 锁 | 用户活动互斥、并发保护 |
| `HTTP_PROVIDER` | HTTP 外部服务 | 用户中心、钱包、风控、通知服务 |
| `GRPC_PROVIDER` | gRPC 外部服务 | 高性能内部服务 |
| `MQ_TOPIC` | 消息资源 | Kafka/RabbitMQ/Pulsar topic |
| `OBJECT_STORAGE` | 文件/对象存储 | 导出文件、执行附件、报表 |
| `PLUGIN_PROVIDER` | 插件资源 | 活动插件、业务扩展 |
| `CUSTOM_PROVIDER` | 自定义协议资源 | Lua/Go 扩展协议、专有系统 |

## 资源能力

每个资源必须声明可支持的操作能力。

```json
{
  "capabilities": [
    "READ",
    "WRITE",
    "UPSERT",
    "APPEND",
    "DELETE",
    "CAS",
    "INCREMENT",
    "CHECK_AND_INCREMENT",
    "LOCK",
    "UNLOCK",
    "PUBLISH",
    "CALL",
    "TEST_CONNECTION"
  ]
}
```

节点运行前必须校验：

```text
node.operation ∈ resource.capabilities
```

如果不满足，节点失败：

```text
RESOURCE_CAPABILITY_DENIED
```

## 资源作用域

资源需要有明确作用域，避免全局资源被误用。

| 作用域 | 说明 |
| --- | --- |
| `SYSTEM` | 系统内置资源，例如默认 `marketing_storage_state` |
| `TENANT` | 租户级资源，租户下多个工作区可共用 |
| `WORKSPACE` | 工作区资源，只能该工作区使用 |
| `FOLDER` | 文件夹资源，只能该文件夹及可选子文件夹使用 |
| `ACTIVITY` | 活动/流程资源，只能指定活动使用 |
| `NODE` | 节点专用资源，通常用于高风险资源或临时资源 |

资源定义示例：

```json
{
  "code": "checkin_user_state",
  "type": "DATABASE_TABLE",
  "scope": "FOLDER",
  "tenantId": "site_001",
  "workspaceId": "hello",
  "folderId": "growth-checkin",
  "status": "ACTIVE"
}
```

## 权限模型

资源权限不要只靠资源作用域，必须有显式绑定策略。

### 权限维度

资源访问至少按这些维度判断：

| 维度 | 示例 |
| --- | --- |
| 租户 | `tenantId = site_001` |
| 工作区 | 只能 `hello` 工作区 |
| 文件夹 | 只能 `folder_checkin` |
| 流程/活动 | 只能 `act_daily_checkin` |
| 节点类型 | 只能 `state-store`、`fact-query` |
| 节点 ID | 只能 `store-checkin-state` |
| 操作 | 只能 `READ`、`UPSERT` |
| namespace | 只能 `checkin` |
| key 前缀 | 只能 `${tenantId}:${activityId}:*` |
| 环境 | 只能 `DEV`、`STAGING`、`PROD` |
| 角色 | `OWNER/ADMIN/EDITOR/VIEWER/GUEST` |

### 权限判定顺序

```text
1. 资源是否存在
2. 资源状态是否 ACTIVE
3. 租户是否匹配
4. 资源作用域是否覆盖当前 workflow
5. 当前 actor 是否有资源查看/编辑/执行权限
6. 当前节点类型是否被允许
7. 当前 operation 是否被允许
8. namespace/key 是否被允许
9. 环境策略是否允许
10. 风险策略是否需要二次确认或审批
```

### Binding 示例

工作区级资源：

```json
{
  "resourceRef": "workflow_state_store",
  "scope": "WORKSPACE",
  "workspaceId": "hello",
  "allowedNodeTypes": ["state-store", "activity-state"],
  "allowedOperations": ["READ", "UPSERT", "APPEND"],
  "allowedNamespaces": ["workflow", "checkin"],
  "minRole": "EDITOR",
  "status": "ACTIVE"
}
```

文件夹级资源：

```json
{
  "resourceRef": "checkin_user_state",
  "scope": "FOLDER",
  "workspaceId": "hello",
  "folderId": "daily-checkin",
  "includeSubfolders": true,
  "allowedNodeTypes": ["state-store", "fact-query"],
  "allowedOperations": ["READ", "UPSERT"],
  "allowedNamespaces": ["checkin"],
  "minRole": "EDITOR"
}
```

流程级资源：

```json
{
  "resourceRef": "wallet_reward_provider",
  "scope": "ACTIVITY",
  "activityId": "act_daily_checkin",
  "allowedNodeTypes": ["grant-order"],
  "allowedOperations": ["CALL"],
  "minRole": "ADMIN",
  "requireApproval": true
}
```

只读事实资源：

```json
{
  "resourceRef": "user_profile_readonly",
  "scope": "WORKSPACE",
  "workspaceId": "hello",
  "allowedNodeTypes": ["fact-query", "qualify", "lua-query"],
  "allowedOperations": ["READ"],
  "minRole": "VIEWER"
}
```

### 角色权限建议

| 角色 | 资源列表 | 创建资源 | 编辑资源 | 绑定资源 | 使用资源 | 查看审计 | 管理密钥 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `OWNER` | 是 | 是 | 是 | 是 | 是 | 是 | 是 |
| `ADMIN` | 是 | 是 | 是 | 是 | 是 | 是 | 受限 |
| `EDITOR` | 是 | 否 | 否 | 否 | 是 | 只看自己流程 | 否 |
| `VIEWER` | 是 | 否 | 否 | 否 | 只读资源 | 只读 | 否 |
| `GUEST` | 受限 | 否 | 否 | 否 | 否 | 否 | 否 |

注意：`VIEWER` 即使能打开 workflow，也不能执行 `WRITE/UPSERT/DELETE/CALL/PUBLISH` 类资源操作。

## 资源数据模型

### marketing_resource

资源主表。

```sql
CREATE TABLE marketing_resource (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    workspace_id TEXT NOT NULL DEFAULT '',
    folder_id TEXT NOT NULL DEFAULT '',
    activity_id TEXT NOT NULL DEFAULT '',
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    type TEXT NOT NULL,
    driver TEXT NOT NULL,
    scope TEXT NOT NULL,
    status TEXT NOT NULL,
    environment TEXT NOT NULL DEFAULT '',
    config_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    capabilities_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    schema_version INTEGER NOT NULL DEFAULT 1,
    created_by TEXT NOT NULL DEFAULT '',
    updated_by TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    UNIQUE (tenant_id, code)
);
```

### marketing_resource_schema

资源 schema 表，支持公共表、KV、消息和 provider payload。

```sql
CREATE TABLE marketing_resource_schema (
    id TEXT PRIMARY KEY,
    resource_id TEXT NOT NULL,
    version INTEGER NOT NULL,
    schema_type TEXT NOT NULL,
    schema_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    status TEXT NOT NULL,
    created_by TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE (resource_id, version)
);
```

### marketing_resource_binding

资源授权绑定。

```sql
CREATE TABLE marketing_resource_binding (
    id TEXT PRIMARY KEY,
    resource_id TEXT NOT NULL,
    tenant_id TEXT NOT NULL,
    workspace_id TEXT NOT NULL DEFAULT '',
    folder_id TEXT NOT NULL DEFAULT '',
    activity_id TEXT NOT NULL DEFAULT '',
    node_type TEXT NOT NULL DEFAULT '',
    node_id TEXT NOT NULL DEFAULT '',
    principal_type TEXT NOT NULL DEFAULT '',
    principal_id TEXT NOT NULL DEFAULT '',
    min_role TEXT NOT NULL DEFAULT '',
    allowed_operations_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    allowed_namespaces_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    key_policy_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    include_subfolders BOOLEAN NOT NULL DEFAULT false,
    require_approval BOOLEAN NOT NULL DEFAULT false,
    status TEXT NOT NULL,
    created_by TEXT NOT NULL DEFAULT '',
    updated_by TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);
```

### marketing_resource_secret

资源密钥引用表。第一阶段只保存 `secretRef`，不直接保存明文；后续接 Vault/KMS。

```sql
CREATE TABLE marketing_resource_secret (
    id TEXT PRIMARY KEY,
    resource_id TEXT NOT NULL,
    secret_ref TEXT NOT NULL,
    secret_type TEXT NOT NULL,
    encrypted_payload BYTEA,
    status TEXT NOT NULL,
    created_by TEXT NOT NULL DEFAULT '',
    updated_by TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);
```

### marketing_resource_audit

资源调用审计。

```sql
CREATE TABLE marketing_resource_audit (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    workspace_id TEXT NOT NULL DEFAULT '',
    folder_id TEXT NOT NULL DEFAULT '',
    activity_id TEXT NOT NULL DEFAULT '',
    workflow_execution_id TEXT NOT NULL DEFAULT '',
    node_id TEXT NOT NULL DEFAULT '',
    node_type TEXT NOT NULL DEFAULT '',
    resource_id TEXT NOT NULL,
    resource_code TEXT NOT NULL,
    operation TEXT NOT NULL,
    status TEXT NOT NULL,
    duration_ms INTEGER NOT NULL DEFAULT 0,
    error_code TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    request_hash TEXT NOT NULL DEFAULT '',
    response_hash TEXT NOT NULL DEFAULT '',
    actor_id TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL
);
```

## 资源配置示例

### 默认数据库 KV

```json
{
  "code": "default_workflow_state",
  "name": "默认工作流状态存储",
  "type": "DATABASE_KV",
  "driver": "POSTGRES",
  "scope": "SYSTEM",
  "status": "ACTIVE",
  "capabilities": ["READ", "WRITE", "UPSERT", "APPEND", "DELETE", "CAS"],
  "config": {
    "table": "marketing_storage_state"
  }
}
```

### 签到公共表

```json
{
  "code": "checkin_user_state",
  "name": "签到用户状态表",
  "type": "DATABASE_TABLE",
  "driver": "POSTGRES",
  "scope": "FOLDER",
  "workspaceId": "hello",
  "folderId": "daily-checkin",
  "status": "ACTIVE",
  "capabilities": ["READ", "UPSERT", "CAS"],
  "config": {
    "physicalTable": "marketing_checkin_user_state",
    "primaryKey": ["tenantId", "activityId", "userId"]
  },
  "schema": {
    "columns": [
      {"name": "tenantId", "type": "STRING", "required": true},
      {"name": "activityId", "type": "STRING", "required": true},
      {"name": "userId", "type": "STRING", "required": true},
      {"name": "streakDays", "type": "NUMBER"},
      {"name": "lastCheckinDate", "type": "DATE"},
      {"name": "updatedAt", "type": "DATETIME"}
    ]
  }
}
```

### 用户资料 HTTP Provider

```json
{
  "code": "user_profile_provider",
  "name": "用户资料查询服务",
  "type": "HTTP_PROVIDER",
  "driver": "REST",
  "scope": "WORKSPACE",
  "workspaceId": "hello",
  "status": "ACTIVE",
  "capabilities": ["READ", "CALL", "TEST_CONNECTION"],
  "config": {
    "endpoint": "https://internal.example.com/user/profile/query",
    "method": "POST",
    "timeoutSeconds": 3,
    "headersRef": "secret:user-profile-provider"
  }
}
```

### Redis 计数器

```json
{
  "code": "daily_checkin_counter",
  "name": "每日签到次数计数器",
  "type": "REDIS_COUNTER",
  "driver": "REDIS",
  "scope": "FOLDER",
  "workspaceId": "hello",
  "folderId": "daily-checkin",
  "status": "ACTIVE",
  "capabilities": ["READ", "INCREMENT", "CHECK_AND_INCREMENT"],
  "config": {
    "redisRef": "marketing-redis",
    "keyPrefix": "marketing:checkin:counter",
    "ttlPolicy": "WINDOW_END"
  }
}
```

## 节点接入设计

所有需要资源的节点统一使用以下字段：

```json
{
  "resourceRef": "some_resource",
  "operation": "READ",
  "inputMapping": {},
  "outputMapping": {},
  "failurePolicy": {},
  "runtimePolicy": {}
}
```

### state-store

存储节点只表达读写语义。

```json
{
  "type": "state-store",
  "resourceRef": "default_workflow_state",
  "operation": "UPSERT",
  "namespace": "checkin",
  "keyTemplate": "${tenantId}:${activityId}:${subject.id}",
  "valueType": "OBJECT",
  "valuePath": "$.input.value",
  "requireVersion": true
}
```

不允许继续放：

```json
{
  "providerEndpoint": "...",
  "requestHeaders": {},
  "tableSchema": {},
  "redisRef": "..."
}
```

### fact-query

事实查询节点只表达查询意图。

```json
{
  "type": "fact-query",
  "resourceRef": "user_profile_provider",
  "operation": "READ",
  "queryMapping": {
    "tenantId": "${tenantId}",
    "userId": "${subject.id}"
  },
  "outputPath": "facts.userProfile",
  "missingPolicy": "BLOCK"
}
```

### quota-check

频控节点引用计数器资源。

```json
{
  "type": "quota-check",
  "resourceRef": "daily_checkin_counter",
  "operation": "CHECK_AND_INCREMENT",
  "scopeKeyTemplate": "${tenantId}:${activityId}:${subject.id}",
  "window": "DAY",
  "maxCount": 1,
  "exceedAction": "BLOCK"
}
```

### grant-order

发奖节点可以引用内部订单资源，也可以引用外部钱包 provider。

```json
{
  "type": "grant-order",
  "resourceRef": "wallet_reward_provider",
  "operation": "CALL",
  "idempotencyKeyTemplate": "${eventId}:${activityId}:${subject.id}",
  "payloadMapping": {
    "userId": "${subject.id}",
    "rewardItems": "$.outputs.reward.reward_items"
  }
}
```

### outbox / callback

通知、回调、MQ 节点引用消息或 HTTP provider 资源。

```json
{
  "type": "outbox",
  "resourceRef": "notification_topic",
  "operation": "PUBLISH",
  "payloadMapping": {
    "userId": "${subject.id}",
    "templateCode": "CHECKIN_SUCCESS",
    "reward": "$.outputs.reward.reward_items"
  }
}
```

### code / lua

Lua 不允许直接打开数据库、Redis 或 HTTP。只能通过受控 resource API。

```lua
local profile = resource_read("user_profile_provider", {
  userId = ctx.event.subject.id
})

resource_upsert("checkin_user_state", {
  tenantId = ctx.event.tenantId,
  activityId = ctx.activity.id,
  userId = ctx.event.subject.id
}, {
  streakDays = 2
})
```

执行前仍然走同一套权限：

```text
Lua 节点类型是否允许访问 resourceRef
Lua 节点是否允许 WRITE/DELETE/CALL
当前活动是否有该资源 binding
```

## Adapter 接口

后端建议抽象统一接口。

```go
type ResourceAdapter interface {
    Type() string
    Capabilities() []string
    Test(ctx context.Context, resource Resource) error
    Execute(ctx context.Context, request ResourceRequest) (ResourceResponse, error)
}
```

请求结构：

```json
{
  "tenantId": "site_001",
  "workspaceId": "hello",
  "folderId": "daily-checkin",
  "activityId": "act_daily_checkin",
  "executionId": "exec_xxx",
  "nodeId": "store_checkin_state",
  "nodeType": "state-store",
  "resourceRef": "checkin_user_state",
  "operation": "UPSERT",
  "namespace": "checkin",
  "key": {},
  "value": {},
  "expectedVersion": 1,
  "requireVersion": true
}
```

响应结构：

```json
{
  "value": {},
  "version": 2,
  "exists": true,
  "metadata": {
    "resourceType": "DATABASE_TABLE",
    "adapter": "POSTGRES",
    "durationMs": 12
  }
}
```

## UI 设计

### 信息架构

建议在管理台新增一级入口：

```text
资源管理
  ├── 资源列表
  ├── 新建资源
  ├── 资源详情
  ├── Schema
  ├── 权限绑定
  ├── 密钥
  ├── 测试连接
  └── 调用日志
```

也可以先放在工作区首页侧栏：

```text
工作区
  ├── 流程
  ├── 模板
  ├── 资源
  ├── 成员
  └── 审计
```

### 资源列表

列表字段：

| 字段 | 说明 |
| --- | --- |
| 名称 | 资源中文名 |
| Code | `resourceRef` |
| 类型 | DATABASE_TABLE、HTTP_PROVIDER 等 |
| 作用域 | SYSTEM/TENANT/WORKSPACE/FOLDER/ACTIVITY |
| 所属位置 | 工作区 / 文件夹 / 活动 |
| 状态 | ACTIVE/DISABLED/DRAFT/DELETED |
| 能力 | READ/UPSERT/CALL/PUBLISH |
| 最近调用 | 时间和结果 |
| 风险等级 | LOW/MEDIUM/HIGH |

筛选项：

```text
工作区、文件夹、类型、状态、能力、创建人、最近错误、可用范围
```

### 新建资源向导

步骤：

1. 选择资源类型。
2. 填写基础信息。
3. 选择作用域。
4. 填写连接配置。
5. 配置 schema。
6. 配置权限绑定。
7. 测试连接。
8. 发布启用。

资源类型选择页：

```text
数据库 KV
数据库公共表
Redis KV
Redis 计数器
Redis 锁
HTTP Provider
gRPC Provider
MQ Topic
对象存储
插件资源
```

### 资源详情页

Tabs：

```text
概览
配置
Schema
权限
密钥
测试
调用日志
引用关系
版本
```

概览展示：

```text
资源 Code
资源类型
Driver
作用域
状态
能力
绑定数量
被哪些流程引用
最近 24h 调用量
错误率
P95 延迟
```

### 权限绑定 UI

绑定策略要可视化，不建议只让用户写 JSON。

表单字段：

```text
可用范围：
  - 全租户
  - 指定工作区
  - 指定文件夹
  - 指定流程

文件夹策略：
  - 仅当前文件夹
  - 包含子文件夹

允许节点：
  - state-store
  - fact-query
  - quota-check
  - grant-order
  - outbox
  - callback
  - code/lua

允许操作：
  - READ
  - UPSERT
  - APPEND
  - DELETE
  - CALL
  - PUBLISH

最低角色：
  - OWNER
  - ADMIN
  - EDITOR
  - VIEWER

高风险选项：
  - 需要审批
  - 禁止生产调试调用
  - 禁止 DELETE
  - 限制 key 前缀
```

### 节点配置 UI

节点面板不再出现资源连接字段。

节点配置只显示：

```text
资源：下拉选择 resourceRef
操作：根据资源 capabilities 动态展示
命名空间：namespace
Key 映射：keyTemplate 或 keyMapping
Value 映射：valuePath 或 valueMapping
缺失策略：EMPTY / BLOCK / SKIP
版本策略：requireVersion / expectedVersion
失败策略：BLOCK / SKIP / RETRY / USE_EMPTY
```

资源下拉要按当前活动上下文过滤：

```text
当前 actor 可见
当前 workspace/folder/activity 可用
当前节点类型可用
当前操作可用
状态 ACTIVE
```

不可用资源可以展示但禁用，并给出原因：

```text
该资源仅限 daily-checkin 文件夹使用
该资源只允许 fact-query 节点 READ
当前角色 VIEWER 不能使用 UPSERT
资源已停用
```

### 测试连接 UI

不同资源测试内容不同：

| 类型 | 测试项 |
| --- | --- |
| DATABASE_KV | 表存在、读写权限、JSONB 支持 |
| DATABASE_TABLE | 表存在、schema 匹配、主键可用 |
| REDIS | 连接、认证、读写测试、TTL 测试 |
| HTTP_PROVIDER | endpoint 可达、鉴权、响应 schema |
| MQ_TOPIC | topic 存在、生产权限 |
| OBJECT_STORAGE | bucket 存在、上传/下载权限 |

测试结果要写审计，但不能写业务数据。

### 调用日志 UI

调用日志字段：

```text
时间
流程
节点
资源
操作
状态
耗时
错误码
调用人/执行来源
request hash
response hash
trace id
```

敏感 payload 默认不展示，只展示脱敏摘要。

## API 设计

### 资源 CRUD

```text
GET    /admin/v1/resources
POST   /admin/v1/resources
GET    /admin/v1/resources/{id}
PATCH  /admin/v1/resources/{id}
DELETE /admin/v1/resources/{id}
```

### 资源 schema

```text
GET    /admin/v1/resources/{id}/schemas
POST   /admin/v1/resources/{id}/schemas
PATCH  /admin/v1/resources/{id}/schemas/{version}
```

### 资源绑定

```text
GET    /admin/v1/resources/{id}/bindings
POST   /admin/v1/resources/{id}/bindings
PATCH  /admin/v1/resources/{id}/bindings/{bindingId}
DELETE /admin/v1/resources/{id}/bindings/{bindingId}
```

### 可用资源查询

节点配置下拉使用：

```text
GET /admin/v1/resources/available?workspaceId=...&folderId=...&activityId=...&nodeType=state-store&operation=UPSERT
```

返回：

```json
{
  "items": [
    {
      "code": "default_workflow_state",
      "name": "默认工作流状态存储",
      "type": "DATABASE_KV",
      "capabilities": ["READ", "UPSERT", "APPEND"],
      "available": true
    },
    {
      "code": "wallet_reward_provider",
      "name": "钱包发奖服务",
      "type": "HTTP_PROVIDER",
      "available": false,
      "unavailableReason": "当前节点类型 state-store 不允许 CALL"
    }
  ]
}
```

### 测试连接

```text
POST /admin/v1/resources/{id}/test
```

### 调用日志

```text
GET /admin/v1/resources/{id}/audit-logs
GET /admin/v1/resource-audit-logs?workspaceId=...&activityId=...
```

## 执行期校验

执行器要在每次资源调用前做强校验。

伪代码：

```go
resource := registry.Get(resourceRef)
if resource.Status != ACTIVE {
    return RESOURCE_DISABLED
}
if !resource.Supports(operation) {
    return RESOURCE_CAPABILITY_DENIED
}
if !policy.CanUse(actor, workflowContext, node, resource, operation, namespace, key) {
    return RESOURCE_PERMISSION_DENIED
}
adapter := adapters.Resolve(resource.Type, resource.Driver)
response := adapter.Execute(ctx, request)
audit.Write(request, response)
```

错误码：

| 错误码 | 含义 |
| --- | --- |
| `RESOURCE_NOT_FOUND` | 资源不存在 |
| `RESOURCE_DISABLED` | 资源未启用 |
| `RESOURCE_SCOPE_DENIED` | 当前流程不在资源作用域内 |
| `RESOURCE_PERMISSION_DENIED` | 当前 actor/节点/操作无权使用 |
| `RESOURCE_CAPABILITY_DENIED` | 资源不支持该操作 |
| `RESOURCE_SCHEMA_INVALID` | 输入输出不满足 schema |
| `RESOURCE_SECRET_UNAVAILABLE` | 密钥不可用 |
| `RESOURCE_ADAPTER_NOT_REGISTERED` | adapter 未注册 |
| `RESOURCE_CALL_FAILED` | 真实调用失败 |
| `RESOURCE_TIMEOUT` | 调用超时 |

## 安全要求

1. workflow definition 里禁止保存 secret、token、password、完整 endpoint credential。
2. 资源配置里的敏感字段必须使用 `secretRef`。
3. 默认禁止 `DELETE`、`PUBLISH`、`CALL` 类高风险操作，必须显式授权。
4. 生产环境资源默认禁止 draft debug 写入，除非开启 `allowDebugWrite`。
5. 资源调用日志必须脱敏，默认只保存 hash 和摘要。
6. 资源 schema 变更需要版本化。
7. 资源停用后，已发布 workflow 运行时应失败并给明确错误，不应静默 fallback。
8. HTTP Provider 必须有 allowlist 或资源注册审批，不能任意 SSRF。
9. Lua/Code 节点只能通过受控 resource API 访问资源，不能直接访问网络和数据库。

## 和现有能力的关系

| 现有能力 | 资源化后 |
| --- | --- |
| `state-store.storageType` | 改为 `resourceRef` 指向资源类型 |
| `state-store.providerEndpoint` | 移入 `HTTP_PROVIDER.config.endpoint` |
| `state-store.tableSchema` | 移入 `marketing_resource_schema` |
| `fact-query` 的 HTTP/Redis/gRPC 配置 | 改为 `resourceRef` |
| `outbox` / `callback` endpoint/topic | 改为资源引用 |
| Lua 直接读节点输出 | 保留 |
| Lua 访问外部资源 | 通过 `resource_read/resource_write` 受控 API |
| workspace/folder/activity ACL | 作为资源 binding 的权限输入 |

## 分阶段实施计划

### P0：资源中心基础落地

目标：先把资源定义从节点配置里移出来。

任务：

1. 新增数据模型：
   - `marketing_resource`
   - `marketing_resource_schema`
   - `marketing_resource_binding`
   - `marketing_resource_audit`
2. 新增 Resource Registry：
   - `GetResourceByCode`
   - `ListAvailableResources`
   - `ValidateResourceAccess`
3. 内置系统资源：
   - `default_workflow_state` -> `DATABASE_KV` -> `marketing_storage_state`
   - `workflow_memory` -> `MEMORY`
4. 改造 `state-store`：
   - 新增 `resourceRef`
   - 优先使用 `resourceRef`
   - `storageType/providerEndpoint/tableSchema` 标记 deprecated
5. 新增资源管理 API：
   - 资源列表
   - 资源详情
   - 可用资源查询
   - 测试连接
6. 节点配置 UI：
   - 资源下拉
   - 操作根据 capabilities 动态过滤
   - 禁止在节点面板编辑 endpoint/table/redis 配置

验收：

```text
state-store 可通过 resourceRef 写 default_workflow_state
节点配置里不需要填写 storageType/providerEndpoint
不可用资源在节点下拉中禁用并显示原因
资源调用产生 audit log
```

### P1：权限和 UI 完整化

目标：资源能限制到工作区、文件夹、流程和节点。

任务：

1. 实现资源 binding 校验：
   - workspace
   - folder
   - includeSubfolders
   - activity
   - nodeType
   - nodeId
   - operation
   - namespace
   - minRole
2. 接入现有 workspace role：
   - OWNER
   - ADMIN
   - EDITOR
   - VIEWER
   - GUEST
3. 资源管理 UI：
   - 资源列表
   - 新建资源向导
   - 资源详情
   - 权限绑定 tab
   - 调用日志 tab
4. 工作区首页增加资源入口。
5. 流程编辑器节点面板增加资源可用性说明。
6. 发布检查增加资源校验：
   - resourceRef 是否存在
   - resource 是否 ACTIVE
   - resource 是否允许该 activity 使用
   - operation 是否被允许

验收：

```text
资源可限制为仅某个工作区可用
资源可限制为仅某个文件夹及子文件夹可用
资源可限制为仅某个流程可用
VIEWER 不能保存写资源节点
发布前能发现无权限资源引用
```

### P2：公共表和 Provider Adapter

目标：支持公共表和外部 provider 的生产使用。

当前实现：

1. `state-store + resourceRef` 已支持 `DATABASE_TABLE`：
   - 资源配置承载 `physicalTable/table/tableNamespace` 和 `primaryKey`
   - 执行器按 `primaryKey` 生成行 key，并自动补齐 `tenantId/activityId/subjectId/eventId`
   - 支持 `READ/WRITE/UPSERT/APPEND/DELETE`，单行写入支持 CAS 版本检查
   - 底层先使用受控的 `marketing_storage_state` 作为虚拟公共表承载，不直接开放任意 SQL 表写入
2. `state-store + resourceRef` 已支持 `HTTP_PROVIDER`：
   - `endpoint/method/headers/timeoutSeconds` 从资源配置读取
   - 节点只需要引用 `resourceRef`、操作、namespace、keyTemplate、valueType
   - 资源配置优先于 schema 默认值，节点显式配置仍可覆盖
3. 资源测试连接已增加 adapter 配置校验：
   - `DATABASE_TABLE` 必须配置表名和 `primaryKey`
   - `HTTP_PROVIDER` 必须配置 endpoint，method 只允许 `GET/POST/PUT`
4. 默认能力已补齐：
   - `DATABASE_TABLE` 默认支持 `READ/WRITE/UPSERT/APPEND/DELETE/CAS`
   - `HTTP_PROVIDER` 默认支持 `READ/WRITE/UPSERT/DELETE/CALL/TEST_CONNECTION`

任务：

1. `DATABASE_TABLE` adapter：
   - schema 校验
   - keyMapping
   - valueMapping
   - READ/UPSERT/DELETE/CAS
2. `HTTP_PROVIDER` adapter：
   - endpoint 从资源配置读取
   - headers 从 secretRef 读取
   - request/response schema 校验
   - allowlist/SSRF 防护
3. `fact-query` 改造：
   - 支持 `resourceRef`
   - 查询配置从资源 schema 推导
4. `outbox/callback` 改造：
   - 支持 `resourceRef`
   - endpoint/topic 从资源读取
5. 测试连接增强：
   - schema 测试
   - 权限测试
   - dry-run 调用

验收：

```text
公共表资源可被 state-store/fact-query 使用
HTTP Provider endpoint 不再出现在节点配置里
节点 debug 能看到资源调用 trace
资源调用失败有统一错误码
```

### P3：Redis、MQ、Lua Resource API

目标：让高级资源类型进入生产执行。

任务：

1. `REDIS_KV` adapter
2. `REDIS_HASH` adapter
3. `REDIS_COUNTER` adapter
4. `REDIS_LOCK` adapter
5. `MQ_TOPIC` adapter
6. Lua 受控资源 API：
   - `resource_read`
   - `resource_write`
   - `resource_upsert`
   - `resource_call`
7. 对高风险操作增加审批策略：
   - DELETE
   - CALL 钱包
   - PUBLISH 外部消息
   - 生产 debug 写入

验收：

```text
quota-check 可使用 REDIS_COUNTER
lock 节点可使用 REDIS_LOCK
outbox 可使用 MQ_TOPIC
Lua 只能访问被 binding 授权的资源
```

### P4：治理和运维

目标：资源管理可维护、可审计、可迁移。

任务：

1. 资源版本管理：
   - schema version
   - config version
   - 引用影响分析
2. 资源引用关系图：
   - 哪些流程引用资源
   - 哪些节点引用资源
3. 资源健康监控：
   - 调用量
   - 错误率
   - P95 延迟
   - 最近失败
4. 资源迁移：
   - dev -> staging -> prod
   - 导入/导出资源定义
   - secretRef 环境映射
5. 资源停用保护：
   - 被发布流程引用时禁止直接删除
   - 只能 DISABLED 或 DEPRECATED

验收：

```text
删除资源前能看到影响流程
资源 schema 变更有版本和审计
生产资源错误率可在管理台看到
资源可跨环境迁移但不导出密钥
```

## 推荐最终节点配置

`state-store`：

```json
{
  "resourceRef": "default_workflow_state",
  "operation": "UPSERT",
  "namespace": "checkin",
  "keyTemplate": "${tenantId}:${activityId}:${subject.id}",
  "valueType": "OBJECT",
  "valuePath": "$.input.value",
  "missingPolicy": "EMPTY",
  "requireVersion": true
}
```

`fact-query`：

```json
{
  "resourceRef": "user_profile_provider",
  "operation": "READ",
  "queryMapping": {
    "tenantId": "${tenantId}",
    "userId": "${subject.id}"
  },
  "outputPath": "facts.userProfile",
  "missingPolicy": "BLOCK"
}
```

`outbox`：

```json
{
  "resourceRef": "notification_topic",
  "operation": "PUBLISH",
  "payloadMapping": {
    "userId": "${subject.id}",
    "template": "CHECKIN_SUCCESS",
    "data": "$.outputs.grant_order"
  }
}
```

## 结论

公共资源管理是 workflow 生产化的基础能力。资源必须从节点配置中抽离，形成独立的资源注册、权限、schema、密钥、adapter 和审计体系。

最终目标：

```text
节点只写 resourceRef + operation + mapping。
资源管理负责 storageType、driver、endpoint、table、schema、secret、权限和审计。
```
