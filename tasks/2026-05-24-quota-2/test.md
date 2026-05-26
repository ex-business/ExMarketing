# Test

需求：2026-05-24-quota-2

## Scope

根据 `task.md`、`analysis.md`、`review.md`、`test-plan.md`、`implementation.md` 执行测试。

本轮只写入本文件，未修改业务代码、测试代码、文档代码、配置文件或其他文件。

## Test Cases

- 全量 Go 单元 / 集成测试：覆盖当前仓库所有 Go package。
- 全量 Go 编译：确认当前实现可编译。
- PostgreSQL 仓储定向测试：覆盖 quota 原子回滚、quota 不足无脏写、callback 确认/释放/幂等/冲突、callback 租户隔离、并发预算不超支、并发库存最后额度。
- PostgreSQL 环境全量测试：使用 `EX_MARKETING_DATABASE_URL` 跑 `go test ./... -count=1`。
- PostgreSQL 环境全量编译：使用 `EX_MARKETING_DATABASE_URL` 跑 `go build ./...`。
- Timeout release 定向测试：覆盖内存仓储 dry-run / 实际释放 / 重复幂等，以及 HTTP Admin endpoint dry-run / 实际释放 / audit。
- PostgreSQL smoke SQL：抽查 quota 聚合不超额、不为负，reservation 无重复键、无孤儿记录，并查看 reservation 状态分布。

## Commands

```bash
go test ./...
```

结果：PASS。

```bash
go build ./...
```

结果：PASS。

```bash
docker compose up -d postgres
```

结果：PASS，`exmarketing-postgres` 已在运行。

```bash
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./internal/postgres -run 'TestPostgres.*Quota|TestPostgres.*Reservation|TestPostgres.*Callback|TestPostgres.*Timeout|TestPostgres.*Concurrent|TestPostgresAtomic' -count=1 -v
```

结果：PASS。实际命中并通过：

- `TestPostgresAtomicGrantRollsBackOnStockExceeded`
- `TestPostgresAtomicGrantRollsBackWhenGrantOrderCreateFails`
- `TestPostgresAtomicGrantRollsBackWhenOutboxCreateFails`
- `TestPostgresQuotaInsufficientBudgetHasNoDirtyWrites`
- `TestPostgresCallbackConfirmReleaseIdempotencyAndConflict`
- `TestPostgresCallbackTenantIsolation`
- `TestPostgresConcurrentBudgetDoesNotOverspend`
- `TestPostgresConcurrentStockLastItem`

```bash
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./... -count=1
```

结果：PASS。

```bash
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go build ./...
```

结果：PASS。

```bash
go test ./internal/marketing ./internal/httpapi -run 'TestMemoryTimeoutReleaseDryRunReleaseAndIdempotency|TestQuotaTimeoutReleaseEndpointDryRunReleaseAndAudit' -count=1 -v
```

结果：PASS。

## PostgreSQL Smoke SQL

宿主机执行测试计划中的 `psql ...` 命令失败，原因是本机没有安装或没有暴露 `psql`：

```text
zsh:1: command not found: psql
```

已改用 PostgreSQL 容器内 `psql` 执行等价 SQL：

```bash
docker compose exec -T postgres psql -U exmarketing -d exmarketing -c "SELECT COUNT(*) AS invalid_quota_count FROM marketing_quota WHERE used_value + frozen_value > total_value OR used_value < 0 OR frozen_value < 0;"
```

结果：PASS，`invalid_quota_count = 0`。

```bash
docker compose exec -T postgres psql -U exmarketing -d exmarketing -c "SELECT tenant_id, grant_order_id, quota_type, scope, scope_key, COUNT(*) FROM marketing_quota_reservation GROUP BY tenant_id, grant_order_id, quota_type, scope, scope_key HAVING COUNT(*) > 1;"
```

结果：PASS，返回 `0 rows`。

```bash
docker compose exec -T postgres psql -U exmarketing -d exmarketing -c "SELECT COUNT(*) AS orphan_reservation_count FROM marketing_quota_reservation r LEFT JOIN marketing_grant_order g ON g.id = r.grant_order_id WHERE g.id IS NULL;"
```

结果：PASS，`orphan_reservation_count = 0`。

```bash
docker compose exec -T postgres psql -U exmarketing -d exmarketing -c "SELECT reservation_status, COUNT(*), COALESCE(SUM(reserved_amount), 0) FROM marketing_quota_reservation GROUP BY reservation_status ORDER BY reservation_status;"
```

结果：PASS，当前测试库返回 `0 rows`。

## Evidence

- `go test ./...`：全部 package 通过。
- `go build ./...`：通过。
- PostgreSQL 定向测试：通过，覆盖原子回滚、quota 不足、callback 幂等/冲突/租户隔离、并发预算和库存。
- PostgreSQL 全量 `go test ./... -count=1`：全部 package 通过。
- PostgreSQL 全量 `go build ./...`：通过。
- Timeout release 定向测试：内存仓储和 HTTP endpoint 均通过。
- Smoke SQL：quota 聚合无超额/负数，reservation 无重复键、无孤儿记录。

## Notes

- 本轮未执行 `docker compose down`，因为测试开始时 `exmarketing-postgres` 已处于 Running 状态，避免关闭用户已有本地服务。
- 当前自动化中未发现 PostgreSQL timeout release 专项测试命名被定向正则命中；timeout release 已由内存仓储和 HTTP endpoint 测试覆盖，PostgreSQL 仓储仍通过带数据库 URL 的全量测试。

## Result

```text
PASS
```
