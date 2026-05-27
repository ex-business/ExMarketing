# Test

## Agent 指令

你是 Test Agent。请根据 task.md、analysis.md、review.md、test-plan.md 和 implementation.md 执行完整测试。

最低要求：

~~~bash
go test ./...
go build ./...
~~~

## Test Cases

- 首存插件上下文：验证 `FIRST_DEPOSIT` 插件从标准 `DEPOSIT_SUCCESS` 事件构建 `state.plugin.FIRST_DEPOSIT.*`，包含 amount、currency、isFirstDeposit、paymentMethod、depositBizId。
- 插件预检查拒绝：覆盖 eventType/bizType 不匹配、缺少金额、金额非正数、缺少币种、首存标识缺失/非法、非首存、depositNo 与 bizId 冲突。
- 活动插件链路：验证 `FIRST_DEPOSIT` 默认注册、规则/奖励可读取插件上下文、grant/outbox 仍由核心流程生成、模拟运行不持久化、未知插件/不兼容活动类型被拒绝。
- Review P2 风险：验证 `USER_LIMIT=1` 等 quota/idempotency/concurrent 相关命名用例通过，PostgreSQL smoke 覆盖重复/冲突、quota 不足无脏写、并发预算/库存不超扣。
- 兼容边界：通过全仓测试确认未绑定插件的既有 `attributes.*` 活动链路未被破坏。

## Commands

~~~bash
go test ./internal/plugin ./internal/engine ./internal/httpapi ./internal/marketing ./internal/postgres -run 'Test.*FirstDeposit|Test.*Plugin|Test.*Quota|Test.*Idempot|Test.*Concurrent|TestPostgres.*' -count=1
go test ./...
go build ./...
docker compose up -d postgres
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./internal/postgres ./internal/httpapi ./internal/engine -run 'TestPostgres|Test.*FirstDeposit|Test.*Quota|Test.*Idempot|Test.*Concurrent' -count=1
go test ./internal/plugin ./internal/engine -run 'TestFirstDeposit|Test.*Plugin' -count=1 -v
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./internal/postgres -run 'TestPostgres' -count=1 -v
docker compose exec -T postgres pg_isready -U exmarketing -d exmarketing
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./... -count=1
~~~

## Result

~~~text
PASS
~~~

## Evidence

- Targeted command passed:
  - `internal/plugin`
  - `internal/engine`
  - `internal/httpapi`
  - `internal/marketing`
  - `internal/postgres`
- Required baseline passed:
  - `go test ./...`
  - `go build ./...`
- PostgreSQL smoke passed:
  - `docker compose up -d postgres` returned `Container exmarketing-postgres Running`.
  - `pg_isready` returned `/var/run/postgresql:5432 - accepting connections`.
  - PostgreSQL URL smoke for `internal/postgres`, `internal/httpapi`, `internal/engine` passed.
  - Full repository test with `EX_MARKETING_DATABASE_URL` passed.
- Key verbose PASS evidence:
  - `TestFirstDepositPluginBuildsContextFromStandardEvent`
  - `TestFirstDepositPluginPrecheckRejectsInvalidInputs`
  - `TestActivityPluginContextFeedsRulesRewardsAndSnapshot`
  - `TestSimulateEventIncludesPluginPreviewWithoutPersistence`
  - `TestFirstDepositPluginIsRegisteredByDefaultAndFeedsRulesRewards`
  - `TestFirstDepositPluginPrecheckRejectsBeforeGrant`
  - `TestFirstDepositPluginRejectsIncompatibleActivityTypeAtCreate`
  - `TestPostgresCreateInboxDetectsDuplicateAndConflict`
  - `TestPostgresQuotaInsufficientBudgetHasNoDirtyWrites`
  - `TestPostgresConcurrentBudgetDoesNotOverspend`
  - `TestPostgresConcurrentStockLastItem`

## Failure Notes

无失败命令，无需打回 Dev Agent。

需求：2026-05-24-113929-11
