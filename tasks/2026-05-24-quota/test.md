# Test

## Agent 指令

Test Agent 按 `task.md`、`analysis.md`、`review.md`、`test-plan.md`、`implementation.md` 执行测试。

本轮只写入本文件，未修改业务代码、测试代码、文档代码、配置文件或其他文件。

## Environment

- CWD: `/Users/tong/Documents/GitHub/OracleBetX-Projects/ExMarketing`
- Date: 2026-05-26
- PostgreSQL: Docker compose service `postgres`, container `exmarketing-postgres`
- PostgreSQL DSN: `postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable`

## Commands

### 1. 测试清单核对

```bash
go test ./internal/postgres -list 'Test.*'
```

结果：PASS

摘要：已确认 PostgreSQL 包包含本任务要求的 quota、callback、tenant isolation、concurrent 用例：

```text
TestPostgresQuotaInsufficientBudgetHasNoDirtyWrites
TestPostgresCallbackConfirmReleaseIdempotencyAndConflict
TestPostgresCallbackTenantIsolation
TestPostgresConcurrentBudgetDoesNotOverspend
TestPostgresConcurrentStockLastItem
```

同时保留原有 atomic rollback / inbox / participation 用例。

```bash
go test ./internal/marketing -list 'Test.*(Quota|Callback|Concurrent|Atomic|Reservation)'
go test ./internal/httpapi -list 'Test.*(Quota|Callback|Simulate)'
```

结果：PASS

摘要：memory 层包含 quota reservation、callback、concurrent、atomic 测试；HTTP 层包含 simulate 和 reward callback 测试。

### 2. 全量 Go 测试

```bash
go test ./...
```

结果：PASS

摘要：

```text
?    github.com/ex-stack/ExMarketing/cmd/exmarketing [no test files]
?    github.com/ex-stack/ExMarketing/cmd/workflowctl [no test files]
ok   github.com/ex-stack/ExMarketing/internal/decimal (cached)
ok   github.com/ex-stack/ExMarketing/internal/engine (cached)
ok   github.com/ex-stack/ExMarketing/internal/fact (cached)
ok   github.com/ex-stack/ExMarketing/internal/httpapi 0.515s
ok   github.com/ex-stack/ExMarketing/internal/marketing (cached)
ok   github.com/ex-stack/ExMarketing/internal/mqconsumer (cached)
?    github.com/ex-stack/ExMarketing/internal/plugin [no test files]
ok   github.com/ex-stack/ExMarketing/internal/postgres (cached)
ok   github.com/ex-stack/ExMarketing/internal/reward (cached)
ok   github.com/ex-stack/ExMarketing/internal/rule (cached)
ok   github.com/ex-stack/ExMarketing/internal/worker (cached)
ok   github.com/ex-stack/ExMarketing/internal/workflow 0.469s
```

### 3. 启动 PostgreSQL

```bash
docker compose up -d postgres
```

结果：PASS

摘要：

```text
Container exmarketing-postgres Running
```

### 4. Test Plan 指定 PostgreSQL 目标正则

```bash
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./internal/postgres -run 'TestPostgres.*Quota|TestPostgres.*Callback|TestPostgres.*Concurrent|TestPostgresAtomic' -count=1 -v
```

结果：PASS

执行并通过的用例：

```text
=== RUN   TestPostgresAtomicGrantRollsBackOnStockExceeded
--- PASS: TestPostgresAtomicGrantRollsBackOnStockExceeded (0.04s)
=== RUN   TestPostgresAtomicGrantRollsBackWhenGrantOrderCreateFails
--- PASS: TestPostgresAtomicGrantRollsBackWhenGrantOrderCreateFails (0.02s)
=== RUN   TestPostgresAtomicGrantRollsBackWhenOutboxCreateFails
--- PASS: TestPostgresAtomicGrantRollsBackWhenOutboxCreateFails (0.02s)
=== RUN   TestPostgresQuotaInsufficientBudgetHasNoDirtyWrites
--- PASS: TestPostgresQuotaInsufficientBudgetHasNoDirtyWrites (0.01s)
=== RUN   TestPostgresCallbackConfirmReleaseIdempotencyAndConflict
--- PASS: TestPostgresCallbackConfirmReleaseIdempotencyAndConflict (0.04s)
=== RUN   TestPostgresCallbackTenantIsolation
--- PASS: TestPostgresCallbackTenantIsolation (0.02s)
=== RUN   TestPostgresConcurrentBudgetDoesNotOverspend
--- PASS: TestPostgresConcurrentBudgetDoesNotOverspend (0.07s)
=== RUN   TestPostgresConcurrentStockLastItem
--- PASS: TestPostgresConcurrentStockLastItem (0.03s)
PASS
ok   github.com/ex-stack/ExMarketing/internal/postgres 0.488s
```

