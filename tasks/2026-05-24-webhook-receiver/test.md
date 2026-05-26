# Test

## Scope

已根据 `task.md`、`analysis.md`、`review.md`、`test-plan.md`、`implementation.md` 执行 Webhook Receiver 测试。

本轮只写入本文件，未修改业务代码、测试代码、文档代码、配置文件或其他文件。

测试环境：

```text
OS: macOS
Go: go1.25.4 darwin/arm64
PostgreSQL: docker compose service exmarketing-postgres, port 54329
EX_MARKETING_WEBHOOK_RECEIVERS_JSON: payment-webhook / receiver-secret / site_001 / payment / allowedIPs=[]
EX_MARKETING_DATABASE_URL: postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable
```

## Test Cases

- 正常 Webhook HTTP 路径：`POST /openapi/v1/webhooks/payment-webhook`，正确 timestamp + HMAC，返回 `202 Accepted` 和 `duplicate=false`。
- 重复 Webhook：同一映射幂等键再次发送，返回 `200 OK` 和 `duplicate=true`。
- Webhook/Event/Inbox/Idempot/Quota/Audit 定向 Go 测试：通过。
- race 定向测试：通过。
- integration tag 下 Webhook 定向测试：通过。
- PostgreSQL repository integration：通过。
- PostgreSQL smoke 查询：`marketing_event_inbox` 存在派生幂等键；`marketing_audit` 存在 `WEBHOOK_RECEIVED`、`WEBHOOK_DUPLICATE` 审计，payload 记录 `rawPayloadSha256`、`signatureSha256`，未出现 receiver secret、完整 signature 或完整 raw payload。
- 构建验证：`go build ./...` 通过。

## Commands

