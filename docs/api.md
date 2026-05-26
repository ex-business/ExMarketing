# API 文档

## 基础约定

服务默认监听：

```text
:8080
```

本地 PostgreSQL 运行方式：

```bash
export EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable'
go run ./cmd/exmarketing
```

所有 JSON API 默认：

```http
Content-Type: application/json
```

外部事件幂等键固定为：

```text
tenantId + ":" + sourceSystem + ":" + eventType + ":" + bizType + ":" + bizId
```

示例：

```text
site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_123
```

## 健康检查

```http
GET /healthz
```

响应：

```json
{
  "status": "ok"
}
```

## 管理后台入口

```http
GET /admin/
```

返回本地后台页面，用于 Agent 工作流看板、活动列表、活动创建和模拟运行。

当前页面只面向本地开发，不应直接暴露到公网生产环境。

## Agent 工作流看板数据

```http
GET /admin/v1/workflow/requirements
```

返回需求列表、流程状态和五个 Agent 的当前状态。

```http
POST /admin/v1/workflow/requirements
```

新增需求：

```json
{
  "title": "预算原子扣减",
  "description": "活动发奖前需要原子扣减预算、库存和用户次数",
  "risk": "高"
}
```

```http
POST /admin/v1/workflow/intake
```

接收所有 `NEW` 需求，生成 `tasks/<id>/` 任务目录和 Agent 产物文件。

```http
GET /admin/v1/workflow/requirements/{id}
```

返回单个需求详情，包括：

```text
requirement              需求元数据
agents                   Analysis / Review / Dev / Test / Commit 状态
artifacts                task.md、analysis.md、review.md、implementation.md、test.md、commit.md 等文件内容
```

```http
POST /admin/v1/workflow/requirements/{id}/run-agent
```

运行指定 Agent。`agent=auto` 时根据当前状态自动选择下一步。

```json
{
  "agent": "auto"
}
```

```http
POST /admin/v1/workflow/requirements/{id}/run-pipeline
```

自动按当前状态连续运行 Agent，最多 6 步：

```text
Analysis -> Review -> Test Plan -> Dev -> Test -> Commit
```

遇到 `REVIEW_BLOCK`、`TEST_FAIL`、`COMMITTED` 或不可运行状态会停止，并生成：

```text
tasks/<id>/agent-report.md
```

请求：

```json
{
  "maxSteps": 6
}
```

```http
GET /admin/v1/workflow/requirements/{id}/logs/{agent}
```

读取指定 Agent 最近一次执行日志。

```http
GET /admin/v1/git/diff
```

查看当前 `git diff --stat` 和 `git diff`。

```http
POST /admin/v1/git/commit
```

本地提交。要求：

```text
requirement.status = COMMIT_READY
go test ./... PASS
go build ./... PASS
```

请求：

```json
{
  "requirementId": "2026-05-24-example",
  "message": "Add workflow admin controls"
}
```

## 创建活动

```http
POST /admin/v1/activities
```

示例：

```bash
curl -X POST http://localhost:8080/admin/v1/activities \
  -H 'content-type: application/json' \
  -d '{
    "tenantId": "site_001",
    "activityCode": "FIRST_DEPOSIT_BONUS",
    "activityName": "First Deposit Bonus",
    "activityType": "FIRST_DEPOSIT",
    "status": "ACTIVE",
    "triggerEvents": ["DEPOSIT_SUCCESS"],
    "startTime": "2026-05-01T00:00:00Z",
    "endTime": "2026-06-01T00:00:00Z",
    "timezone": "UTC",
    "priority": 100,
    "exclusiveGroup": "deposit_bonus",
    "stackable": false,
    "rules": [
      {
        "ruleCode": "FIRST_DEPOSIT_AND_AMOUNT",
        "rulePhase": "QUALIFY",
        "ruleType": "CONDITION",
        "ruleConfig": {
          "logic": "AND",
          "conditions": [
            {"field": "attributes.isFirstDeposit", "op": "=", "value": true},
            {"field": "attributes.amount", "op": ">=", "value": "100.00"}
          ]
        },
        "sort": 1,
        "enabled": true
      }
    ],
    "rewards": [
      {
        "rewardCode": "FIRST_DEPOSIT_RATIO",
        "rewardType": "BONUS",
        "calculateType": "RATIO",
        "rewardConfig": {
          "baseField": "attributes.amount",
          "rate": "0.20",
          "maxAmount": "500.00",
          "currency": "CNY",
          "flowMultiple": "3",
          "targetRef": "https://wallet.example.com/reward"
        },
        "grantMode": "AUTO",
        "grantChannel": "HTTP",
        "enabled": true,
        "version": 1
      }
    ]
  }'
```

