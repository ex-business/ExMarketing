# Test

## Agent 指令

你是 Test Agent。请根据 task.md、analysis.md、review.md、test-plan.md 和 implementation.md 执行完整测试。

只允许写本文件 `tasks/2026-05-29-admin-ops-pages-dlq/test.md`。
禁止修改业务代码、测试代码、文档代码、配置文件或任何其他文件。

## Test Scope

- 全量 Go 回归。
- 构建验证。
- HTTP Admin 定向 smoke，覆盖审计过滤、对账查询、DLQ replay/close、operator/reason 必填、人工发奖 decision 和手工导出审计 action。
- PostgreSQL 迁移与集成 smoke，覆盖 DLQ、manual export、tenant isolation 和返工过的审计 action。
- 带 `EX_MARKETING_DATABASE_URL` 的全量 Go 回归，避免 Postgres 集成测试被默认跳过。

## Environment

- 工作目录：`/Users/tong/Documents/GitHub/OracleBetX-Projects/ExMarketing`
- Go：`go1.25.4`
- Postgres 容器：`exmarketing-postgres`，端口 `54329 -> 5432`
- 数据库连接串：

~~~bash
postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable
~~~

## Commands

~~~bash
go test ./...
go build ./...
go test ./internal/httpapi -run 'Test(AuditQueryFiltersAndSnapshots|AuditQueryTimeTraceAndRequestFilters|RewardReconciliationSupportsActivitySubjectAndEventQueries|RewardReconciliationValidatesQueryAndSupportsPartialLinks|OutboxDLQQueryReplayAndClose|OutboxDLQOperationRequiresOperatorAndReason|ManualGrantDecisionAndExportWriteStableAuditActions)$'
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go run ./cmd/db-migration-release --mode apply
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./internal/postgres -run 'TestPostgres(CloseOutboxDLQOnlyClosesOutbox|OutboxDLQCountByTenantAndChannel|ManualExportReadySuccessPersistsTraceAuditAndQuota|ManualExportConcurrentUniqueInclusionAndFailedHistoryReexport|CallbackTenantIsolation)$' -count=1
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./internal/postgres -run 'TestPostgresManualExportReadySuccessPersistsTraceAuditAndQuota$' -count=1 -v
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./...
~~~

## Result

~~~text
PASS
~~~

## Evidence

通过项：

- `go test ./...` 通过。
  - 注意：未设置 `EX_MARKETING_DATABASE_URL` 时，Postgres 集成测试按代码逻辑默认跳过。
- `go build ./...` 通过。
- HTTP Admin 定向 smoke 通过：
  - `TestAuditQueryFiltersAndSnapshots`
  - `TestAuditQueryTimeTraceAndRequestFilters`
  - `TestRewardReconciliationSupportsActivitySubjectAndEventQueries`
  - `TestRewardReconciliationValidatesQueryAndSupportsPartialLinks`
  - `TestOutboxDLQQueryReplayAndClose`
  - `TestOutboxDLQOperationRequiresOperatorAndReason`
  - `TestManualGrantDecisionAndExportWriteStableAuditActions`
- 本地 Postgres 迁移通过：
  - `APPLY PASS`
  - `applied=3 pending=0`
  - `001_marketing_core.sql success`
  - `002_audit_fact_outbox_indexes.sql success`
  - `003_activity_unique_constraint.sql success`
- PostgreSQL 定向 smoke 通过：
  - `TestPostgresCloseOutboxDLQOnlyClosesOutbox`
  - `TestPostgresOutboxDLQCountByTenantAndChannel`
  - `TestPostgresManualExportReadySuccessPersistsTraceAuditAndQuota`
  - `TestPostgresManualExportConcurrentUniqueInclusionAndFailedHistoryReexport`
  - `TestPostgresCallbackTenantIsolation`
- 返工验证命令通过：
  - `TestPostgresManualExportReadySuccessPersistsTraceAuditAndQuota`
  - 日志摘要：`--- PASS: TestPostgresManualExportReadySuccessPersistsTraceAuditAndQuota`
- 带 Postgres 的全量回归通过：
  - `EX_MARKETING_DATABASE_URL=... go test ./...`
  - `internal/postgres` 本轮实际执行并通过。

## Notes

- 上一版 `test.md` 记录的失败点是 `TestPostgresManualExportReadySuccessPersistsTraceAuditAndQuota` 仍断言旧审计 action `MANUAL_GRANT_EXPORT_READY` / `MANUAL_GRANT_CONFIRM_SUCCESS`。本轮复测该测试已通过，说明 implementation.md 记录的返工修复已生效。
- 未发现现成 `scripts/admin-ops-smoke.sh` 或其他 admin ops smoke 脚本；本轮未新增临时脚本，遵守只写 `test.md` 的限制。
- `psql` 不在当前环境 PATH 中，未执行 test-plan.md 建议的手工 SQL 只读 smoke。等价覆盖由 Postgres 迁移、定向集成测试和带数据库连接的全量 Go 回归完成。
- 本轮未修改业务代码、测试代码、文档代码、配置文件或其他文件。

需求：2026-05-29-admin-ops-pages-dlq
