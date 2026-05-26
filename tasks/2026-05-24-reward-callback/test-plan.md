# Test Plan

## Agent 指令

你是 Test Plan Agent。请在 Review PASS 后、Dev 开发前，根据 task.md、analysis.md、review.md 设计测试用例。

只写 test-plan.md，不改业务代码，不执行提交。

## Test Strategy

- HTTP 层覆盖 callback JSON、签名、错误码和响应字段，确保失败请求没有业务副作用。
- 业务层覆盖 `SENDING` / `RETRY` grant order 的首次成功、首次失败、重复回调、相反终态、外部流水冲突和 pending 拒绝。
- 定位层覆盖 `commandId`、`billNo`、`bill_no`、`idempotencyKey`、`idempotency_key` 兼容语义，以及多 locator 不一致时返回 `CALLBACK_LOCATOR_CONFLICT`。
- Quota 层覆盖有 reservation 时成功确认、失败释放，不限额 grant order 无 reservation 仍可终结，应有 reservation 缺失时返回 `QUOTA_RESERVATION_MISSING`。
- 租户隔离层覆盖相同单号在不同 tenant 下只能更新请求 tenant 的 grant order。
- 持久化层覆盖内存仓储和 PostgreSQL 仓储的一致语义；PostgreSQL 集成测试在无数据库时按现有 skip 规则跳过。

## Test Cases

- 成功回调：合法签名，按 `commandId` / `billNo` 定位 `SENDING` grant order，返回 200，`grantStatus/status=SUCCESS`，保存 `externalGrantId`，quota reservation 变为 `CONFIRMED`。
- 失败回调：合法签名，按 locator 定位 `SENDING` grant order，返回 200，`grantStatus/status=FAILED`，quota reservation 变为 `RELEASED`。
- 重复成功：相同 `SUCCESS + externalGrantId` 返回 `duplicate=true`，quota 不重复移动。
- 重复失败：相同 `FAILED` 返回 `duplicate=true`，quota 不重复移动。
- 相反终态：`SUCCESS` 后 `FAILED`、`FAILED` 后 `SUCCESS` 返回 `CALLBACK_STATUS_CONFLICT`。
- 外部流水冲突：已成功单收到不同 `externalGrantId` 的成功回调返回冲突。
- 未知单号：未知 `tenantId + billNo/commandId/idempotencyKey` 返回 `GRANT_ORDER_NOT_FOUND`。
- 签名失败：缺少签名返回 `CALLBACK_SIGNATURE_MISSING`，错误签名返回 `CALLBACK_SIGNATURE_INVALID`，过期时间戳返回 `CALLBACK_TIMESTAMP_INVALID`，均不更新业务状态。
- 定位冲突：`billNo` 与 `idempotencyKey` 指向不同 grant order，或同一请求的 locator 与命中单不一致，返回 `CALLBACK_LOCATOR_CONFLICT`。
- 非法请求：缺 `tenantId`、缺 locator、非法 `status`、`SUCCESS` 缺 `externalGrantId`、非法 JSON / Content-Type 返回 `INVALID_CALLBACK_REQUEST`。
- 不限额兼容：活动从未配置 quota 时，无 reservation 的 grant order 可成功终结。
- reservation 缺失异常：活动已有 quota 但 grant order 无 reservation 时返回 `QUOTA_RESERVATION_MISSING`。
- 租户隔离：同一 `billNo` 或相同形态 locator 不跨 tenant 更新。

## Required Commands

~~~bash
go test ./...
go build ./...
~~~

## Data / Smoke

- 使用 handler 测试构造真实活动、quota、事件、grant order，并通过 HTTP callback 验证响应体。
- 使用内存仓储测试 reservation、quota 聚合、participation 推进和不限额兼容。
- 使用 PostgreSQL 集成测试验证事务、锁定、幂等重复和 tenant 隔离；无 `EX_MARKETING_DATABASE_URL` 时跳过。

## Decision

~~~text
PASS
~~~

需求：2026-05-24-reward-callback