关键字段：

```text
tenantId          租户 ID
activityCode      活动编码
activityType      活动类型
status            DRAFT / SCHEDULED / ACTIVE / PAUSED / ENDED
triggerEvents     触发事件类型列表
timezone          活动时区，必须显式填写
exclusiveGroup    互斥组，同组默认只命中一个
stackable         是否允许叠加命中
pluginCode        可选 ActivityPlugin 编码；普通满减、首存、续存不需要配置
config/configJson 活动主配置扩展 JSON object
rules             活动规则
rewards           奖励配置
```

`config` / `configJson` / `config_json` 仅允许通用活动扩展字段，不承载具体业务页面模型。当前允许的一层字段为 `entry`、`display`、`audience`、`metadata`；未知一层字段返回 `400 Bad Request`。

展示与入口配置示例：

```json
{
  "display": {
    "visible": true,
    "title": "首存赠送",
    "subtitle": "首笔充值享奖励",
    "description": "活动说明",
    "bannerUrl": "https://example.com/banner.png",
    "sort": 100,
    "tags": ["new_user", "deposit"]
  },
  "entry": {
    "enabled": true,
    "entryCodes": ["HOME_BANNER", "PROMOTION_CENTER"],
    "channels": ["APP", "WEB"],
    "sourceSystems": ["payment"],
    "tenantIds": ["site_001"],
    "audience": {
      "subjectTypes": ["USER"],
      "segments": ["new_user"],
      "riskLevels": ["NORMAL"]
    }
  }
}
```

字段语义：

- `display.visible=false` 只表示不对展示入口可见，不会让内部事件处理或模拟运行失效。
- `entry.enabled=false` 会在 MATCH 阶段拒绝该活动候选，并返回 `ENTRY_DISABLED`。
- `entry.channels` 读取标准事件 `attributes.channel`；非空且不命中时返回 `ENTRY_CHANNEL_NOT_ALLOWED`。
- `entry.sourceSystems` 读取标准事件 `sourceSystem`；非空且不命中时返回 `ENTRY_SOURCE_SYSTEM_NOT_ALLOWED`。
- `entry.tenantIds` 只能为空或包含活动自身 `tenantId`，不能扩大租户边界；运行时非空且不命中返回 `ENTRY_TENANT_NOT_ALLOWED`。
- `entry.audience.subjectTypes` 读取 `subject.type`，`segments` 读取 `attributes.segments` 或 `attributes.segment`，`riskLevels` 读取 `attributes.riskLevel`。
- 旧配置字段 `entry.entranceCodes`、`entry.landingUrl`、顶层 `audience.segmentCodes/tags/countries` 仍可保存和读回；新入口匹配只依赖通用 `entry.*` 与标准事件字段。

