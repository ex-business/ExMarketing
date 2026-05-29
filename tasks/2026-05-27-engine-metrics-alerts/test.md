# Test

## Agent 指令

你是 Test Agent。请根据 `task.md`、`analysis.md`、`review.md`、`test-plan.md` 和 `implementation.md` 执行测试。

本轮只写本文件，未修改业务代码、测试代码、配置或其他文档。

## Environment

- 仓库：`/Users/tong/Documents/GitHub/OracleBetX-Projects/ExMarketing`
- PostgreSQL：`docker compose ps postgres` 显示 `exmarketing-postgres` 已运行 3 天且 `healthy`
- PostgreSQL URL：`postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable`
- 本轮未执行 `docker compose down`，避免停止已有本地服务。

## Static Checks

- `review.md` Decision 为 `PASS`。
- `docs/operations.md` 已声明 `GET /metrics`、Prometheus text format、内网采集边界、Counter 重启语义、Gauge 从仓储刷新。
- 文档覆盖七类核心指标：event intake、quota fail rate、outbox backlog、DLQ count、callback fail rate、worker claim latency、reward command latency。
- 告警阈值覆盖 5 分钟失败率、outbox backlog count/age、DLQ > 0、worker claim P95、reward command P95。
- Review P2 已覆盖：quota 指标包含 `action="reserve"`，文档声明 fail rate 只以 reserve attempts 为分母，避免 confirm/release 稀释。
- 指标标签实现未使用 `eventId`、`bizId`、`subjectId`、`billNo`、`commandId`、`targetRef`、payload、token、Authorization、Cookie 或原始错误文本作为 metrics 标签。

## Commands

```bash
go test ./...
```

结果：PASS。全量 Go 测试通过；覆盖 `internal/httpapi`、`internal/engine`、`internal/marketing`、`internal/postgres`、`internal/worker` 等包。

```bash
go build ./...
```

结果：PASS。

```bash
go test ./... -run 'Metrics|Metric|Prometheus|Recorder|Collector'
```

结果：PASS。命中 `TestMetricsEndpointRendersEventAndOutboxMetrics`，验证 `/metrics` 返回 event failure counter、outbox backlog gauge、oldest eligible age 等 Prometheus 文本片段。

```bash
go test ./... -run 'Event|Intake|Idempot|Quota'
```

结果：PASS。覆盖 event schema/auth/idempotency、quota exceeded、quota competition、PostgreSQL quota dirty-write 防护等相关测试。

```bash
go test ./... -run 'Outbox|Worker|Claim|DLQ|Callback'
```

结果：PASS。覆盖 outbox retry/DLQ/claim、worker send/retry/max-retry、callback success/duplicate/conflict/tenant isolation 等相关测试。

```bash
go test ./... -run 'Postgres|PostgreSQL|Smoke|Integration'
```

结果：PASS。无 DSN 的通用过滤命令通过；随后使用真实 PostgreSQL DSN 执行强 smoke。

```bash
go test ./... -race -run 'Quota|Worker|Claim|Idempot'
```

结果：PASS。race 定向测试通过。

```bash
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' \
go test ./internal/postgres -run 'TestPostgresOutbox|TestPostgresCloseOutboxDLQOnlyClosesOutbox|TestPostgresCallback|TestPostgresConcurrent|TestPostgresQuota|TestPostgresCreateInbox' -count=1 -v
```

结果：PASS。真实 PostgreSQL 执行并通过：

- `TestPostgresCreateInboxDetectsDuplicateAndConflict`
- `TestPostgresCloseOutboxDLQOnlyClosesOutbox`
- `TestPostgresOutboxRetryEligibilityAndTerminalStatusNotClaimed`
- `TestPostgresOutboxConcurrentClaimSingleMessage`
- `TestPostgresOutboxConcurrentClaimMultipleMessages`
- `TestPostgresOutboxStaleRecoveryConcurrentClaim`
- `TestPostgresOutboxDLQCountByTenantAndChannel`
- `TestPostgresQuotaInsufficientBudgetHasNoDirtyWrites`
- `TestPostgresCallbackConfirmReleaseIdempotencyAndConflict`
- `TestPostgresCallbackTenantIsolation`
- `TestPostgresConcurrentBudgetDoesNotOverspend`
- `TestPostgresConcurrentStockLastItem`

```bash
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' \
go test ./internal/postgres ./internal/httpapi ./internal/engine -run 'Postgres|Outbox|DLQ|Callback|Quota|Idempot|Metrics' -count=1 -v
```

结果：PASS。真实 PostgreSQL + HTTP/engine 定向命令通过，包含 metrics endpoint、quota preview/exceeded、idempotency、callback、DLQ 操作和 PostgreSQL outbox/quota/callback/concurrency 用例。

```bash
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' \
go run ./cmd/engine-e2e-postgres
```

结果：PASS。

输出摘要：

```text
PASS runId=e2e_pg_20260529_102016_000000000 tenantId=site_001_e2e_pg_20260529_102016_000000000 activityId=act_cc702e07c8999a1097388abf activityCode=e2e_pg_20260529_102016_000000000 successCommand=cmd_c65a3a87ce72b11008bfd9e8 failedCommand=cmd_a0227d4692eaa3062a3f29c1
```

## Coverage Mapping

| Test-plan area | Result | Evidence |
| --- | --- | --- |
| Metrics endpoint / parseable output | PASS | `TestMetricsEndpointRendersEventAndOutboxMetrics` and metrics static inspection. |
| 指标文档和告警文档 | PASS | `docs/operations.md` lists endpoint, metrics, labels, threshold windows and exposure boundary. |
| 正常 Event -> Quota -> Outbox -> Worker -> Callback | PASS | `cmd/engine-e2e-postgres` completed success and failed callback paths against real PostgreSQL. |
| 额度不足路径 | PASS | HTTP quota exceeded tests and real PostgreSQL `TestPostgresQuotaInsufficientBudgetHasNoDirtyWrites`. |
| 幂等重复路径 | PASS | HTTP idempotency tests and `TestPostgresCreateInboxDetectsDuplicateAndConflict`. |
| Callback 重复、冲突和失败 | PASS | HTTP callback tests and real PostgreSQL callback idempotency/conflict/tenant isolation tests. |
| Outbox backlog / DLQ | PASS | Metrics endpoint test covers backlog gauge; real PostgreSQL outbox retry, DLQ count and close tests pass. |
| Worker claim latency / concurrency | PASS | Worker claim/outbox tests, PostgreSQL concurrent claim tests and race command pass. |
| PostgreSQL smoke / 重启恢复口径 | PASS | Real DSN PostgreSQL tests and E2E command pass; Gauge implementation refreshes from repository snapshot on scrape. |
| 敏感信息和高基数标签 | PASS | Static inspection confirms metrics labels use bounded business dimensions and sanitized values, not sensitive request identifiers or payloads. |

## Result

```text
PASS
```

需求：2026-05-27-engine-metrics-alerts
