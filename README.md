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

The worker sends `HTTP` and `WEBHOOK` reward commands to `rewardConfig.targetRef`, retries failures, then marks exhausted messages as `DLQ`.

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
POST /admin/v1/simulate
POST /openapi/v1/events
POST /openapi/v1/facts/query
POST /openapi/v1/reward/callback
```

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
          "flowMultiple": "3"
        },
        "grantMode": "AUTO",
        "grantChannel": "HTTP"
      }
    ]
  }'
```

Submit event example:

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

## Layout

```text
cmd/exmarketing       service entrypoint
internal/httpapi      HTTP handlers and routing
internal/engine       event processing pipeline
internal/marketing    domain model and repository contracts
internal/rule         condition rule evaluator
internal/reward       fixed and ratio reward calculator
internal/decimal      decimal money helper, no float64 for amount math
internal/worker       outbox HTTP/Webhook sender with retry and DLQ
internal/fact         HTTP fact provider client
internal/plugin       activity plugin registry contract
db/migration          first-phase schema
docs                  design notes
```