创建阶段会进行基础 Schema 校验。`tenantId + activityCode` 在同一租户内唯一，重复创建返回 `409 Conflict`；不同租户可以复用同一 `activityCode`。`startTime` / `endTime` 必须是带时区偏移的 RFC3339 时间，且 `startTime < endTime`；`timezone` 必须是可识别的 IANA 时区名或 `UTC`。当前普通活动只允许已注册 evaluator 的 `CONDITION` 规则；`FORMULA`、`LIMIT`、`SCRIPT` 只保留配置 Schema，未注册 evaluator 前即使结构合法也会返回 `400 Bad Request`。奖励计算允许 `FIXED` / `RATIO` / `LADDER` / `RANDOM` / `PROBABILITY` / `POOL_SPLIT`，发奖模式允许 `AUTO`，发奖通道允许 `HTTP` / `WEBHOOK`；`rewardConfig.targetRef` 对可执行发奖必填且必须是 `http` 或 `https` URL。`MQ`、`MANUAL_EXPORT`、`MANUAL`、未知配置字段、非法版本或不可解析 Decimal 会返回 `400 Bad Request`。配置 `pluginCode` 时必须命中服务内注册的 ActivityPlugin，且与 `activityType` 兼容；未知插件会在创建或上线前校验阶段拒绝。

奖励计算使用 Decimal 语义并输出 4 位小数。`RATIO` 支持 `maxAmount` 封顶，`LADDER` 使用闭区间且拒绝重叠阶梯，`RANDOM` / `PROBABILITY` 使用 tenant、activity、rewardCode、幂等键、配置版本和 `seedSalt` 形成稳定 seed，`POOL_SPLIT` 只支持事件即时 `COUNT` / `WEIGHT` 分摊。计算结果为 0 时不会生成发奖单、outbox 或 quota 扣减。

规则执行顺序固定为：

```text
MATCH -> QUALIFY -> REWARD -> RISK_LIMIT_BUDGET
```

`RISK_LIMIT_BUDGET` 是业务阶段名称，实际规则阶段仍使用 `RISK` 或 `LIMIT`；`BUDGET` 是 quota 类型，不是合法 `rulePhase`。`CONDITION.logic` 必填且只能是 `AND` / `OR`，`conditions` 必须非空，`op` 只能是 `=`、`==`、`!=`、`<>`、`>`、`>=`、`<`、`<=`、`IN`、`NOT_IN`；比较操作符要求可解析 Decimal，`IN` / `NOT_IN` 要求非空数组。字段路径支持标准事件字段 `eventId`、`eventType`、`sourceSystem`、`tenantId`、`subject.type`、`subject.id`、`biz.bizType`、`biz.bizId`、`occurredAt`，以及 `attributes.*`、`facts.<factCode>.*`、`state.activity.*`、`state.subject.*`、`state.quota.*`、`state.exclusive.*`、绑定插件时唯一允许的 `state.plugin.<pluginCode>.*`。非标准顶层业务字段如 `orderAmount`、`deposit.amount`、`user.vipLevel` 会被拒绝；`enabled=false` 只影响运行时跳过，不放宽保存校验。插件上下文不能使用 `plugin.*`、`state.<activityType>.*` 或短路径别名；字段缺失、类型不匹配、非法操作符或未注册 evaluator 不会默认通过。

## 查询活动

```http
GET /admin/v1/activities
```

响应：

```json
{
  "items": []
}
```

详情查询：

```http
GET /admin/v1/activities/{id}
```

不存在的活动返回 `404 Not Found`。

## 更新活动主配置

```http
PUT /admin/v1/activities/{id}
```

允许更新 `activityName`、`status`、`startTime`、`endTime`、`timezone`、`priority`、`exclusiveGroup`、`stackable`、`config` / `configJson` / `config_json`。每次成功更新会递增 `version` 并刷新 `updatedAt`。`id`、`tenantId`、`activityCode`、`activityType` 是身份字段，出现在更新请求中会返回 `400 Bad Request`；不存在的活动返回 `404 Not Found`。

## 创建 Quota

```http
POST /admin/v1/quotas
```

活动总预算：

```bash
curl -X POST http://localhost:8080/admin/v1/quotas \
  -H 'content-type: application/json' \
  -d '{
    "activityId": "act_xxx",
    "quotaType": "BUDGET",
    "scope": "ACTIVITY",
    "scopeKey": "TOTAL",
    "totalValue": "10000.00"
  }'
```

用户参与次数限制：

