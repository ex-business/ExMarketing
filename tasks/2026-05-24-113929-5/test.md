# Test

## Scope

- 已确认 `review.md` Decision 为 `PASS`。
- 按 `test-plan.md` 验证 Outbox retry / DLQ / replay / close、grant_order `RETRY/CLOSED` 同步、callback quota 终结、PostgreSQL 仓储语义、并发 claim/race 基础回归。
- 本轮仅写入本文件，未修改业务代码、测试代码、文档代码或配置文件。

## Commands

```bash
docker compose up -d postgres
```

Result: PASS

```bash
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./internal/marketing ./internal/httpapi ./internal/worker ./internal/postgres
```

Result: PASS

```bash
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test -race ./internal/worker ./internal/postgres
```

Result: PASS

```bash
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./...
```

Result: PASS

```bash
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go build ./...
```

Result: PASS

```bash
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test -count=1 ./...
```

Result: PASS

## Evidence

定向包测试通过：

```text
ok  	github.com/ex-stack/ExMarketing/internal/marketing	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/httpapi	0.626s
ok  	github.com/ex-stack/ExMarketing/internal/worker	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/postgres	0.659s
```

Race 测试通过：

```text
ok  	github.com/ex-stack/ExMarketing/internal/worker	1.272s
ok  	github.com/ex-stack/ExMarketing/internal/postgres	1.989s
```

全量无缓存测试通过：

```text
ok  	github.com/ex-stack/ExMarketing/internal/decimal	0.193s
ok  	github.com/ex-stack/ExMarketing/internal/engine	0.309s
ok  	github.com/ex-stack/ExMarketing/internal/fact	0.474s
ok  	github.com/ex-stack/ExMarketing/internal/httpapi	0.841s
ok  	github.com/ex-stack/ExMarketing/internal/marketing	0.955s
ok  	github.com/ex-stack/ExMarketing/internal/mqconsumer	0.816s
ok  	github.com/ex-stack/ExMarketing/internal/postgres	1.387s
ok  	github.com/ex-stack/ExMarketing/internal/reward	1.194s
ok  	github.com/ex-stack/ExMarketing/internal/rule	1.316s
ok  	github.com/ex-stack/ExMarketing/internal/worker	1.462s
ok  	github.com/ex-stack/ExMarketing/internal/workflow	1.778s
```

相关测试名已存在并被上述命令覆盖：

```text
TestMemoryCloseOutboxDLQClosesGrantAndReleasesQuota
TestPostgresCloseOutboxDLQClosesGrantAndReleasesQuota
TestOutboxDLQQueryReplayAndClose
TestOutboxDLQOperationRequiresOperatorAndReason
TestOutboxWorkerAdvancesGrantOrderState
TestOutboxWorkerTimeoutIsRetryableFailure
TestOutboxWorkerNetworkErrorIsRetryableFailure
TestOutboxWorkerRetryableFailureUsesExponentialBackoff
TestOutboxWorkerRetryAfterOverridesBackoff
TestOutboxWorkerNonRetryableFailureMovesToDLQ
TestOutboxWorkerMaxRetriesMovesToDLQ
TestOutboxWorkerInvalidTargetMovesToDLQ
TestOutboxWorkerMQSendFailureKeepsOutboxRetryable
TestOutboxWorkerMQInvalidTargetMovesToDLQ
TestPostgresCallbackConfirmReleaseIdempotencyAndConflict
TestPostgresQuotaInsufficientBudgetHasNoDirtyWrites
TestMemoryConcurrentQuotaCompetitionDoesNotOverReserve
```

PostgreSQL smoke 查询：

```bash
docker compose exec -T postgres psql -U exmarketing -d exmarketing -c "SELECT send_status, retry_count, next_retry_at, error_message FROM marketing_outbox ORDER BY created_at DESC LIMIT 10;"
docker compose exec -T postgres psql -U exmarketing -d exmarketing -c "SELECT grant_status, retry_count, error_message FROM marketing_grant_order ORDER BY created_at DESC LIMIT 10;"
docker compose exec -T postgres psql -U exmarketing -d exmarketing -c "SELECT reservation_status, reserved_amount FROM marketing_quota_reservation ORDER BY created_at DESC LIMIT 10;"
docker compose exec -T postgres psql -U exmarketing -d exmarketing -c "SELECT quota_type, total_value, used_value, frozen_value, version FROM marketing_quota ORDER BY created_at DESC LIMIT 10;"
```

Result: PASS. 查询返回 0 rows，说明当前测试清理后无残留业务数据；表结构和字段可访问。

说明：`test-plan.md` 建议的查询使用了 `marketing_quota_reservation.amount`，当前 schema 实际字段为 `reserved_amount`。原建议查询失败日志为：

```text
ERROR:  column "amount" does not exist
LINE 1: SELECT reservation_status, amount FROM marketing_quota_reser...
                                   ^
```

复现方式：

```bash
docker compose exec -T postgres psql -U exmarketing -d exmarketing -c "SELECT reservation_status, amount FROM marketing_quota_reservation ORDER BY created_at DESC LIMIT 10;"
```

失败原因：测试计划中的 smoke 查询字段名与当前数据库 schema 不一致，不是业务实现失败。已使用 `reserved_amount` 重跑并通过。

## Result

```text
PASS
```

需求：2026-05-24-113929-5
