# Test Plan

## Agent 指令

你是 Test Plan Agent。请在 Review PASS 后、Dev 开发前，根据 task.md、analysis.md、review.md 设计测试用例。

只写 test-plan.md，不改业务代码，不执行提交。

## Test Strategy

- 覆盖 grant order 状态机合法流转：`PENDING -> SENDING`、`SENDING -> RETRY`、`SENDING/RETRY -> SUCCESS/FAILED`。
- 覆盖非法 callback：`PENDING -> SUCCESS/FAILED`、终态反向更新、`CLOSED` 后 callback。
- 覆盖重复 callback 幂等：相同 `SUCCESS + externalGrantId` 返回 duplicate，不重复确认 quota。
- 覆盖重复 `SUCCESS` callback 但 `externalGrantId` 不同的冲突，返回 `CALLBACK_STATUS_CONFLICT` 或等价机器可识别错误。
- 覆盖 outbox worker 投递前推进 grant order 到 `SENDING`，投递失败后进入 `RETRY` 并递增 `retry_count`。
- 覆盖 callback 与 quota reservation 联动：`SUCCESS` 确认冻结额度，`FAILED` 释放冻结额度。
- 覆盖 tenant 隔离：callback 只能按 `tenantId + commandId` 命中本 tenant grant order。

## Test Cases

- `TestRewardCallbackSuccessConfirmsQuotaAndIsIdempotent`
  - 先将 grant order 推进到 `SENDING`。
  - 首次 `SUCCESS` callback 进入 `SUCCESS`，participation 进入 `GRANTED`，quota reservation 进入 `CONFIRMED`。
  - 相同 `externalGrantId` 重复 `SUCCESS` callback 返回 duplicate。
  - 不同 `externalGrantId` 重复 `SUCCESS` callback 返回冲突。
- `TestRewardCallbackFailedReleasesQuotaAndRejectsConflictAndUnknown`
  - 先将 grant order 推进到 `SENDING`。
  - `FAILED` callback 进入 `FAILED`，participation 进入 `FAILED`，quota reservation 进入 `RELEASED`。
  - 终态后相反 `SUCCESS` callback 返回冲突。
  - 未知 `commandId` 返回 `GRANT_ORDER_NOT_FOUND`。
- `TestRewardCallbackRejectsPendingGrantOrder`
  - 新建后仍处于 `PENDING` 的 grant order 收到 callback 时返回 `CALLBACK_STATUS_CONFLICT`，不得落终态。
- `TestOutboxWorkerAdvancesGrantOrderState`
  - outbox worker 发送前将 grant order 从 `PENDING` 推进到 `SENDING`。
  - HTTP 可重试失败后 grant order 进入 `RETRY`，`retry_count` 递增。
- 既有重复事件 / 并发幂等测试
  - 同一 participation 的同一 reward 不重复创建 grant order / outbox。

## Required Commands

~~~bash
go test ./...
go build ./...
~~~

## Data / Smoke

- 使用内存仓储覆盖 HTTP handler、callback、quota reservation、outbox worker 的快速单元测试。
- PostgreSQL 集成测试继续覆盖唯一约束、事务回滚、quota reservation 与 outbox 基础路径。

## Decision

~~~text
PASS
~~~

需求：2026-05-24-grant-order
