# ExMarketing

`ExMarketing` is a standalone marketing system under `ex-stack`.

It is designed to hold reusable marketing capabilities:

- activity lifecycle
- rule evaluation
- reward definition
- event intake
- grant order generation
- idempotency, quota, budget, inventory, audit, outbox and DLQ

## Run

Start local PostgreSQL:

```bash
docker compose up -d postgres
```

Use the Docker PostgreSQL database:

```bash
export EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable'
go run ./cmd/exmarketing
```

Default address:

```text
:8080
```

Without `EX_MARKETING_DATABASE_URL`, the service uses the in-memory repository.

Enable the Outbox sender:

```bash
export EX_MARKETING_OUTBOX_WORKER=true
```

The worker sends `HTTP`, `WEBHOOK`, and configured `MQ` reward commands to `rewardConfig.targetRef`, retries retryable failures with exponential backoff, then marks exhausted or non-retryable messages as `DLQ`. MQ delivery uses an adapter boundary for Kafka/RabbitMQ/Pulsar; broker credentials stay in runtime configuration and never in activity config or outbox payload.

DLQ messages can be inspected and operated from the admin API:

```bash
GET  /admin/v1/outbox/dlq?tenantId=site_001&includeClosed=true
POST /admin/v1/outbox/{id}/replay
POST /admin/v1/outbox/{id}/close
```

Stop local PostgreSQL:

```bash
docker compose down
```

## APIs

```text
GET  /healthz
GET  /admin/
GET  /admin/v1/activities
POST /admin/v1/activities
POST /admin/v1/quotas
GET  /admin/v1/audit
GET  /admin/v1/reward/reconciliation
POST /admin/v1/simulate
GET  /admin/v1/workflow/requirements
GET  /admin/v1/workflow/requirements/{id}
POST /openapi/v1/events
POST /openapi/v1/facts/query
POST /openapi/v1/reward/callback
```

Reward callbacks accept signed JSON from external grant systems. A callback is scoped by `tenantId` and can locate a grant order by `billNo` / `commandId` or `idempotencyKey`; conflicting locators return `CALLBACK_LOCATOR_CONFLICT`, and configured tenants must pass `X-ExMarketing-Timestamp` plus HMAC-SHA256 `X-ExMarketing-Signature` verification before any grant, participation, or quota state changes.

Create activity example:

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
        }
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
        "enabled": true
      }
    ]
  }'
```

Submit event example:

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

The event endpoint accepts only the standard event fields:

```text
eventId, eventType, sourceSystem, tenantId, subject, biz, occurredAt, attributes
```

`Idempotency-Key` is required and must equal `tenantId:sourceSystem:eventType:bizType:bizId`. Reposting the same payload with the same key returns `200 OK` with `duplicate=true` and does not create another participation, grant order, or outbox message. Reusing the same key for a different payload returns `400 INVALID_IDEMPOTENCY_KEY`.

For production, require OpenAPI event auth:

```bash
export EX_MARKETING_REQUIRE_OPENAPI_AUTH=true
export EX_MARKETING_OPENAPI_TOKENS='dev-token:site_001:payment'
```

`EX_MARKETING_OPENAPI_TOKENS` uses `token:tenantId[,tenantId...]:sourceSystem[,sourceSystem...]` entries separated by `;`. `EX_MARKETING_EVENT_MAX_BODY_BYTES` optionally overrides the default 1 MiB event request body limit.

Webhook receivers can accept provider-specific JSON and map it into the same standard event chain:

```bash
export EX_MARKETING_WEBHOOK_RECEIVERS_JSON='[{"receiverCode":"payment-webhook","enabled":true,"secret":"receiver-secret","allowedTenantIds":["site_001"],"allowedSourceSystems":["payment"],"allowedIPs":[],"mapping":{"eventId":{"path":"$.id"},"eventType":{"const":"DEPOSIT_SUCCESS"},"sourceSystem":{"const":"payment"},"tenantId":{"const":"site_001"},"subject.type":{"const":"USER"},"subject.id":{"path":"$.user.id"},"biz.bizType":{"const":"DEPOSIT"},"biz.bizId":{"path":"$.payment.id"},"occurredAt":{"path":"$.createdAt"},"attributes":{"amount":{"path":"$.payment.amount"},"currency":{"path":"$.payment.currency"},"providerEventId":{"path":"$.id"}}}}]'
```

`POST /openapi/v1/webhooks/{receiverCode}` requires JSON plus `X-ExMarketing-Timestamp` and `X-ExMarketing-Signature`. The signature is lowercase hex `HMAC-SHA256(receiver.secret, "<timestamp>.<rawBody>")`. `Idempotency-Key` is optional for Webhook requests; when present it must equal the mapped standard event key.

## Layout

```text
cmd/exmarketing       service entrypoint
internal/httpapi      HTTP handlers and routing
internal/engine       event processing pipeline
internal/marketing    domain model and repository contracts
internal/rule         condition rule evaluator
internal/reward       reward calculator and reward schema validation helpers
internal/decimal      decimal money helper, no float64 for amount math
internal/worker       outbox HTTP/Webhook/MQ sender with retry and DLQ
internal/fact         HTTP fact provider client
internal/plugin       activity plugin registry contract
db/migration          first-phase schema
docs                  design notes
```

ActivityPlugin is reserved for new gameplay context that cannot be expressed
with normal activity config, rules, rewards, and quotas. Plugin context is
exposed to rules and rewards only through `state.plugin.<pluginCode>.*`; normal
first-deposit, re-deposit, and discount activities should stay data-configured.
Plugins do not directly grant rewards, reserve quota, write outbox commands, or
persist real gameplay state in this phase.

## Agent Workflow

```bash
go run ./cmd/workflowctl add --title "example" --description "需求描述"
go run ./cmd/workflowctl intake
go run ./cmd/workflowctl list
EX_MARKETING_AGENT_CMD='/Applications/Codex.app/Contents/Resources/codex exec --cd . --sandbox danger-full-access --ask-for-approval never -' \
  go run ./cmd/workflowctl run-agent --id <requirement-id>
go run ./cmd/workflowctl agent-status --id <requirement-id>
```

本地服务启动后可以打开工作流看板：

```text
http://localhost:8080/admin/
```

## Documents

```text
docs/architecture.md  架构说明
docs/api.md           API 接入示例
docs/data-model.md    数据模型和状态枚举
docs/operations.md    本地运维、排障和生产前检查
docs/roadmap.md       分阶段路线图
docs/workflow.md      任务流水线 Agent 协作流程
docs/automation.md    需求列表驱动的自动化流程
docs/langgraph-studio.md LangGraph + LangSmith Studio 集成
CLAUDE.md             开发流程和硬性规则
tasks/                任务级分析、审核、开发、测试、提交产物
```
