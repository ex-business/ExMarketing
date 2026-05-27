# Test

## Agent 指令

你是 Test Agent。已根据 task.md、analysis.md、review.md、test-plan.md 和 implementation.md 执行测试。

本轮只写入本文件；未修改业务代码、测试代码、文档代码、配置文件或其他文件。

## Test Cases

- 基础全量测试：覆盖项目现有所有 Go package 测试。
- 基础全量构建：验证所有 Go package 可编译。
- 对账专项测试：覆盖 `GET /admin/v1/reward/reconciliation` 的 activity / subject / event / idempotencyKey 查询、租户隔离、缺少定位条件校验、`participationId` 查询 rejected participation、`outboxId` 回溯 grant/participation 链路，以及 diagnostics 字段。
- 关键词专项测试：执行 `Reconciliation|Postgres|PostgreSQL` 相关用例。
- Race 专项测试：执行 `Reconciliation|Concurrent|Idempotent` 相关用例。
- PostgreSQL 集成测试检查：确认真实 PostgreSQL 用例存在环境门禁；当前环境未设置 `EX_MARKETING_DATABASE_URL` / `POSTGRES_DSN`，相关真实数据库用例按测试代码 `t.Skip` 跳过。

## Commands

~~~bash
go test ./...
go build ./...
go test ./... -run 'Reconciliation|Postgres|PostgreSQL' -count=1
go test ./... -race -run 'Reconciliation|Concurrent|Idempotent' -count=1
go test ./internal/httpapi -run TestRewardReconciliation -count=1 -v
go test ./internal/postgres -run 'TestPostgres.*Reconciliation|TestPostgres' -count=1 -v
printenv EX_MARKETING_DATABASE_URL POSTGRES_DSN
~~~

## Result

~~~text
PASS
~~~

## Evidence

- `go test ./...`：通过。
- `go build ./...`：通过。
- `go test ./... -run 'Reconciliation|Postgres|PostgreSQL' -count=1`：通过；`internal/httpapi` 对账相关测试执行通过，其他无匹配测试的 package 显示 `[no tests to run]`。
- `go test ./... -race -run 'Reconciliation|Concurrent|Idempotent' -count=1`：通过。
- `go test ./internal/httpapi -run TestRewardReconciliation -count=1 -v`：
  - `TestRewardReconciliationSupportsActivitySubjectAndEventQueries` PASS。
  - `TestRewardReconciliationValidatesQueryAndSupportsPartialLinks` PASS。
- `go test ./internal/postgres -run 'TestPostgres.*Reconciliation|TestPostgres' -count=1 -v`：命令通过；真实 PostgreSQL 集成用例因 `EX_MARKETING_DATABASE_URL is not set` 全部 SKIP。
- `printenv EX_MARKETING_DATABASE_URL POSTGRES_DSN`：无输出，退出码 1，确认当前 shell 未提供真实 PostgreSQL DSN。

未执行手工 Admin API smoke：当前任务未提供运行中的 `ADMIN_BASE_URL`、`ADMIN_TOKEN`、租户和测试数据 ID；本轮已用 `httptest` 覆盖对账 API 行为。

需求：2026-05-24-113929-10
