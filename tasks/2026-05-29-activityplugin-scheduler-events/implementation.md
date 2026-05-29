# Implementation

## Agent 指令

你是 Dev Agent。只有 review.md 为 PASS 且 test-plan.md 为 PASS 时才能开发或修复。

必须按 task.md、analysis.md、review.md、test-plan.md 执行，不得扩大范围。
如果 test.md 已存在且 Result 为 FAIL，必须优先读取失败原因，做最小修复后再更新 implementation.md。
如果 commit.md 或提交日志存在 BLOCK / FAILED / 提交失败，必须优先读取提交失败上下文，做最小修复后再更新 implementation.md。

完成开发后必须先做基础验证：

~~~bash
go test ./...
go build ./...
~~~

## Summary

- Added a minimal ActivityPlugin scheduler implementation in `internal/plugin`.
- Scheduler jobs support `START` / `END` / `DRAW` / `SETTLE` / `COMPENSATE`, status transitions, retry state, manual close, audit records, duplicate registration idempotency, and atomic claim semantics.
- Plugin-generated schedule events are converted to stable standard `marketing.ActivityEvent` payloads and written through the existing Event Inbox repository boundary.
- Review P2 was addressed by storing `activityVersion` and `activityConfigHash` on the job snapshot and by generating standard payload fields from the job snapshot; default `occurredAt` is `job.scheduledAt`, not executor `now`.
- Test返工：补齐 scheduler 自动化覆盖，包括全 jobType 映射、`COMPENSATE` 无事件成功审计、插件未注册/禁用/活动类型不支持/未绑定拒绝、活动配置变化后 payload 仍使用 job snapshot、20 goroutine 并发 claim、RUNNING 超时恢复、敏感错误脱敏。
- Test返工：修复 PostgreSQL 集成测试从包目录运行时找不到 `db/migration` 的问题，migration manager 现在会从当前目录向上查找仓库根目录的默认迁移目录。

## Changed Files

- `internal/plugin/scheduler.go`
  - New `PluginScheduleJob` model, job type/status constants, scheduler executor, Event Inbox mapping, retry/close operations, and in-memory schedule repository.
  - New `ScheduledActivityPlugin` interface for plugins that can execute schedule jobs.
  - Added `RecoverRunningJobs` / `RecoverRunning` to move stale `RUNNING` jobs back to `RETRY` with audit evidence, without touching terminal `SUCCESS` / `CLOSED` jobs.
- `internal/plugin/scheduler_test.go`
  - Covers duplicate registration, successful execution, repeated terminal execution, stable standard payload, retryable plugin failure, Event Inbox idempotency conflict, missing `EVENT_HANDLE`, manual close audit, full jobType event mapping, no-event compensation audit, invalid plugin registration classes, sensitive error redaction, concurrent single-claim behavior, and stale running recovery.
- `internal/dbmigration/migration.go`
  - Default migration directory resolution now works when PostgreSQL tests run from package working directories.
- `internal/postgres/repository_integration_test.go`
  - Added `TestPostgresPluginScheduleEventInboxSmoke`, which verifies a scheduler-generated standard event uses stable `occurredAt=scheduledAt` and real PostgreSQL Event Inbox duplicate/conflict semantics.
- `docs/activityplugin-lifecycle.md`
  - Documents scheduler capability reuse, job state model, snapshot stability, Event Inbox mapping, and no-direct-grant boundary.
- `docs/data-model.md`
  - Documents ActivityPlugin scheduler event Inbox semantics and minimal `PluginScheduleJob` fields.

## Behavior Changes

- Schedule registration requires the activity-bound plugin to be registered, enabled, compatible with the activity type, and declare `EVENT_HANDLE`.
- Duplicate registration for the same `tenantId + activityId + pluginCode + jobType + scheduledAt` returns the existing job.
- A due `PENDING` or eligible `RETRY` job can be claimed once and executed by one worker.
- Stale `RUNNING` jobs can be recovered to `RETRY` by compensation scan, while terminal jobs remain unchanged.
- `SUCCESS` and `CLOSED` are terminal; repeated execution of a successful job does not call the plugin again.
- Event Inbox duplicate with the same payload is treated as success; same key with different payload marks the job `FAILED` with `IDEMPOTENCY_CONFLICT` and no automatic retry.
- Retryable plugin and Inbox failures move to `RETRY` until the default retry schedule is exhausted.

## Notes

- No PostgreSQL scheduler table or distributed scheduler integration was added; this is the requested minimal implementation with an in-memory schedule repository.
- Scheduler code does not create `grant_order`, `outbox`, quota reservations, or call external reward systems.
- Validation commands run:
  - `go test ./internal/plugin -run 'Test.*Plugin.*Schedule|Test.*Schedule.*Plugin|Test.*Scheduler|Test.*Idempot|TestMemoryPluginSchedule'` PASS
  - `EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./internal/postgres -run 'TestPostgres.*Schedule|TestPostgresCreateInboxDetectsDuplicateAndConflict' -count=1 -v` PASS
  - `go test ./...` PASS
  - `go build ./...` PASS

需求：2026-05-29-activityplugin-scheduler-events
