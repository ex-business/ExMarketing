# Test

## Scope

Test Agent 按 `task.md`、`analysis.md`、`review.md`、`test-plan.md`、`implementation.md` 执行验证。

本轮只写入本文件；未修改业务代码、测试代码、其他文档或配置。

## Environment

- 仓库：`/Users/tong/Documents/GitHub/OracleBetX-Projects/ExMarketing`
- PostgreSQL：`docker-compose.yml` service `postgres`
- PostgreSQL DSN：`postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable`
- 说明：`exmarketing-postgres` 在测试时已处于 `Running`，本轮未执行 `docker compose down`，避免停止当前本地共享服务。

## Commands

```bash
go test ./internal/worker -run TestOutboxWorker -count=1 -v
go test ./...
go build ./...
docker compose up -d postgres
docker compose exec -T postgres pg_isready -U exmarketing -d exmarketing
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./internal/postgres -count=1 -v
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./... -count=1
go test ./internal/worker -list 'Outbox|Sender|Claim|Concurrent'
go test ./internal/marketing ./internal/engine ./internal/httpapi ./internal/postgres -list 'Outbox|Quota|Insufficient|Reject|Zero|Duplicate|Idempot|Concurrent|Claim|Postgres'
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./... -run 'Outbox|Postgres|Sender|Claim|Concurrent' -count=1
go test ./internal/postgres -list 'Outbox|Claim|Concurrent|Worker|Sender'
```

## Results

- `go test ./internal/worker -run TestOutboxWorker -count=1 -v`：PASS。
  - 覆盖 `HTTP 200` 成功、`WEBHOOK 2xx` 成功、`302` 不跟随、grant order 状态推进、超时、网络错误、`5xx` retry、`429 Retry-After`、不可重试 `4xx` DLQ、最大重试 DLQ、非法 target、非法 payload、`MQ` 跳过。
- `go test ./...`：PASS。
- `go build ./...`：PASS。
- `docker compose up -d postgres`：PASS，容器 `exmarketing-postgres` Running。
- `docker compose exec -T postgres pg_isready -U exmarketing -d exmarketing`：PASS，`/var/run/postgresql:5432 - accepting connections`。
- `EX_MARKETING_DATABASE_URL=... go test ./internal/postgres -count=1 -v`：PASS，`ok github.com/ex-stack/ExMarketing/internal/postgres 1.699s`。
  - 覆盖 PostgreSQL atomic grant rollback、duplicate/conflict inbox、activity unique/update、participation/grant idempotency constraints、outbox create rollback、quota insufficient no dirty writes、callback idempotency/tenant isolation、concurrent budget/stock。
- `EX_MARKETING_DATABASE_URL=... go test ./... -count=1`：PASS，数据库环境下全仓测试通过。
- `EX_MARKETING_DATABASE_URL=... go test ./... -run 'Outbox|Postgres|Sender|Claim|Concurrent' -count=1`：PASS，相关命名用例通过。

## Coverage Evidence

- HTTP / WEBHOOK sender：
  - `TestOutboxWorkerSendsHTTPAndMarksSent`
  - `TestOutboxWorkerSendsWebhookAndMarksSent`
  - `TestOutboxWorkerDoesNotFollowRedirects`
  - `TestOutboxWorkerTimeoutIsRetryableFailure`
  - `TestOutboxWorkerNetworkErrorIsRetryableFailure`
  - `TestOutboxWorkerRetryableFailureUsesExponentialBackoff`
  - `TestOutboxWorkerRetryAfterOverridesBackoff`
  - `TestOutboxWorkerNonRetryableFailureMovesToDLQ`
  - `TestOutboxWorkerMaxRetriesMovesToDLQ`
  - `TestOutboxWorkerInvalidTargetMovesToDLQ`
  - `TestOutboxWorkerInvalidPayloadMovesToDLQ`
  - `TestOutboxWorkerSkipsMQMessages`
- 上游回归：
  - quota / stock 不足和回滚：`TestPostgresAtomicGrantRollsBackOnStockExceeded`、`TestPostgresQuotaInsufficientBudgetHasNoDirtyWrites`
  - 规则拒绝不生成 grant/outbox：`TestActivityEventRuleRejectionReturnsExplainWithoutGrant`
  - simulate quota exceeded suppress grant preview：`TestSimulateEventQuotaExceededSuppressesGrantPreview`
  - duplicate / idempotency：`TestActivityEventDuplicateAndConflict`、`TestPostgresParticipationAndGrantIdempotencyUniqueConstraints`、`TestMemoryConcurrentDuplicateGrantRequestOnlyProcessesOnce`
  - DLQ replay/close API：`TestOutboxDLQQueryReplayAndClose`
- PostgreSQL smoke：
  - 真实 PostgreSQL DSN 下 `internal/postgres` 和全仓测试均通过。
  - 仓库实现中 `ListPendingOutbox` 使用 `FOR UPDATE SKIP LOCKED` claim，并将 `PENDING` / due `FAILED` 更新为 `SENDING` 后返回。

## Notes

- `go test ./internal/postgres -list 'Outbox|Claim|Concurrent|Worker|Sender'` 显示现有 PostgreSQL 测试没有专门命名的 outbox sender 多 worker claim 用例；本轮受限于只能写 `test.md`，未新增测试代码。
- 已通过真实 PostgreSQL 全仓 smoke、现有并发仓储测试、以及源码检查确认 `ListPendingOutbox` 的 claim SQL 存在，但没有执行“多个真实 worker + mock HTTP server + PostgreSQL claim”的新专用场景。

## Result

```text
PASS
```

需求：2026-05-24-outbox-http-sender