```bash
curl -X POST http://localhost:8080/admin/v1/quotas \
  -H 'content-type: application/json' \
  -d '{
    "activityId": "act_xxx",
    "quotaType": "USER_LIMIT",
    "scope": "USER",
    "scopeKey": "10001",
    "totalValue": "1"
  }'
```

奖励库存限制：

```bash
curl -X POST http://localhost:8080/admin/v1/quotas \
  -H 'content-type: application/json' \
  -d '{
    "activityId": "act_xxx",
    "quotaType": "STOCK",
    "scope": "ACTIVITY",
    "scopeKey": "FIRST_DEPOSIT_RATIO",
    "totalValue": "100"
  }'
```

当前支持的 quota：

```text
BUDGET       活动预算
STOCK        奖品库存，scopeKey 固定为 rewardCode，每个 enabled reward 默认扣 1
USER_LIMIT   用户次数限制
DAILY_LIMIT  每日次数限制，模型已定义，业务流程待扩展
```

当前支持的 scope：

```text
ACTIVITY
USER
IP
DEVICE
```

## 模拟运行

```http
POST /admin/v1/simulate
```

模拟运行不会写入：

```text
marketing_event_inbox
marketing_participation
marketing_grant_order
marketing_outbox
marketing_quota
```

示例：

```bash
curl -X POST http://localhost:8080/admin/v1/simulate \
  -H 'content-type: application/json' \
  -d '{
    "eventId": "evt_sim_001",
    "eventType": "DEPOSIT_SUCCESS",
    "sourceSystem": "payment",
    "tenantId": "site_001",
    "subject": {"type": "USER", "id": "10001"},
    "biz": {"bizType": "DEPOSIT", "bizId": "pay_sim_001"},
    "occurredAt": "2026-05-24T12:00:00Z",
    "attributes": {
      "amount": "299.00",
      "currency": "CNY",
      "isFirstDeposit": true
    }
  }'
```

输出包含：

```text
simulatedAt
matchedActivities / unmatchedActivities
summary.candidateCount / matchedCount / unmatchedCount
summary.hasRewardableActivity / hasQuotaRisk
每个活动的 decision / decisionReasonCode / decisionReason
ruleResult 规则阶段与规则级解释
rewardPreviews 奖励计算预览和计算明细
quotaImpactPreviews 预算、库存、用户次数影响预览
participationPreview
grantOrderPreviews
outboxCommandPreviews
plugin.pluginCode / plugin.pluginContext / plugin.precheckResult / plugin.stateChangeIntents
```

模拟结果只表示按当前配置和当前 quota 快照的预览，不代表 quota 已锁定、奖励已发放、插件状态已消费或发奖指令已投递。若 quota 预览 `wouldExceed=true`，该活动会返回 `QUOTA_EXCEEDED` 风险原因，并且不会展示可执行的 grant order / outbox command 预览。缺少对应 quota 行时按无限制展示为 `unlimited=true`。

## Outbox DLQ

```http
GET /admin/v1/outbox/dlq?tenantId=site_001&includeClosed=true
```

返回当前 tenant 的 DLQ outbox。默认只返回 `DLQ`，`includeClosed=true` 时同时返回 `DLQ_CLOSED`。

```http
POST /admin/v1/outbox/{id}/replay
POST /admin/v1/outbox/{id}/close
```

请求体：

```json
{
  "tenantId": "site_001",
  "operator": "ops",
  "reason": "target recovered"
}
```

规则：

- `replay` 只允许当前状态为 `DLQ`，成功后同一 outbox 回到 `PENDING`，payload 不变。
- `close` 只允许当前状态为 `DLQ`，成功后同一 outbox 进入 `DLQ_CLOSED`。
- `DLQ_CLOSED` 不允许 replay。
- `operator` 和 `reason` 必填。
- 操作会写入 `marketing_audit`。

## 发奖回调与对账

```http
POST /openapi/v1/reward/callback
```

请求：

```json
{
  "tenantId": "site_001",
  "commandId": "cmd_xxx",
  "status": "SUCCESS",
  "externalGrantId": "wallet_001",
  "message": "paid",
  "finishedAt": "2026-05-24T12:05:00Z"
}
```

