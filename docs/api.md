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
rules             活动规则
rewards           奖励配置
```

创建阶段会进行基础 Schema 校验。当前普通活动只允许 `CONDITION` 规则、`FIXED` / `RATIO` / `LADDER` / `RANDOM` / `PROBABILITY` / `POOL_SPLIT` 奖励计算、`AUTO` 发奖模式，以及 `HTTP` / `WEBHOOK` 发奖通道；`rewardConfig.targetRef` 对可执行发奖必填且必须是 `http` 或 `https` URL。`MQ`、`MANUAL_EXPORT`、`MANUAL`、未知配置字段、非法版本或不可解析 Decimal 会返回 `400 Bad Request`。

奖励计算使用 Decimal 语义并输出 4 位小数。`RATIO` 支持 `maxAmount` 封顶，`LADDER` 使用闭区间且拒绝重叠阶梯，`RANDOM` / `PROBABILITY` 使用 tenant、activity、rewardCode、幂等键、配置版本和 `seedSalt` 形成稳定 seed，`POOL_SPLIT` 只支持事件即时 `COUNT` / `WEIGHT` 分摊。计算结果为 0 时不会生成发奖单、outbox 或 quota 扣减。

规则执行顺序固定为：

```text
MATCH -> QUALIFY -> REWARD -> RISK_LIMIT_BUDGET
```

`RISK_LIMIT_BUDGET` 是业务阶段名称，实际规则阶段仍使用 `RISK` 或 `LIMIT`；`BUDGET` 是 quota 类型，不是合法 `rulePhase`。`CONDITION` 字段路径支持 `attributes.*`、`state.*` 和 `facts.*`，字段缺失、类型不匹配、非法操作符或未注册 evaluator 不会默认通过。

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
命中的 participation 预览
预计 grant_order
预计 outbox reward command
拒绝原因
ruleResults 规则阶段与规则级解释
```

## 上报标准事件

```http
POST /openapi/v1/events
Idempotency-Key: site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_123
```

`Idempotency-Key` 为必填 Header，必须等于：

```text
tenantId + ":" + sourceSystem + ":" + eventType + ":" + biz.bizType + ":" + biz.bizId
```

事件模型只允许以下字段：`eventId`、`eventType`、`sourceSystem`、`tenantId`、`subject`、`biz`、`occurredAt`、`attributes`。业务扩展字段必须放在 `attributes` 内；顶层、`subject`、`biz` 中的未知字段会返回 `INVALID_EVENT`。

示例：

```bash
curl -X POST http://localhost:8080/openapi/v1/events \
  -H 'content-type: application/json' \
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
409  QUOTA_EXCEEDED: <quotaType>: <scope>: <scopeKey>
500  非预期内部错误
```

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
    "provider": {
      "factCode": "USER_PROFILE",
      "provider": "HTTP",
      "endpoint": "https://member.example.com/openapi/user/profile",
      "method": "POST",
      "headers": {
        "Authorization": "Bearer example"
      },
      "cacheTtlSeconds": 300
    },
    "event": {
      "eventId": "evt_1",
      "eventType": "DEPOSIT_SUCCESS",
      "sourceSystem": "payment",
      "tenantId": "site_001",
      "subject": {"type": "USER", "id": "10001"},
      "biz": {"bizType": "DEPOSIT", "bizId": "pay_123"},
      "occurredAt": "2026-05-24T12:00:00Z",
      "attributes": {}
    }
  }'
```

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
GET /admin/v1/audit?limit=100
```

当前记录：

```text
CREATE_ACTIVITY
CREATE_QUOTA
EVENT_IN
QUOTA_EXCEEDED
```

`QUOTA_EXCEEDED` payload 包含 `activityId`、`eventInboxId`、`subject`、`biz`、`quotaType`、`scope`、`scopeKey`、`amount`、`reason` 和 `error`，用于排查预算、库存或次数不足导致的发奖回滚。
