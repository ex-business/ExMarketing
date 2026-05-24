# Test

## Agent 指令

你是 Test Agent。请根据 task.md、analysis.md、review.md、test-plan.md 和 implementation.md 执行完整测试。

最低要求：

~~~bash
go test ./...
go build ./...
~~~

## Test Cases

- HTTP Event API:
  - 首次合法事件返回 `202 Accepted`，生成 grant order 和 outbox。
  - 缺少 `Idempotency-Key` 返回 `400 INVALID_IDEMPOTENCY_KEY`。
  - `Idempotency-Key` 与 payload 不匹配返回 `400 INVALID_IDEMPOTENCY_KEY`。
  - 顶层未知字段、`subject` 嵌套未知字段、缺少必填字段返回 `400 INVALID_EVENT`。
  - `attributes: null` 和缺失 `attributes` 归一化为 `{}`。
  - 完全重复事件返回 `200 OK` 且 `duplicate=true`，响应不包含新的 grant/outbox。
  - 同一幂等键不同 payload 返回 `400 INVALID_IDEMPOTENCY_KEY`。
  - quota 不足路径返回 `409 QUOTA_EXCEEDED`，且无 grant/outbox。
- Repository / canonical payload:
  - Memory repository 首次写入、重复写入、同 key 不同 payload 冲突通过。
  - PostgreSQL repository 首次写入、重复写入、同 key 不同 payload 冲突通过。
  - JSON 数字在 `json.Number` 与 `float64` 表示下的重复事件比较通过。
- Field boundary:
  - rule evaluator 支持标准字段含 `occurredAt` 与 `attributes.*`。
  - reward `baseField=attributes.amount` 正常，非 `attributes.*` 返回错误。
- PostgreSQL smoke:
  - `marketing_event_inbox.idempotency_key` 唯一约束存在。
  - 带真实 PostgreSQL 的 `go test ./...` 通过。
- Concurrency:
  - 临时 HTTP 并发测试验证 20 个 goroutine 同时提交相同事件时仅 1 个首次处理，其余 duplicate。
  - 临时 HTTP 并发测试验证 20 个 goroutine 同时提交同一 key 不同 payload 时仅 1 个首次处理，其余为 duplicate 或幂等冲突。
  - 临时测试已在验证后删除，未作为业务代码变更保留。

## Commands

~~~bash
go test ./...
go build ./...
go test -race ./...
docker compose up -d postgres
docker compose exec -T postgres psql -U exmarketing -d exmarketing -f /docker-entrypoint-initdb.d/001_marketing_core.sql
docker compose exec -T postgres psql -U exmarketing -d exmarketing -f /docker-entrypoint-initdb.d/002_audit_fact_outbox_indexes.sql
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./...
docker compose exec -T postgres psql -U exmarketing -d exmarketing -c "SELECT conname, pg_get_constraintdef(c.oid) AS def FROM pg_constraint c JOIN pg_class t ON t.oid = c.conrelid WHERE t.relname = 'marketing_event_inbox' AND pg_get_constraintdef(c.oid) LIKE '%idempotency_key%';"
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test -count=1 ./internal/httpapi ./internal/marketing ./internal/postgres ./internal/reward ./internal/rule
go test -race -count=1 ./internal/httpapi -run TestTestAgentConcurrent
go test ./internal/httpapi
~~~

## Result

~~~text
PASS
~~~

## Evidence

- `go test ./...` passed.
- `go build ./...` passed.
- `go test -race ./...` passed.
- PostgreSQL-backed `EX_MARKETING_DATABASE_URL=... go test ./...` passed.
- PostgreSQL unique constraint check returned:

~~~text
marketing_event_inbox_idempotency_key_key | UNIQUE (idempotency_key)
~~~

- `go test -count=1 ./internal/httpapi ./internal/marketing ./internal/postgres ./internal/reward ./internal/rule` passed.
- Temporary concurrency verification passed:

~~~text
ok github.com/ex-stack/ExMarketing/internal/httpapi 1.636s
~~~

Run logs:

- `tasks/2026-05-24-113400/runs/20260524T121903Z-test-current.log`
- `tasks/2026-05-24-113400/runs/20260524T121913Z-race-current.log`
- `tasks/2026-05-24-113400/runs/20260524T121931Z-postgres-current.log`
- `tasks/2026-05-24-113400/runs/20260524T121952Z-postgres-verify-current.log`
- `tasks/2026-05-24-113400/runs/20260524T122045Z-concurrency-current.log`

Note: re-running `001_marketing_core.sql` against the existing local PostgreSQL container printed `relation already exists` messages because the schema had already been initialized. This did not block the smoke result; the expected unique constraint was verified with a read-only query and the PostgreSQL-backed tests passed.

需求：2026-05-24-113400