```bash
export EX_MARKETING_WEBHOOK_RECEIVERS_JSON='[{"receiverCode":"payment-webhook","enabled":true,"secret":"receiver-secret","allowedTenantIds":["site_001"],"allowedSourceSystems":["payment"],"allowedIPs":[],"mapping":{"eventId":{"path":"$.id"},"eventType":{"const":"DEPOSIT_SUCCESS"},"sourceSystem":{"const":"payment"},"tenantId":{"const":"site_001"},"subject.type":{"const":"USER"},"subject.id":{"path":"$.user.id"},"biz.bizType":{"const":"DEPOSIT"},"biz.bizId":{"path":"$.payment.id"},"occurredAt":{"path":"$.createdAt"},"attributes":{"amount":{"path":"$.payment.amount"},"currency":{"path":"$.payment.currency"},"providerEventId":{"path":"$.id"}}}}]'
go test ./...

export EX_MARKETING_WEBHOOK_RECEIVERS_JSON='[{"receiverCode":"payment-webhook","enabled":true,"secret":"receiver-secret","allowedTenantIds":["site_001"],"allowedSourceSystems":["payment"],"allowedIPs":[],"mapping":{"eventId":{"path":"$.id"},"eventType":{"const":"DEPOSIT_SUCCESS"},"sourceSystem":{"const":"payment"},"tenantId":{"const":"site_001"},"subject.type":{"const":"USER"},"subject.id":{"path":"$.user.id"},"biz.bizType":{"const":"DEPOSIT"},"biz.bizId":{"path":"$.payment.id"},"occurredAt":{"path":"$.createdAt"},"attributes":{"amount":{"path":"$.payment.amount"},"currency":{"path":"$.payment.currency"},"providerEventId":{"path":"$.id"}}}}]'
go test ./... -run 'Webhook|Event|Inbox|Idempot|Quota|Audit'

export EX_MARKETING_WEBHOOK_RECEIVERS_JSON='[{"receiverCode":"payment-webhook","enabled":true,"secret":"receiver-secret","allowedTenantIds":["site_001"],"allowedSourceSystems":["payment"],"allowedIPs":[],"mapping":{"eventId":{"path":"$.id"},"eventType":{"const":"DEPOSIT_SUCCESS"},"sourceSystem":{"const":"payment"},"tenantId":{"const":"site_001"},"subject.type":{"const":"USER"},"subject.id":{"path":"$.user.id"},"biz.bizType":{"const":"DEPOSIT"},"biz.bizId":{"path":"$.payment.id"},"occurredAt":{"path":"$.createdAt"},"attributes":{"amount":{"path":"$.payment.amount"},"currency":{"path":"$.payment.currency"},"providerEventId":{"path":"$.id"}}}}]'
go test -race ./... -run 'Webhook|Idempot|Concurrent|Quota'

export EX_MARKETING_WEBHOOK_RECEIVERS_JSON='[{"receiverCode":"payment-webhook","enabled":true,"secret":"receiver-secret","allowedTenantIds":["site_001"],"allowedSourceSystems":["payment"],"allowedIPs":[],"mapping":{"eventId":{"path":"$.id"},"eventType":{"const":"DEPOSIT_SUCCESS"},"sourceSystem":{"const":"payment"},"tenantId":{"const":"site_001"},"subject.type":{"const":"USER"},"subject.id":{"path":"$.user.id"},"biz.bizType":{"const":"DEPOSIT"},"biz.bizId":{"path":"$.payment.id"},"occurredAt":{"path":"$.createdAt"},"attributes":{"amount":{"path":"$.payment.amount"},"currency":{"path":"$.payment.currency"},"providerEventId":{"path":"$.id"}}}}]'
go test ./... -tags=integration -run 'Webhook'

docker compose up -d postgres

export EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable'
export EX_MARKETING_WEBHOOK_RECEIVERS_JSON='[{"receiverCode":"payment-webhook","enabled":true,"secret":"receiver-secret","allowedTenantIds":["site_001"],"allowedSourceSystems":["payment"],"allowedIPs":[],"mapping":{"eventId":{"path":"$.id"},"eventType":{"const":"DEPOSIT_SUCCESS"},"sourceSystem":{"const":"payment"},"tenantId":{"const":"site_001"},"subject.type":{"const":"USER"},"subject.id":{"path":"$.user.id"},"biz.bizType":{"const":"DEPOSIT"},"biz.bizId":{"path":"$.payment.id"},"occurredAt":{"path":"$.createdAt"},"attributes":{"amount":{"path":"$.payment.amount"},"currency":{"path":"$.payment.currency"},"providerEventId":{"path":"$.id"}}}}]'
go test ./internal/postgres -count=1

export EX_MARKETING_ADDR='127.0.0.1:18080'
export EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable'
export EX_MARKETING_WEBHOOK_RECEIVERS_JSON='[{"receiverCode":"payment-webhook","enabled":true,"secret":"receiver-secret","allowedTenantIds":["site_001"],"allowedSourceSystems":["payment"],"allowedIPs":[],"mapping":{"eventId":{"path":"$.id"},"eventType":{"const":"DEPOSIT_SUCCESS"},"sourceSystem":{"const":"payment"},"tenantId":{"const":"site_001"},"subject.type":{"const":"USER"},"subject.id":{"path":"$.user.id"},"biz.bizType":{"const":"DEPOSIT"},"biz.bizId":{"path":"$.payment.id"},"occurredAt":{"path":"$.createdAt"},"attributes":{"amount":{"path":"$.payment.amount"},"currency":{"path":"$.payment.currency"},"providerEventId":{"path":"$.id"}}}}]'
go run ./cmd/exmarketing

timestamp="$(date +%s)"
body='{"id":"provider_evt_001_testagent_20260526","user":{"id":"user_001"},"payment":{"id":"pay_001_testagent_20260526","amount":100,"currency":"USD"},"createdAt":"2026-05-24T12:00:00Z"}'
signature="$(printf '%s.%s' "$timestamp" "$body" | openssl dgst -sha256 -hmac 'receiver-secret' -binary | xxd -p -c 256)"
curl -i -sS \
  -X POST 'http://127.0.0.1:18080/openapi/v1/webhooks/payment-webhook' \
  -H 'Content-Type: application/json' \
  -H "X-ExMarketing-Timestamp: $timestamp" \
  -H "X-ExMarketing-Signature: $signature" \
  --data "$body"

# Re-run the same curl body with a fresh timestamp/signature for duplicate smoke.

docker exec exmarketing-postgres psql -U exmarketing -d exmarketing \
  -c "select id, idempotency_key, process_status from marketing_event_inbox where idempotency_key = 'site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_001_testagent_20260526';" \
  -c "select action, resource, resource_id, payload_json from marketing_audit where resource = 'WEBHOOK_RECEIVER' and resource_id = 'payment-webhook' order by created_at desc limit 5;"

export EX_MARKETING_WEBHOOK_RECEIVERS_JSON='[{"receiverCode":"payment-webhook","enabled":true,"secret":"receiver-secret","allowedTenantIds":["site_001"],"allowedSourceSystems":["payment"],"allowedIPs":[],"mapping":{"eventId":{"path":"$.id"},"eventType":{"const":"DEPOSIT_SUCCESS"},"sourceSystem":{"const":"payment"},"tenantId":{"const":"site_001"},"subject.type":{"const":"USER"},"subject.id":{"path":"$.user.id"},"biz.bizType":{"const":"DEPOSIT"},"biz.bizId":{"path":"$.payment.id"},"occurredAt":{"path":"$.createdAt"},"attributes":{"amount":{"path":"$.payment.amount"},"currency":{"path":"$.payment.currency"},"providerEventId":{"path":"$.id"}}}}]'
go build ./...
```