规则：

- `tenantId + commandId` 唯一定位 `marketing_grant_order`，不会跨 tenant 更新。
- `status` 只接受 `SUCCESS` / `FAILED`；`SUCCESS` 必须提供 `externalGrantId`。
- 首次 `SUCCESS` 会把该 grant_order 关联的 quota reservation 从 `FROZEN` 置为 `CONFIRMED`，并将 quota 聚合从 `frozen_value` 转入 `used_value`。
- 首次 `FAILED` 会把该 grant_order 关联的 quota reservation 从 `FROZEN` 置为 `RELEASED`，并释放 quota 聚合 `frozen_value`。
- 相同最终态的重复回调返回 `duplicate=true`，不会重复确认或释放。
- 相反最终态、`externalGrantId` 冲突、缺少 reservation 或 quota 聚合不一致会返回机器可识别错误。

错误码：

```text
400  INVALID_CALLBACK_REQUEST
404  GRANT_ORDER_NOT_FOUND
409  CALLBACK_STATUS_CONFLICT
409  GRANT_ORDER_CLOSED
409  QUOTA_RESERVATION_MISSING
409  QUOTA_RESERVATION_CONFLICT
409  QUOTA_AGGREGATE_CONFLICT
```

对账查询：

```http
GET /admin/v1/reward/reconciliation?tenantId=site_001&commandId=cmd_xxx
GET /admin/v1/reward/reconciliation?tenantId=site_001&activityId=act_xxx
GET /admin/v1/reward/reconciliation?tenantId=site_001&subjectType=USER&subjectId=10001
GET /admin/v1/reward/reconciliation?tenantId=site_001&eventId=evt_xxx
GET /admin/v1/reward/reconciliation?tenantId=site_001&idempotencyKey=site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_xxx
```

支持参数：

- `tenantId` 必填，所有查询按租户隔离。
- `commandId` / `grantOrderId`：按发奖单查询。
- `activityId`：按活动查询。
- `subjectType + subjectId`：按用户或主体查询。
- `eventId` / `idempotencyKey`：按事件或标准幂等键查询。
- `limit`：分页上限，默认 100。

返回 grant_order、participation 状态、event inbox、outbox、回调信息、quota reservations 以及相关审计时间线，供排查长期冻结悬挂、回调异常、活动/用户/事件维度链路。

## Webhook Receiver

```http
POST /openapi/v1/webhooks/{receiverCode}
Content-Type: application/json
X-ExMarketing-Timestamp: <unix_seconds>
X-ExMarketing-Signature: <lowercase_hex_hmac_sha256>
Idempotency-Key: <optional>
```

Webhook receivers are configured at startup through `EX_MARKETING_WEBHOOK_RECEIVERS_JSON`. The receiver validates `receiverCode`, enabled status, optional source IP allowlist, timestamp replay window, and HMAC signature before parsing JSON or mapping fields.

Signature input:

```text
<X-ExMarketing-Timestamp> + "." + <raw request body>
```

Signature algorithm:

```text
hex(HMAC-SHA256(receiver.secret, signingInput))
```

Example receiver config:

```bash
export EX_MARKETING_WEBHOOK_RECEIVERS_JSON='[{"receiverCode":"payment-webhook","enabled":true,"secret":"receiver-secret","allowedTenantIds":["site_001"],"allowedSourceSystems":["payment"],"allowedIPs":[],"mapping":{"eventId":{"path":"$.id"},"eventType":{"const":"DEPOSIT_SUCCESS"},"sourceSystem":{"const":"payment"},"tenantId":{"const":"site_001"},"subject.type":{"const":"USER"},"subject.id":{"path":"$.user.id"},"biz.bizType":{"const":"DEPOSIT"},"biz.bizId":{"path":"$.payment.id"},"occurredAt":{"path":"$.createdAt"},"attributes":{"amount":{"path":"$.payment.amount"},"currency":{"path":"$.payment.currency"},"providerEventId":{"path":"$.id"}}}}]'
```

