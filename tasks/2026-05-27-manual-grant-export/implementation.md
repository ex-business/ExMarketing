# Implementation

## Scope

按 `task.md`、`analysis.md`、`review.md`、`test-plan.md` 和本轮 `test.md` 失败反馈做最小返工。

`review.md` Decision 为 `PASS`，`test-plan.md` Decision 为 `PASS`。本轮优先处理 `test.md` 的 `FAIL`：PostgreSQL 聚焦测试未命中 `Manual` / `Export` 持久化链路，不能满足 PostgreSQL smoke 验收。

## Changes

- 在 `internal/postgres/repository.go` 增加 `marketing_manual_grant_export_file` 和 `marketing_manual_grant_export_detail` 的轻量建表逻辑，并在 `NewRepository` 初始化时确保表和查询索引存在。
- 在 `internal/postgres/repository_integration_test.go` 新增 `TestPostgresManualExportReadySuccessPersistsTraceAuditAndQuota`：
  - `MANUAL_EXPORT` 单据在 READY 导出前确认 `SUCCESS` 返回 `ErrManualGrantExportRequired`。
  - 创建 READY 导出后写入导出文件、明细、checksum，并把 grant order 推进到 `SENDING`。
  - READY 文件内 `SENDING` 单据确认 `SUCCESS` 后，grant order、participation、reward-level quota、participation-level quota 均落库为预期状态。
  - 通过 PostgreSQL 查询验证导出追踪和 service 审计记录。
- 在 `internal/postgres/repository_integration_test.go` 新增 `TestPostgresManualExportConcurrentUniqueInclusionAndFailedHistoryReexport`：
  - 并发导出同一批 `PENDING` 单据时只有一个请求成功纳入，另一个返回无候选。
  - 未失败导出明细中同一 grant order 不重复出现。
  - `FAILED` 导出历史保留后不阻断同一 grant order 重新导出，重导后进入 READY / `SENDING`。
- 扩展 PostgreSQL 测试清理逻辑，覆盖 manual export 文件、明细和相关 audit 记录，避免测试数据污染。

## Verification

已执行：

~~~bash
go test ./internal/postgres -list 'TestPostgres.*Manual|TestPostgres.*Export|TestPostgres.*Grant|TestPostgres.*Quota|TestPostgres.*Concurrent'
~~~

结果：PASS，列表已包含：

~~~text
TestPostgresManualExportReadySuccessPersistsTraceAuditAndQuota
TestPostgresManualExportConcurrentUniqueInclusionAndFailedHistoryReexport
~~~

已执行：

~~~bash
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./internal/postgres -run 'TestPostgres.*Manual|TestPostgres.*Export|TestPostgres.*Grant|TestPostgres.*Quota|TestPostgres.*Concurrent' -count=1
go test ./...
go build ./...
docker compose up -d postgres
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./...
~~~

结果均为 PASS。

## Result

~~~text
PASS
~~~

需求：2026-05-27-manual-grant-export
