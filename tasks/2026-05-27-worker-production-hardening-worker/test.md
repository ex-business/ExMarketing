# Test

## Scope

根据 `task.md`、`analysis.md`、`review.md`、`test-plan.md`、`implementation.md` 执行回归测试。

本轮严格遵守 Test Agent 约束：只写本文件，未修改业务代码、测试代码、文档代码、配置文件或其他文件。

## Environment

- 工作目录：`/Users/tong/Documents/GitHub/OracleBetX-Projects/ExMarketing`
- PostgreSQL：`docker compose ps postgres` 显示 `exmarketing-postgres` 为 `Up 3 days (healthy)`
- PostgreSQL URL：`postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable`
- 本轮未执行 `docker compose down` / teardown，避免停止用户已有本地服务。

## Commands

~~~bash
docker compose ps postgres
~~~

结果：PASS。

摘要：`exmarketing-postgres` 状态为 `Up 3 days (healthy)`，端口 `54329->5432`。

~~~bash
go test ./internal/worker -count=1
~~~

结果：PASS。

摘要：worker 单元测试通过，覆盖 HTTP / WEBHOOK / MQ 投递、retry / DLQ、单条失败不中断、常驻轮询、shutdown drain、停止后不 claim 新消息、stale 恢复、DLQ 统计、claim delay 和结构化日志脱敏。

~~~bash
go test ./... -count=1
~~~

结果：PASS。

摘要：全量 Go 测试通过；本命令未设置 `EX_MARKETING_DATABASE_URL`，PostgreSQL 集成测试按现有测试逻辑跳过。

~~~bash
go build ./...
~~~

结果：PASS。

~~~bash
EX_MARKETING_OUTBOX_WORKER=true go test ./... -run 'Outbox|Worker|DLQ|Quota|Idempot' -count=1 -v
~~~

结果：PASS。

摘要：

- worker outbox / retry / DLQ / shutdown / stale / structured log 用例通过。
- quota、idempotency、callback、DLQ 管理相关过滤用例通过。
- 该命令未设置 `EX_MARKETING_DATABASE_URL`，`internal/postgres` 过滤命中的 PostgreSQL 用例为 SKIP；真实 PostgreSQL 已由后续命令覆盖。

~~~bash
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./internal/postgres -run 'TestPostgresOutbox|TestPostgresCloseOutboxDLQOnlyClosesOutbox' -count=1 -v
~~~

结果：PASS。

通过用例：

- `TestPostgresCloseOutboxDLQOnlyClosesOutbox`
- `TestPostgresOutboxRetryEligibilityAndTerminalStatusNotClaimed`
- `TestPostgresOutboxConcurrentClaimSingleMessage`
- `TestPostgresOutboxConcurrentClaimMultipleMessages`
- `TestPostgresOutboxStaleRecoveryConcurrentClaim`
- `TestPostgresOutboxDLQCountByTenantAndChannel`

~~~bash
EX_MARKETING_OUTBOX_WORKER=true EX_MARKETING_REPOSITORY=postgres EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./... -run 'Postgres|Outbox|Worker|Claim|Smoke' -count=1 -v
~~~

结果：PASS。

摘要：

- `internal/postgres` 真实库执行并通过 19 个过滤用例，包括 outbox retry/terminal、并发 claim、stale recovery、DLQ count、quota/idempotency/callback/concurrent。
- `internal/worker` 执行并通过 21 个 outbox worker 用例。

~~~bash
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go run ./cmd/engine-e2e-postgres
~~~

结果：PASS。

输出摘要：

~~~text
PASS runId=e2e_pg_20260529_090246_000000000 tenantId=site_001_e2e_pg_20260529_090246_000000000 activityId=act_c90446407d8db9f3386fad47 activityCode=e2e_pg_20260529_090246_000000000 successCommand=cmd_2724361434e9a02d6794192e failedCommand=cmd_c1a8db4a3be5a6200d47863c
~~~

该命令回归了上一轮 `POST_EVENT_SUCCESS_CASE` 因历史 `site_001` active 活动污染导致的 `QUOTA_EXCEEDED` 失败；本轮已通过，tenant 随 runId 隔离生效。