Mapping supports only `const` or simple object field paths such as `$.payment.id`. It does not support scripts, filters, wildcard arrays, provider SDKs, external database reads, or implicit raw payload passthrough. The mapped event is processed by the same standard event chain as `POST /openapi/v1/events`; if `Idempotency-Key` is omitted, the internal key is derived from the mapped standard event.

Stable error responses use:

```json
{
  "errorCode": "WEBHOOK_SIGNATURE_INVALID",
  "message": "webhook signature invalid"
}
```

Webhook-specific error codes include `WEBHOOK_RECEIVER_NOT_FOUND`, `WEBHOOK_RECEIVER_DISABLED`, `WEBHOOK_SIGNATURE_MISSING`, `WEBHOOK_TIMESTAMP_INVALID`, `WEBHOOK_SIGNATURE_INVALID`, `WEBHOOK_SOURCE_FORBIDDEN`, `WEBHOOK_MAPPING_FAILED`, and `WEBHOOK_AUDIT_FAILED`. Standard event errors such as `INVALID_JSON`, `INVALID_EVENT`, `INVALID_IDEMPOTENCY_KEY`, `REQUEST_TOO_LARGE`, `UNSUPPORTED_MEDIA_TYPE`, and `QUOTA_EXCEEDED` are reused after the corresponding validation stage.

## 上报标准事件

```http
POST /openapi/v1/events
Authorization: Bearer <token>
Content-Type: application/json
Idempotency-Key: site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_123
```

Production deployments must set `EX_MARKETING_REQUIRE_OPENAPI_AUTH=true` and configure at least one valid `EX_MARKETING_OPENAPI_TOKENS` entry. Token entries use:

```text
token:tenantId[,tenantId...]:sourceSystem[,sourceSystem...];token2:tenantId2:sourceSystem2
```

If auth is required, missing Authorization, non-Bearer Authorization, empty Bearer token, or unknown token returns `401 UNAUTHORIZED`. A valid token that is not authorized for the event `tenantId` returns `403 TENANT_FORBIDDEN`; one that is not authorized for the event `sourceSystem` returns `403 SOURCE_FORBIDDEN`. The endpoint requires `Content-Type: application/json` and accepts media type parameters such as `application/json; charset=utf-8`. Event request bodies default to a 1 MiB limit and can be overridden with `EX_MARKETING_EVENT_MAX_BODY_BYTES`.

`Idempotency-Key` 为必填 Header，必须等于：

```text
tenantId + ":" + sourceSystem + ":" + eventType + ":" + biz.bizType + ":" + biz.bizId
```

事件模型只允许以下 8 个顶层字段：`eventId`、`eventType`、`sourceSystem`、`tenantId`、`subject`、`biz`、`occurredAt`、`attributes`。业务扩展字段必须放在 `attributes` 内；顶层、`subject`、`biz` 中的未知字段会返回 `INVALID_EVENT`。

字段约束：

| 字段 | 类型 | 必填 | 未知字段 | 说明 |
| --- | --- | --- | --- | --- |
| `eventId` | non-empty string | 是 | 不允许 | 外部事件 ID，用于追踪；不作为唯一幂等主键。 |
| `eventType` | non-empty string | 是 | 不允许 | 事件类型。Schema 不内置具体业务枚举，活动由 `triggerEvents` 匹配。 |
| `sourceSystem` | non-empty string | 是 | 不允许 | 来源系统。Schema 不内置来源系统白名单。 |
| `tenantId` | non-empty string | 是 | 不允许 | 租户隔离字段，必须参与 `Idempotency-Key`。 |
| `subject` | object | 是 | 只允许 `type`、`id` | 事件主体。 |
| `biz` | object | 是 | 只允许 `bizType`、`bizId` | 业务对象标识。 |
| `occurredAt` | RFC3339 string | 是 | 不允许 | 必须包含时区，例如 `Z` 或 `+08:00`。 |
| `attributes` | object | 是 | 允许任意内部业务字段 | 唯一业务扩展入口，可为空 `{}`，可包含嵌套 object/array。 |

