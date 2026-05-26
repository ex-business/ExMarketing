# Test

## Agent 指令

你是 Test Agent。请根据 task.md、analysis.md、review.md、test-plan.md 和 implementation.md 执行完整测试。

最低要求：

~~~bash
go test ./...
go build ./...
~~~

## Test Cases

- `TestRewardCallbackSuccessConfirmsQuotaAndIsIdempotent`
  - 覆盖 `SENDING -> SUCCESS` callback、quota reservation `CONFIRMED`、重复相同 `externalGrantId` 幂等、重复不同 `externalGrantId` 冲突。
- `TestRewardCallbackFailedReleasesQuotaAndRejectsConflictAndUnknown`
  - 覆盖 `SENDING -> FAILED` callback、quota reservation `RELEASED`、终态相反 callback 冲突、未知 `commandId` 返回 not found。
- `TestRewardCallbackRejectsPendingGrantOrder`
  - 覆盖 `PENDING` grant order 收到 callback 时返回状态冲突，不允许直接落终态。
- `TestOutboxWorkerAdvancesGrantOrderState`
  - 覆盖 outbox worker 发送前推进 grant order 到 `SENDING`，投递失败后进入 `RETRY` 并累计 `retry_count`。
- `go test ./...`
  - 覆盖全仓 Go 单元/集成测试，包括既有重复事件、并发幂等、PostgreSQL 仓储、quota/outbox 基础路径。

## Commands

~~~bash
go test ./...
go build ./...
go test ./internal/httpapi ./internal/worker ./internal/marketing ./internal/postgres
go test ./internal/httpapi ./internal/worker -run 'TestRewardCallbackSuccessConfirmsQuotaAndIsIdempotent|TestRewardCallbackFailedReleasesQuotaAndRejectsConflictAndUnknown|TestRewardCallbackRejectsPendingGrantOrder|TestOutboxWorkerAdvancesGrantOrderState' -count=1 -v
~~~

## Result

~~~text
PASS
~~~

## Evidence

- `go test ./...` 通过。
  - `internal/httpapi` 通过，包含 reward callback handler 测试。
  - `internal/worker` 通过，包含 outbox worker 状态推进测试。
  - `internal/marketing` 通过，包含内存仓储幂等/事务测试。
  - `internal/postgres` 通过，包含 PostgreSQL 仓储集成路径测试。
- `go build ./...` 通过，无构建错误。
- 定向测试通过日志摘要：

~~~text
=== RUN   TestRewardCallbackSuccessConfirmsQuotaAndIsIdempotent
--- PASS: TestRewardCallbackSuccessConfirmsQuotaAndIsIdempotent (0.00s)
=== RUN   TestRewardCallbackFailedReleasesQuotaAndRejectsConflictAndUnknown
--- PASS: TestRewardCallbackFailedReleasesQuotaAndRejectsConflictAndUnknown (0.00s)
=== RUN   TestRewardCallbackRejectsPendingGrantOrder
--- PASS: TestRewardCallbackRejectsPendingGrantOrder (0.00s)
PASS
ok   github.com/ex-stack/ExMarketing/internal/httpapi
=== RUN   TestOutboxWorkerAdvancesGrantOrderState
--- PASS: TestOutboxWorkerAdvancesGrantOrderState (0.00s)
PASS
ok   github.com/ex-stack/ExMarketing/internal/worker
~~~

未发现测试失败，无需打回 Dev Agent。

需求：2026-05-24-grant-order
