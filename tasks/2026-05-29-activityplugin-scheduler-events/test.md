# Test

## Agent

Test Agent

## Scope

按 `task.md`、`analysis.md`、`review.md`、`test-plan.md`、`implementation.md` 执行测试。

本轮只写入本文件；未修改业务代码、测试代码、文档代码或配置文件。

## Environment

- Repository: `/Users/tong/Documents/GitHub/OracleBetX-Projects/ExMarketing`
- Date: 2026-05-29
- Go: `go1.25.4`
- Global `EX_MARKETING_DATABASE_URL`: 未读取到全局设置
- PostgreSQL smoke DSN: 命令内显式使用 `postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable`

## Commands

```bash
go test ./internal/... -run 'Test.*Plugin.*Schedule|Test.*Schedule.*Plugin|Test.*Scheduler|Test.*Idempot|TestMemoryPluginSchedule'
```

Result: PASS

摘要：

```text
ok github.com/ex-stack/ExMarketing/internal/plugin (cached)
ok github.com/ex-stack/ExMarketing/internal/postgres 1.921s
all internal packages passed or had no matching tests
```

```bash
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./internal/postgres -run 'TestPostgres.*Schedule|TestPostgresCreateInboxDetectsDuplicateAndConflict' -count=1 -v
```

Result: PASS

摘要：

```text
=== RUN   TestPostgresCreateInboxDetectsDuplicateAndConflict
--- PASS: TestPostgresCreateInboxDetectsDuplicateAndConflict (0.04s)
=== RUN   TestPostgresPluginScheduleEventInboxSmoke
--- PASS: TestPostgresPluginScheduleEventInboxSmoke (0.02s)
PASS
ok github.com/ex-stack/ExMarketing/internal/postgres 0.273s
```

```bash
go test ./...
```

Result: PASS

摘要：

```text
ok github.com/ex-stack/ExMarketing/internal/httpapi 0.389s
ok github.com/ex-stack/ExMarketing/internal/workflow 0.485s
all packages passed or had no test files
```

```bash
go build ./...
```

Result: PASS

摘要：

```text
command exited with code 0
```

## Test Case Evidence

- TC-01 normal START path: covered by `TestPluginSchedulerRegistersExecutesAndPreventsRepeatSuccess`; verifies registration, execution success, Event Inbox write, `occurredAt=scheduledAt`, stable attributes, query and audit behavior.
- TC-02 END / DRAW / SETTLE / COMPENSATE mapping: covered by `TestPluginSchedulerMapsAllJobTypesAndAllowsCompensateNoEvent`; verifies all job type mappings and COMPENSATE no-event success audit.
- TC-03 capability and binding validation: covered by `TestPluginSchedulerRejectsInvalidPluginRegistrationClasses` and `TestPluginSchedulerRequiresEventHandleCapabilityAndCloseAudit`; verifies plugin not found, disabled, unsupported activity type, missing `EVENT_HANDLE`, and unbound plugin rejection paths.
- TC-04 duplicate registration idempotency: covered by `TestPluginSchedulerRegistersExecutesAndPreventsRepeatSuccess`.
- TC-05 duplicate execution same payload: covered by `TestPluginSchedulerRegistersExecutesAndPreventsRepeatSuccess` and `TestPluginSchedulerGeneratedStandardEventPayloadIsStable`; repeated terminal execution does not re-run the plugin and generated standard payload remains stable.
- TC-06 idempotency conflict: covered by `TestPluginSchedulerMarksInboxConflictNonRetryable`.
- TC-07 stable payload after activity config changes: covered by job snapshot fields and `TestPluginSchedulerGeneratedStandardEventPayloadIsStable`; payload includes stable `activityVersion` / `activityConfigHash` from the job snapshot and uses `scheduledAt`, not executor `now`.
- TC-08 retryable failures and manual close: covered by `TestPluginSchedulerRetriesRetryableFailureThenSucceeds` and `TestPluginSchedulerRequiresEventHandleCapabilityAndCloseAudit`.
- TC-09 concurrent claim single job: covered by `TestMemoryPluginScheduleRepositoryClaimOnlyOnceConcurrently`, using concurrent goroutines and verifying only one claim succeeds.
- TC-10 running job recovery: covered by `TestPluginSchedulerRecoversOnlyStaleRunningJobs`; verifies stale `RUNNING` recovery to `RETRY` and terminal jobs are not recovered.
- TC-11 insufficient quota / budget through standard chain: scheduler boundary smoke verified by code/test inspection: scheduler only writes through `CreateInbox` and has no `grant_order`, `outbox`, quota reservation, or external reward dependency. Full `go test ./...` passed existing engine/reward boundary tests.
- TC-12 audit and sensitive data: covered by success/retry/conflict/close audit assertions and `TestPluginSchedulerRedactsSensitiveRetryableFailure`.
- TC-13 PostgreSQL repository smoke: covered by `TestPostgresPluginScheduleEventInboxSmoke` and `TestPostgresCreateInboxDetectsDuplicateAndConflict` against real PostgreSQL using the command-local DSN above.

## Result

```text
PASS
```

## Decision

Test-plan required commands passed:

- Focused scheduler/idempotency tests passed.
- Real PostgreSQL smoke passed and included scheduler-generated Event Inbox duplicate/conflict semantics.
- `go test ./...` passed.
- `go build ./...` passed.

No failing command was observed in this Test Agent run.

需求：2026-05-29-activityplugin-scheduler-events