## Result

```text
PASS
```

## Evidence

- `go test ./...`: PASS. `internal/httpapi` executed in `0.566s`; all packages passed or had no test files.
- `go test ./... -run 'Webhook|Event|Inbox|Idempot|Quota|Audit'`: PASS. `internal/httpapi`, `internal/engine`, `internal/marketing`, `internal/mqconsumer`, `internal/postgres`, `internal/rule` executed matching tests; other packages reported no matching tests where applicable.
- `go test -race ./... -run 'Webhook|Idempot|Concurrent|Quota'`: PASS. No race detector failures.
- `go test ./... -tags=integration -run 'Webhook'`: PASS. `internal/httpapi` executed matching Webhook tests; most other packages had no matching tests.
- `docker compose up -d postgres`: PASS. `exmarketing-postgres` was already running.
- `go test ./internal/postgres -count=1` with `EX_MARKETING_DATABASE_URL`: PASS.
- Manual HTTP smoke first request:

```text
HTTP/1.1 202 Accepted
"eventId":"provider_evt_001_testagent_20260526"
"idempotencyKey":"site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_001_testagent_20260526"
"processStatus":"PROCESSED"
"duplicate":false
```

- Manual HTTP smoke duplicate request:

```text
HTTP/1.1 200 OK
"processStatus":"DUPLICATE"
"duplicate":true
```

- PostgreSQL smoke:

```text
marketing_event_inbox:
inbox_2137bd9f7932a8a69acf8d67 | site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_001_testagent_20260526 | PROCESSED

marketing_audit:
WEBHOOK_DUPLICATE | WEBHOOK_RECEIVER | payment-webhook | payload_json includes receiverCode, eventId, idempotencyKey, rawPayloadSha256, signatureSha256
WEBHOOK_RECEIVED  | WEBHOOK_RECEIVER | payment-webhook | payload_json includes receiverCode, eventId, idempotencyKey, rawPayloadSha256, signatureSha256
```

- Host `psql` command was unavailable (`zsh:1: command not found: psql`), so PostgreSQL smoke was executed with `docker exec exmarketing-postgres psql ...`.
- The table name in the actual schema is `marketing_event_inbox`; the `test-plan.md` command used `event_inbox` as an adjustable placeholder and that query failed with `relation "event_inbox" does not exist`. Re-running against `marketing_event_inbox` passed.
- `go build ./...`: PASS.

## Not Covered / Residual Risk

- No standalone end-to-end fixture was created for a real activity producing participation, grant_order, outbox, and quota exhaustion through the Webhook path. Existing Go tests cover Event/Quota/Audit paths and Webhook HTTP mapping/idempotency paths, while the manual Webhook smoke used the running service with no matching activity and therefore returned `participations:null`, `grantOrders:null`, `outboxMessages:null`.
- No 20-100 request external HTTP concurrency smoke was run. Race-targeted Go tests passed, but if production-grade concurrent HTTP smoke is required it should be added as a dedicated test fixture.

需求：2026-05-24-webhook-receiver
