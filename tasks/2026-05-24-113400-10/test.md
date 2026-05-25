# Test

## Agent 指令

你是 Test Agent。请根据 task.md、analysis.md、review.md、test-plan.md 和 implementation.md 执行完整测试。

最低要求：

~~~bash
go test ./...
go build ./...
~~~

## Test Cases

- 基础 Go 单元/包测试：覆盖活动、事件处理、审计查询、模拟运行、额度不足、幂等、回调、DLQ、内存仓储并发和 workflow 相关包。
- 构建验证：覆盖所有 Go packages 可编译。
- PostgreSQL smoke：启动仓库提供的 `postgres` compose 服务，设置 `EX_MARKETING_DATABASE_URL` 后执行 PostgreSQL 集成测试。
- 审计与对账相关用例已由现有测试覆盖：
  - `TestAuditQueryFiltersAndSnapshots`
  - `TestSimulateEventReturnsRuleExplainWithoutPersistence`
  - `TestSimulateEventReturnsActivityAndQuotaPreviewsWithoutPersistence`
  - `TestActivityEventCreatesGrantCommand`
  - `TestActivityEventRuleRejectionReturnsExplainWithoutGrant`
  - `TestActivityEventBudgetExceededReturnsConflictAndAudit`
  - `TestActivityEventUserLimitExceededReturnsConflictAndAudit`
  - `TestActivityEventStockExceededReturnsConflictAndNoRewardableResult`
  - `TestActivityEventDuplicateAndConflict`
  - `TestActivityEventRejectsMismatchedIdempotencyKey`
  - `TestRewardReconciliationSupportsActivitySubjectAndEventQueries`
  - `TestRewardCallbackSuccessConfirmsQuotaAndIsIdempotent`
  - `TestRewardCallbackFailedReleasesQuotaAndRejectsConflictAndUnknown`
  - `TestOutboxDLQQueryReplayAndClose`
  - `TestOutboxDLQOperationRequiresOperatorAndReason`
  - `TestMemoryConcurrentDuplicateGrantRequestOnlyProcessesOnce`
  - `TestMemoryConcurrentQuotaCompetitionDoesNotOverReserve`
  - `TestPostgresAtomicGrantRollsBackOnStockExceeded`
  - `TestPostgresCreateInboxDetectsDuplicateAndConflict`
  - `TestPostgresParticipationAndGrantIdempotencyUniqueConstraints`
  - `TestPostgresAtomicGrantRollsBackWhenGrantOrderCreateFails`
  - `TestPostgresAtomicGrantRollsBackWhenOutboxCreateFails`

## Commands

~~~bash
go test ./...
go build ./...
docker compose up -d postgres
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./internal/postgres -run 'Postgres|Audit|Reconcile|Idempot|Quota|DLQ' -count=1
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./... -count=1
go test ./internal/httpapi -list 'Audit|Reconciliation|Simulate|Quota|DLQ|Callback|Event|Tenant'
go test ./internal/marketing -list 'Audit|Reconciliation|Quota|Idempot|Concurrent|Duplicate'
go test ./internal/worker -list 'DLQ|Replay|Close|Outbox|Audit'
go test ./internal/postgres -list 'Postgres|Audit|Reconcile|Idempot|Quota|DLQ'
~~~

## Result

~~~text
PASS
~~~

## Evidence

- `go test ./...`：PASS。所有 packages 通过；`internal/postgres` 在未设置 `EX_MARKETING_DATABASE_URL` 时跳过数据库集成用例。
- `go build ./...`：PASS。无编译错误。
- `docker compose up -d postgres`：PASS。`exmarketing-postgres` 容器处于 `Running`。
- `EX_MARKETING_DATABASE_URL=... go test ./internal/postgres -run 'Postgres|Audit|Reconcile|Idempot|Quota|DLQ' -count=1`：PASS，`ok github.com/ex-stack/ExMarketing/internal/postgres 0.443s`。
- `EX_MARKETING_DATABASE_URL=... go test ./... -count=1`：PASS。数据库环境下全量测试通过，包含 PostgreSQL 集成用例。
- 测试列表命令确认了审计筛选与快照、模拟运行无持久化、事件成功/拒绝/额度不足/幂等冲突、对账查询、回调幂等与冲突、DLQ replay/close、并发额度竞争、PostgreSQL 事务回滚和幂等唯一约束等覆盖点。
- 未发现失败命令，无需打回 Dev Agent。

需求：2026-05-24-113400-10
