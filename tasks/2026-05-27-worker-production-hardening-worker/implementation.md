# Implementation

## Summary

已按 `task.md` / `analysis.md` / `review.md` 的生产化口径完成 Worker 最小加固。

- Outbox worker 默认常驻轮询，保留 `EX_MARKETING_OUTBOX_WORKER=true` 才启动的既有入口。
- 增加 shutdown drain：收到停止信号后停止新 claim，等待当前处理中 outbox 写回；默认 drain timeout 为 30 秒，超时后取消处理上下文，后续由 stale claim 恢复兜底。
- 增加 stale `SENDING` 恢复：默认阈值 5 分钟，将超时 `SENDING` 恢复为 `PENDING`，不增加 retry，不重建 participation / grant_order / outbox。
- 增加可测试统计快照：in-flight、claim 尝试/成功/空批/冲突、投递成功/失败/DLQ、stale 恢复、claim delay、DLQ tenant/channel 堆积。
- 增加结构化日志字段：worker_id、event、tenant_id、outbox_id、target_channel、target_ref 摘要、message_type、状态流转、retry_count、next_retry_at、claim_delay_ms、delivery_duration_ms、error_class、error_message。

本轮返工根据 `test.md` 的 FAIL 摘要补齐强制测试覆盖缺口，失败原因不是既有命令失败，而是 PostgreSQL worker production smoke 缺少可执行用例。已补充最小测试并修复一个脱敏边界：

- PostgreSQL outbox retry eligibility：未来 `next_retry_at` 不 claim，到期 retry 可 claim。
- PostgreSQL terminal status：`SENT` / `DLQ` / `DLQ_CLOSED` 不参与普通 claim，且终态 `updated_at` 不被普通扫描改动。
- PostgreSQL 多 worker 并发 claim：同一 outbox 只能被 claim 一次；多 outbox 并发 claim 无重复。
- PostgreSQL stale recovery 并发：同一 stale `SENDING` 只有一个恢复成功，恢复后可再次普通 claim。
- PostgreSQL DLQ 堆积聚合：按 tenant / channel 统计 `DLQ`，排除 `DLQ_CLOSED`。
- Worker 单元测试补齐：单条失败不导致后续消息停止、停止后不再 claim 新消息、结构化日志字段完整且敏感错误脱敏。
- 脱敏修复：`sanitizeError` 增加 `Cookie` 关键字脱敏。

本轮再次返工根据 `test.md` 最新 FAIL 摘要处理 PostgreSQL E2E smoke 污染：

- `cmd/engine-e2e-postgres` 原先每次运行使用唯一 activity code，但 tenant 固定为 `site_001`，subject 固定为 `success_user` / `failed_user`。
- 本地 PostgreSQL 中历史 `site_001` 下的 `e2e_pg_%` active 活动仍会匹配新事件，导致历史 user quota 抢先返回 `QUOTA_EXCEEDED`。
- 已将 E2E runner 的 `tenantID` 改为基于本轮 `runID` 生成，保持 activity、event、quota、outbox、callback、reconciliation 全链路仍在真实 PostgreSQL 执行，同时避免被历史 active 活动污染。

## Changed Files

- `internal/worker/outbox_worker.go`
  - 增加 worker 生命周期、drain、stale 恢复、DLQ 堆积采样和 `Snapshot()`。
  - 保留现有 HTTP / WEBHOOK / MQ 投递、retry、DLQ 语义。
  - 返工补充 `Cookie` 类敏感错误脱敏。
- `internal/worker/outbox_worker_test.go`
  - 覆盖空批后继续常驻处理新增 outbox、shutdown drain、stale 恢复、claim delay 和 DLQ 统计快照。
  - 返工补充单条失败不中断、停止后不 claim 新消息、结构化日志字段与脱敏断言。
- `cmd/exmarketing/main.go`
  - 主进程收到停止信号后等待 worker drain 完成，再结束进程并关闭 repository。
- `internal/marketing/model.go`
  - 增加 `OutboxDLQCount` 统计结构。
- `internal/marketing/memory_repository.go`
  - 增加测试/本地可用的 stale 恢复和 DLQ tenant/channel 统计。
- `internal/postgres/repository.go`
  - 增加 PostgreSQL stale `SENDING` 恢复，使用状态条件和 `FOR UPDATE SKIP LOCKED`。
  - 增加 PostgreSQL DLQ tenant/channel 聚合统计。
- `internal/postgres/repository_integration_test.go`
  - 返工补充 PostgreSQL outbox retry / terminal / concurrent claim / stale recovery / DLQ count 集成测试。
- `cmd/engine-e2e-postgres/main.go`
  - 返工修复 PostgreSQL E2E smoke 数据隔离：每次 run 使用 `site_001_<runID>` tenant，避免历史 `site_001` active 活动和固定 subject quota 污染重复运行。

## Behavior Changes

- Worker 处理完一批或空批后不会退出，会继续下一轮扫描。
- 停止时已开始处理的消息优先完成投递和状态写回；超时退出时不会删除 outbox，后续超过 stale 阈值可恢复处理。
- `SENDING` 且 `updated_at` 早于 stale 阈值的 outbox 会恢复为 `PENDING`，原 payload、target、command id 和 retry_count 保持不变。
- `DLQ` 和 `DLQ_CLOSED` 仍不参与普通 claim；DLQ replay / close 入口不变。

## Verification

~~~bash
go test ./...
go build ./...
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go run ./cmd/engine-e2e-postgres
~~~

结果：三条命令均已通过。E2E 输出示例：

~~~text
PASS runId=e2e_pg_20260529_090054_000000000 tenantId=site_001_e2e_pg_20260529_090054_000000000 activityId=act_83f1b9aa20976b23907a7e13 activityCode=e2e_pg_20260529_090054_000000000 successCommand=cmd_8743c6b1117630ac77b5e853 failedCommand=cmd_97bd327a2897ef090bfb7599
~~~

返工后额外执行：

~~~bash
go test ./internal/worker -count=1
EX_MARKETING_REPOSITORY=postgres EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./internal/postgres -run 'PostgresOutbox|PostgresCloseOutboxDLQOnlyClosesOutbox' -count=1 -v
EX_MARKETING_OUTBOX_WORKER=true EX_MARKETING_REPOSITORY=postgres EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./... -run 'Postgres|Outbox|Worker|Claim|Smoke' -count=1 -v
~~~

结果：全部通过。过滤测试已执行新增的 `TestPostgresOutboxRetryEligibilityAndTerminalStatusNotClaimed`、`TestPostgresOutboxConcurrentClaimSingleMessage`、`TestPostgresOutboxConcurrentClaimMultipleMessages`、`TestPostgresOutboxStaleRecoveryConcurrentClaim`、`TestPostgresOutboxDLQCountByTenantAndChannel`，以及新增 worker 行为 / 日志用例。

## Notes

- `test.md` 已提供 FAIL 原因，本轮已按最新失败摘要修复 E2E run 隔离问题。
- `commit.md` 仍为模板态，未提供实际提交失败上下文。
- 当前工作区存在其他任务文件的既有未提交修改，本次未回退或改动那些无关文件。

需求：2026-05-27-worker-production-hardening-worker
