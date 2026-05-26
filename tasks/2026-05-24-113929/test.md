# Test

## Scope

按 `task.md`、`analysis.md`、`review.md`、`test-plan.md`、`implementation.md` 执行需求 `2026-05-24-113929`：活动主配置模型。

`review.md` Decision 为 `PASS`，`test-plan.md` Decision 为 `PASS`。本轮只写本文件，未修改业务代码、测试代码、文档代码或配置文件。

## Commands

```bash
go test ./...
go build ./...
go test ./... -run 'Activity|MarketingActivity'
go test ./... -run 'Postgres|PostgreSQL|Repository|Migration'
go test ./... -run 'Quota|Budget|Insufficient|Limit|Stock'
docker compose up -d postgres
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./internal/postgres -count=1 -run 'Postgres|PostgreSQL|Smoke|Activity|Repository|Migration|Quota|Budget|Insufficient|Limit|Stock' -v
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./... -count=1 -run 'Postgres|PostgreSQL|Smoke|Activity|MarketingActivity|Quota|Budget|Insufficient|Limit|Stock'
docker compose exec -T postgres psql -U exmarketing -d exmarketing -c "SELECT conname, pg_get_constraintdef(oid) FROM pg_constraint WHERE conrelid = 'marketing_activity'::regclass AND conname = 'marketing_activity_tenant_code_key';"
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./internal/postgres -count=1 -run '^TestPostgresActivityUniqueDetailAndUpdate$' -v
go test ./internal/httpapi -count=1 -run 'Activity|MarketingActivity' -v
go test ./internal/marketing -count=1 -run 'Activity|MarketingActivity|Quota|Budget|Insufficient|Limit|Stock' -v
```

## Passed Checks

- `go test ./...` passed.
- `go build ./...` passed.
- `go test ./... -run 'Activity|MarketingActivity'` passed.
- `go test ./... -run 'Postgres|PostgreSQL|Repository|Migration'` passed.
- `go test ./... -run 'Quota|Budget|Insufficient|Limit|Stock'` passed.
- `docker compose up -d postgres` passed; container `exmarketing-postgres` was running.
- PostgreSQL smoke passed with `EX_MARKETING_DATABASE_URL`, including:
  - `TestPostgresAtomicGrantRollsBackOnStockExceeded`
  - `TestPostgresCreateInboxDetectsDuplicateAndConflict`
  - `TestPostgresActivityUniqueDetailAndUpdate`
  - `TestPostgresParticipationAndGrantIdempotencyUniqueConstraints`
  - `TestPostgresAtomicGrantRollsBackWhenGrantOrderCreateFails`
  - `TestPostgresAtomicGrantRollsBackWhenOutboxCreateFails`
- Full DSN-backed targeted suite passed:

```bash
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./... -count=1 -run 'Postgres|PostgreSQL|Smoke|Activity|MarketingActivity|Quota|Budget|Insufficient|Limit|Stock'
```

- PostgreSQL unique constraint exists:

```text
marketing_activity_tenant_code_key | UNIQUE (tenant_id, activity_code)
```

- Narrow regression command for the previous PostgreSQL duplicate activity failure passed:

```bash
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./internal/postgres -count=1 -run '^TestPostgresActivityUniqueDetailAndUpdate$' -v
```

- HTTP activity tests passed, covering activity create/read/update, same-tenant duplicate `activityCode`, identity-field update rejection, invalid config/rule schema, event validation, and budget/user-limit/stock exceeded regressions.
- Marketing repository quota/stock regression tests passed, including rollback on stock exceeded and concurrent quota competition.

## Failed Checks

None.

## Notes

- `Quota|Budget|Insufficient|Limit|Stock` tests were found and executed; existing budget, user limit, stock, and quota exceeded behavior remained passing.
- PostgreSQL integration tests were executed against the repository-provided `docker-compose.yml` PostgreSQL service on `localhost:54329`.
- The earlier failure mode documented in the previous `test.md` no longer reproduces: duplicate `(tenant_id, activity_code)` on PostgreSQL-backed `CreateActivity` now passes the test expectation for domain conflict handling.

## Result

```text
PASS
```

需求：2026-05-24-113929
