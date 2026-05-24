# Test

## Agent 指令

根据 `task.md`、`analysis.md`、`review.md`、`test-plan.md`、`implementation.md` 执行测试。

本次只写入本文件，未修改业务代码、测试代码、文档代码或配置文件。

## Commands

~~~bash
go test ./...
go build ./...
go test -race ./...
EXMARKETING_TEST_POSTGRES_DSN='postgres://postgres:postgres@127.0.0.1:5432/exmarketing_test?sslmode=disable' go test ./... -run 'Postgres|Integration|Smoke'
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test -count=1 ./internal/postgres -run 'TestPostgres' -v
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test -count=1 ./...
~~~

## Result

~~~text
PASS
~~~

## Evidence

- `go test ./...` passed.
- `go build ./...` passed.
- `go test -race ./...` passed.
- PostgreSQL container `exmarketing-postgres` was already running and healthy on `localhost:54329`.
- Repository PostgreSQL smoke passed with the documented project DSN:

~~~text
=== RUN   TestPostgresAtomicGrantRollsBackOnStockExceeded
--- PASS: TestPostgresAtomicGrantRollsBackOnStockExceeded (0.03s)
=== RUN   TestPostgresCreateInboxDetectsDuplicateAndConflict
--- PASS: TestPostgresCreateInboxDetectsDuplicateAndConflict (0.01s)
PASS
ok github.com/ex-stack/ExMarketing/internal/postgres 0.238s
~~~

- Full suite with PostgreSQL repository enabled also passed:

~~~text
ok github.com/ex-stack/ExMarketing/internal/httpapi 0.303s
ok github.com/ex-stack/ExMarketing/internal/marketing 0.318s
ok github.com/ex-stack/ExMarketing/internal/postgres 0.510s
ok github.com/ex-stack/ExMarketing/internal/reward 0.748s
ok github.com/ex-stack/ExMarketing/internal/rule 0.613s
ok github.com/ex-stack/ExMarketing/internal/workflow 1.020s
~~~

## Coverage Notes

- `internal/reward/calculator_test.go` covers `FIXED` rounding, `RATIO` base field and cap, `LADDER` closed interval, `RANDOM` stable seed, `PROBABILITY` zero reward skip, `POOL_SPLIT` weight split, and explicit errors for empty config, missing field, type mismatch, unmatched ladder, and unsupported calculate type.
- Existing HTTP and repository tests cover quota exceeded rollback paths, duplicate/conflicting inbox behavior, memory atomic grant rollback, PostgreSQL atomic rollback, and idempotency canonicalization.
- `go test -race ./...` provides race coverage for existing concurrency-sensitive tests, but no new temporary concurrency tests were added because this Test Agent is not allowed to modify test code.

## Non-blocking Failed Attempts

### Test-plan DSN variable

Command:

~~~bash
EXMARKETING_TEST_POSTGRES_DSN='postgres://postgres:postgres@127.0.0.1:5432/exmarketing_test?sslmode=disable' go test ./... -run 'Postgres|Integration|Smoke'
~~~

Result summary:

~~~text
ok github.com/ex-stack/ExMarketing/internal/postgres 0.877s
~~~

Reason: this repository's PostgreSQL tests read `EX_MARKETING_DATABASE_URL`, not `EXMARKETING_TEST_POSTGRES_DSN`, so this command did not actually enable the PostgreSQL integration tests. It was followed by the repository-correct DSN command above, which passed.

### Default localhost:5432 credentials

Command:

~~~bash
EX_MARKETING_DATABASE_URL='postgres://postgres:postgres@127.0.0.1:5432/exmarketing_test?sslmode=disable' go test -count=1 ./internal/postgres -run 'TestPostgres' -v
~~~

Failure log summary:

~~~text
failed to connect to `user=postgres database=exmarketing_test`: 127.0.0.1:5432 failed SASL auth:
FATAL: password authentication failed for user "postgres" (SQLSTATE 28P01)
~~~

Failure reason: local default PostgreSQL credentials on port `5432` are not valid for this workspace. This was an environment credential failure, not a product test assertion failure.

Reproduction:

~~~bash
EX_MARKETING_DATABASE_URL='postgres://postgres:postgres@127.0.0.1:5432/exmarketing_test?sslmode=disable' go test -count=1 ./internal/postgres -run 'TestPostgres' -v
~~~

Resolution used for validation: rerun with the repository-documented compose DSN:

~~~bash
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test -count=1 ./internal/postgres -run 'TestPostgres' -v
~~~

需求：2026-05-24-113400-4