### 5. PostgreSQL DSN 下全量 Go 测试

```bash
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./... -count=1
```

结果：PASS

摘要：

```text
?    github.com/ex-stack/ExMarketing/cmd/exmarketing [no test files]
?    github.com/ex-stack/ExMarketing/cmd/workflowctl [no test files]
ok   github.com/ex-stack/ExMarketing/internal/decimal 0.171s
ok   github.com/ex-stack/ExMarketing/internal/engine 0.359s
ok   github.com/ex-stack/ExMarketing/internal/fact 0.194s
ok   github.com/ex-stack/ExMarketing/internal/httpapi 0.943s
ok   github.com/ex-stack/ExMarketing/internal/marketing 0.452s
ok   github.com/ex-stack/ExMarketing/internal/mqconsumer 0.620s
?    github.com/ex-stack/ExMarketing/internal/plugin [no test files]
ok   github.com/ex-stack/ExMarketing/internal/postgres 1.463s
ok   github.com/ex-stack/ExMarketing/internal/reward 1.028s
ok   github.com/ex-stack/ExMarketing/internal/rule 0.893s
ok   github.com/ex-stack/ExMarketing/internal/worker 1.257s
ok   github.com/ex-stack/ExMarketing/internal/workflow 1.642s
```

### 6. 构建

```bash
go build ./...
```

结果：PASS

### 7. SQL 对账抽查

本轮使用 Docker PostgreSQL 容器内 `psql` 执行 SQL smoke，避免依赖本机 `psql`。

```bash
docker compose exec -T postgres psql -U exmarketing -d exmarketing -c "SELECT COUNT(*) AS over_quota_count FROM marketing_quota WHERE used_value + frozen_value > total_value;"
```

结果：PASS

```text
over_quota_count
----------------
0
```

```bash
docker compose exec -T postgres psql -U exmarketing -d exmarketing -c "SELECT tenant_id, grant_order_id, quota_type, scope, scope_key, COUNT(*) FROM marketing_quota_reservation GROUP BY tenant_id, grant_order_id, quota_type, scope, scope_key HAVING COUNT(*) > 1;"
```

结果：PASS

```text
(0 rows)
```

```bash
docker compose exec -T postgres psql -U exmarketing -d exmarketing -c "SELECT COUNT(*) AS orphan_reservation_count FROM marketing_quota_reservation r LEFT JOIN marketing_grant_order g ON g.id = r.grant_order_id WHERE g.id IS NULL;"
```

结果：PASS

```text
orphan_reservation_count
------------------------
0
```

## Test Cases

| Area | Result | Evidence |
| --- | --- | --- |
| Freeze / confirm / release callback | PASS | `TestPostgresCallbackConfirmReleaseIdempotencyAndConflict` plus HTTP callback tests. |
| Quota insufficient no dirty writes | PASS | `TestPostgresQuotaInsufficientBudgetHasNoDirtyWrites` and PostgreSQL atomic rollback tests. |
| Idempotent duplicate event | PASS | PostgreSQL inbox duplicate/conflict test and memory duplicate grant test. |
| Idempotent duplicate callback | PASS | `TestPostgresCallbackConfirmReleaseIdempotencyAndConflict` and HTTP callback idempotency tests. |
| Callback tenant isolation | PASS | `TestPostgresCallbackTenantIsolation`. |
| Multi-quota atomicity / rollback | PASS | PostgreSQL atomic stock exceeded, grant_order failure, outbox failure tests; memory multi-reward stock rollback test. |
| PostgreSQL concurrency budget / stock | PASS | `TestPostgresConcurrentBudgetDoesNotOverspend` and `TestPostgresConcurrentStockLastItem`. |
| Simulation no side effects | PASS | HTTP simulate tests pass in full suite. |
| Decimal boundary / decimal handling | PASS | Decimal package tests pass; quota tests use decimal amounts under full suite. |
| SQL smoke | PASS | over-quota count 0, duplicate reservation rows 0, orphan reservation count 0. |

## Result

```text
PASS
```

理由：`test-plan.md` 要求的基础全量测试、PostgreSQL 指定正则集成测试、PostgreSQL DSN 全量测试、构建和 SQL smoke 均已通过；本轮确认 PostgreSQL 并发、callback、租户隔离和 quota no-dirty-write 覆盖已存在并被实际执行。
