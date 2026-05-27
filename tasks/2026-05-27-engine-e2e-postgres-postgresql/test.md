# Test

## Agent 指令

你是 Test Agent。已根据 `task.md`、`analysis.md`、`review.md`、`test-plan.md` 和 `implementation.md` 执行测试。

本轮只写入本文件，未修改业务代码、测试代码、文档代码、配置文件或其他文件。

## Environment

- Date: 2026-05-27
- Workdir: `/Users/tong/Documents/GitHub/OracleBetX-Projects/ExMarketing`
- PostgreSQL container: `exmarketing-postgres`
- PostgreSQL DSN: `postgres://exmarketing:***@localhost:54329/exmarketing?sslmode=disable`
- E2E command from Dev: `EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go run ./cmd/engine-e2e-postgres`
- Outbox worker: E2E runner output reported `enabled`

## Test Cases

| Case | Result | Notes |
| --- | --- | --- |
| Static check: Dev documented command | PASS | `implementation.md` documents `go run ./cmd/engine-e2e-postgres`. |
| Required gate: `go test ./...` | PASS | All Go tests passed. |
| Required gate: `go build ./...` | PASS | Build completed with exit code 0. |
| TC-08 negative: missing `EX_MARKETING_DATABASE_URL` | PASS | Command failed with explicit in-memory repository rejection. |
| TC-08 negative: invalid PostgreSQL DSN | PASS | Command failed with PostgreSQL connection refused. |
| TC-01 / TC-03 / TC-05 / TC-09 PostgreSQL E2E | FAIL | Correct DSN run failed at `POST_EVENT_SUCCESS_CASE`; repeat run failed the same way. |
| TC-02 / TC-04 / TC-06 / TC-07 / TC-10 | NOT RUN | Blocked by primary PostgreSQL E2E failure before successful chain could be established. |

## Commands

```bash
go test ./...
```

Result: PASS.

```bash
go build ./...
```

Result: PASS.

```bash
go run ./cmd/engine-e2e-postgres
```

Result: PASS for negative check. Exit code 1 with expected error:

```text
FAIL: EX_MARKETING_DATABASE_URL is required; PostgreSQL E2E cannot run against the in-memory repository
```

```bash
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54330/exmarketing?sslmode=disable' go run ./cmd/engine-e2e-postgres
```

Result: PASS for negative check. Exit code 1 with expected connection refused error for port `54330`.

```bash
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go run ./cmd/engine-e2e-postgres
```

Result: FAIL. First failed run:

```text
stage: POST_EVENT_SUCCESS_CASE
runId: e2e_pg_20260527_151510_000000000
HTTP: POST /openapi/v1/events
status: 409
response: {"code":"QUOTA_EXCEEDED","error":"quota exceeded: USER_LIMIT: USER: success_user","quotaType":"USER_LIMIT","scope":"USER","scopeKey":"success_user"}
```

Repeat run:

```bash
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go run ./cmd/engine-e2e-postgres
```

Result: FAIL. Repeat failure:

```text
stage: POST_EVENT_SUCCESS_CASE
runId: e2e_pg_20260527_151556_000000000
HTTP: POST /openapi/v1/events
status: 409
response: {"code":"QUOTA_EXCEEDED","error":"quota exceeded: USER_LIMIT: USER: success_user","quotaType":"USER_LIMIT","scope":"USER","scopeKey":"success_user"}
```

## Failure Log Summary

The PostgreSQL E2E runner creates a new activity and quota rows per run, but it uses fixed subject IDs:

```text
success_user
failed_user
```

The database already contained an earlier active E2E activity:

```text
e2e_pg_20260527_151210_000000000 | ACTIVE
```

That earlier run has exhausted `success_user` quota:

```text
activity_code                     | quota_type  | scope | scope_key                | total_value | used_value | frozen_value
e2e_pg_20260527_151210_000000000 | USER_LIMIT  | USER  | success_user             | 1.0000      | 1.0000     | 0.0000
e2e_pg_20260527_151210_000000000 | DAILY_LIMIT | USER  | success_user:2026-05-27 | 1.0000      | 1.0000     | 0.0000
```

After failed reruns, PostgreSQL contained multiple active E2E activities:

```text
e2e_pg_20260527_151556_000000000 | ACTIVE
e2e_pg_20260527_151510_000000000 | ACTIVE
e2e_pg_20260527_151210_000000000 | ACTIVE
```

The later runs created new quota rows with `used_value = 0`, but the event still failed with `QUOTA_EXCEEDED` for `USER_LIMIT: USER: success_user`. This indicates repeat execution is not isolated from previously active E2E activities and fixed subject IDs.

The runner's own failure snapshot also has diagnostic query defects:

```text
activity: expected 2 arguments, got 3
quota: expected 1 arguments, got 3
participation: expected 1 arguments, got 3
grant_order: expected 1 arguments, got 3
reservation: expected 1 arguments, got 3
inbox: ERROR: could not determine data type of parameter $1
outbox: ERROR: could not determine data type of parameter $1
audit: ERROR: column "payload" does not exist
```

This means failure output does not satisfy the requirement to include usable activity / quota / inbox / participation / grant_order / outbox / reservation / audit database snapshots.

## Failure Reason

Result is `FAIL`.

Primary failure: `go run ./cmd/engine-e2e-postgres` is not repeatable against the same PostgreSQL database without manual cleanup. Historical E2E activities remain `ACTIVE`, and the runner uses fixed `success_user` / `failed_user` subject IDs with `USER_LIMIT` and `DAILY_LIMIT` quotas of `1`, so later runs can be rejected before the success branch creates the expected grant/outbox chain.

Secondary failure: the failure diagnostics contain broken SQL snapshot queries, so the command does not provide the required database state summary when it fails.

## Reproduction

Use a PostgreSQL database that already has one successful E2E run, then run:

```bash
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go run ./cmd/engine-e2e-postgres
```

Expected by task/test-plan: command exits 0 with `PASS`, and can be run repeatedly.

Actual: command exits 1 at `POST_EVENT_SUCCESS_CASE` with HTTP 409:

```text
quota exceeded: USER_LIMIT: USER: success_user
```

需求：2026-05-27-engine-e2e-postgres-postgresql