## Test Cases

| ID | Result | Evidence |
| --- | --- | --- |
| TP-001 Worker 开关关闭不处理 | STATIC PASS | `cmd/exmarketing/main.go` 仅在 `env("EX_MARKETING_OUTBOX_WORKER", "false") == "true"` 时创建并启动 worker；本轮未启动完整服务做运行时黑盒验证。 |
| TP-002 常驻处理第一批和后续新增 outbox | PASS | `TestOutboxWorkerRunContinuesAfterEmptyBatch`。 |
| TP-003 额度不足不创建错误发奖链路 | PASS | `TestPostgresQuotaInsufficientBudgetHasNoDirtyWrites`、`TestSimulateEventRewardConditionFailureDoesNotPreviewGrantOrOutbox`。 |
| TP-004 幂等重复不重复扣减和不重复 outbox | PASS | `TestPostgresCreateInboxDetectsDuplicateAndConflict`、`TestPostgresParticipationAndGrantIdempotencyUniqueConstraints`。 |
| TP-005 单条失败不导致 worker 退出 | PASS | `TestOutboxWorkerSingleFailureDoesNotStopNextMessage`。 |
| TP-006 Retry 到期边界 | PASS | `TestPostgresOutboxRetryEligibilityAndTerminalStatusNotClaimed`。 |
| TP-007 终态不参与普通 claim | PASS | `TestPostgresOutboxRetryEligibilityAndTerminalStatusNotClaimed` 覆盖 `SENT` / `DLQ` / `DLQ_CLOSED`。 |
| TP-008 Shutdown drain 停止后不再 claim 新消息 | PASS | `TestOutboxWorkerShutdownDrainsInFlight`、`TestOutboxWorkerShutdownStopsClaimingNewMessages`。 |
| TP-009 Shutdown drain timeout 后 stale 恢复 | PARTIAL PASS | `OutboxWorker.Run` 中存在 drain timeout 后 `cancelProcess()` 逻辑；`TestOutboxWorkerRecoversStaleClaimsAndSnapshotsDLQ` 和 `TestPostgresOutboxStaleRecoveryConcurrentClaim` 覆盖 stale 恢复。未发现真实 drain timeout 退出后重启恢复的端到端自动化用例。 |
| TP-010 PostgreSQL 并发 claim 同一 outbox 只能一个 worker 成功 | PASS | `TestPostgresOutboxConcurrentClaimSingleMessage`。 |
| TP-011 PostgreSQL 多 worker 多记录并发吞吐 | PASS | `TestPostgresOutboxConcurrentClaimMultipleMessages`。 |
| TP-012 Stale 恢复与正常 claim 并发 | PASS | `TestPostgresOutboxStaleRecoveryConcurrentClaim`。 |
| TP-013 DLQ 堆积观测 | PASS | `TestPostgresOutboxDLQCountByTenantAndChannel`、`TestOutboxWorkerRecoversStaleClaimsAndSnapshotsDLQ`。 |
| TP-014 Claim 延迟观测 | PASS | `TestOutboxWorkerRecoversStaleClaimsAndSnapshotsDLQ`。 |
| TP-015 结构化日志字段和敏感信息脱敏 | PASS | `TestOutboxWorkerStructuredLogsRedactSensitiveFields`。 |
| TP-016 PostgreSQL smoke 端到端最小生产路径 | PASS | `go run ./cmd/engine-e2e-postgres` 通过，输出包含本轮隔离 tenant。 |

## Result

~~~text
PASS
~~~

## Notes

- 本轮所有执行命令均通过，无失败命令需要打回 Dev Agent。
- 剩余覆盖备注：TP-001 只做静态验证，TP-009 未发现“drain timeout 退出后重启恢复”的完整端到端自动化用例；现有单元/集成测试已分别覆盖 drain 完成、停止后不 claim、stale 恢复和 PostgreSQL 并发恢复。

需求：2026-05-27-worker-production-hardening-worker