`subject` 子字段：

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `subject.type` | non-empty string | 是 | 主体类型，例如 `USER`；Schema 不固定枚举。 |
| `subject.id` | non-empty string | 是 | 主体 ID。 |

`biz` 子字段：

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `biz.bizType` | non-empty string | 是 | 业务对象类型，例如 `DEPOSIT`、`ORDER`；Schema 不固定枚举。 |
| `biz.bizId` | non-empty string | 是 | 业务对象 ID。 |

时间示例：

```text
合法:   2026-05-24T12:00:00Z
合法:   2026-05-24T12:00:00+08:00
非法:   2026-05-24T12:00:00        # 缺少时区
非法:   2026-99-99                 # 日期非法
```

示例：

```bash
curl -X POST http://localhost:8080/openapi/v1/events \
  -H 'content-type: application/json' \
  -H 'Authorization: Bearer dev-token' \
  -H 'Idempotency-Key: site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_123' \
  -d '{
    "eventId": "evt_20260524_000001",
    "eventType": "DEPOSIT_SUCCESS",
    "sourceSystem": "payment",
    "tenantId": "site_001",
    "subject": {"type": "USER", "id": "10001"},
    "biz": {"bizType": "DEPOSIT", "bizId": "pay_123"},
    "occurredAt": "2026-05-24T12:00:00Z",
    "attributes": {
      "amount": "299.00",
      "currency": "CNY",
      "isFirstDeposit": true
    }
  }'
```

处理流程：

```text
1. 校验事件模型
2. 校验 Idempotency-Key
3. 写入 marketing_event_inbox
4. 匹配活动
5. 按 MATCH -> QUALIFY -> REWARD -> RISK_LIMIT_BUDGET 执行规则
6. 规则拒绝时写 REJECTED participation
7. 规则通过后计算全部奖励
8. 在同一 activity 级事务内写 REWARDABLE participation、扣 BUDGET/STOCK/USER_LIMIT、写 grant_order、写 outbox
9. quota 不足时回滚 participation/grant_order/outbox，写 QUOTA_EXCEEDED 审计
```

错误码：

```text
400  INVALID_EVENT: 事件格式错误、缺少必填字段或未知字段
400  INVALID_IDEMPOTENCY_KEY: 缺少 Idempotency-Key、Header 与 payload 不匹配，或同一 key 的 payload 不一致
400  INVALID_JSON: JSON 不可解析
401  UNAUTHORIZED: 缺少、格式错误、空或未知 Bearer token
403  TENANT_FORBIDDEN: token 无权上报该 tenantId
403  SOURCE_FORBIDDEN: token 无权上报该 sourceSystem
413  REQUEST_TOO_LARGE: 请求 body 超过 1 MiB 或 EX_MARKETING_EVENT_MAX_BODY_BYTES
415  UNSUPPORTED_MEDIA_TYPE: 缺少 Content-Type 或不是 application/json
409  QUOTA_EXCEEDED: <quotaType>: <scope>: <scopeKey>
500  非预期内部错误
```

OpenAPI Event API 失败响应包含稳定的 `errorCode` 和 `message` 字段，且不得包含完整 token、完整 Authorization header、内部配置或敏感 payload。

重复事件响应：

```json
{
  "duplicate": true,
  "participations": null,
  "grantOrders": null,
  "outboxMessages": null
}
```

完全重复的事件返回 `200 OK` 且 `duplicate=true`，不会再次创建 participation、grant_order 或 outbox。相同 `Idempotency-Key` 但 payload 不一致返回 `400 INVALID_IDEMPOTENCY_KEY`。

非法事件示例：

```json
{
  "code": "INVALID_EVENT",
  "error": "amount is unknown; business fields must be placed in attributes"
}
```

```json
{
  "code": "INVALID_EVENT",
  "error": "subject.vipLevel is unknown; business fields must be placed in attributes"
}
```

```json
{
  "code": "INVALID_EVENT",
  "error": "occurredAt must be RFC3339 time with timezone"
}
```

幂等键错误示例：

```json
{
  "code": "INVALID_IDEMPOTENCY_KEY",
  "error": "Idempotency-Key does not match event payload"
}
```

Fact Query 的 `event` payload 使用同一标准事件结构；能通过事件 `attributes` 携带的数据，不应通过 Fact Query 实时查询外部系统。

Quota 不足响应体：

```json
{
  "code": "QUOTA_EXCEEDED",
  "error": "quota exceeded: STOCK: ACTIVITY: FIRST_DEPOSIT_RATIO",
  "quotaType": "STOCK",
  "scope": "ACTIVITY",
  "scopeKey": "FIRST_DEPOSIT_RATIO"
}
```

## Fact Query

```http
POST /openapi/v1/facts/query
```

Fact Query 是可选能力。能通过事件 `attributes` 携带的数据，不要实时查询外部系统。

`/openapi/v1/facts/query` 只接受 registry key 和事件上下文。服务端按 `tenantId + factCode` 查找 enabled Fact Provider，禁止在请求中传 inline endpoint、auth、mapping 或 provider 配置。

必须配置：

```bash
export EX_MARKETING_FACT_ALLOWLIST='member.example.com'
```

安全约束：

```text
endpoint host 必须在 allowlist 内
拒绝 private/local IP
禁止 redirect
禁用 proxy
```

示例：

```bash
curl -X POST http://localhost:8080/openapi/v1/facts/query \
  -H 'content-type: application/json' \
  -d '{
    "tenantId": "site_001",
    "factCode": "USER_PROFILE",
    "subject": {"type": "USER", "id": "10001"},
    "biz": {"bizType": "DEPOSIT", "bizId": "pay_123"},
    "attributes": {},
    "traceId": "trace_1"
  }'
```

成功响应：

```json
{
  "tenantId": "site_001",
  "factCode": "USER_PROFILE",
  "success": true,
  "cached": false,
  "facts": {
    "USER_PROFILE": {
      "vipLevel": "VIP3"
    }
  },
  "providerStatus": "200 OK"
}
```

失败响应会返回稳定 `errorCode`，例如 `FACT_PROVIDER_NOT_CONFIGURED`、`FACT_PROVIDER_DISABLED`、`FACT_PROVIDER_HTTP_FAILED`、`FACT_PROVIDER_TIMEOUT`、`FACT_MAPPING_FAILED`。

## 发奖回调

```http
POST /openapi/v1/reward/callback
```

示例：

```bash
curl -X POST http://localhost:8080/openapi/v1/reward/callback \
  -H 'content-type: application/json' \
  -d '{
    "commandId": "cmd_xxx",
    "tenantId": "site_001",
    "status": "SUCCESS",
    "externalGrantId": "wallet_bill_888",
    "message": "granted",
    "finishedAt": "2026-05-24T12:00:03Z"
  }'
```

安全要求：

```text
更新 grant_order 时必须同时匹配 tenantId + commandId
后续生产版本必须增加签名或 token 鉴权
```

## 审计查询

```http
GET /admin/v1/audit?tenantId=site_001&limit=100
```

当前记录：

```text
CREATE_ACTIVITY
CREATE_QUOTA
EVENT_IN
SIMULATION_RUN
QUOTA_EXCEEDED
OUTBOX_DLQ_REPLAY
OUTBOX_DLQ_CLOSE
```

支持按 `tenantId`、`actor`、`action`、`resource`、`resourceId`、`activityId`、`eventId`、`commandId` 和 `limit` 过滤。创建和模拟类审计 payload 包含 `beforeSnapshot` / `afterSnapshot`；失败类审计在 payload 中记录错误上下文。`QUOTA_EXCEEDED` payload 包含 `activityId`、`eventInboxId`、`subject`、`biz`、`quotaType`、`scope`、`scopeKey`、`amount`、`reason` 和 `error`，用于排查预算、库存或次数不足导致的发奖回滚。
