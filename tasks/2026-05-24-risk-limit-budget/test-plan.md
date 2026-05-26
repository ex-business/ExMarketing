# Test Plan

## Agent 指令

你是 Test Plan Agent。请在 Review PASS 后、Dev 开发前，根据 task.md、analysis.md、review.md 设计测试用例。

只写 test-plan.md，不改业务代码，不执行提交。

## Test Strategy

- 以单元测试覆盖引擎层 RISK/LIMIT/BUDGET 行为边界，重点验证失败时不生成 grant_order/outbox，且 participation/audit 具备可排查原因。
- 以 repository 原子命令测试覆盖 quota 并发安全、重复幂等和多 reward 回滚。
- 以 HTTP handler 测试覆盖 OpenAPI 对外错误结构：HTTP 409、`code=QUOTA_EXCEEDED`、`quotaType/scope/scopeKey`。
- `DAILY_LIMIT` 必须验证活动 timezone 自然日 scopeKey，而不是服务器本地日期。
- 互斥只验证同一事件候选活动裁剪，不验证历史 participation 活动期互斥。

## Test Cases

1. RISK 条件拒绝：配置 RISK CONDITION 命中黑名单/高风险字段，事件处理结果为 `REJECTED`，不生成 grant_order/outbox，不扣 quota。
2. LIMIT 条件拒绝：配置 LIMIT CONDITION 不通过，事件处理结果为 `REJECTED`，不生成 grant_order/outbox，不扣 quota。
3. USER_LIMIT 不足：`USER_LIMIT + USER + subject.id` quota 不足时返回 `409 QUOTA_EXCEEDED`，错误体和 audit 包含 `quotaType=USER_LIMIT`、`scope=USER`、`scopeKey=subject.id`。
4. DAILY_LIMIT 不足：按活动 timezone 计算 `subject.id:YYYY-MM-DD`，同一活动自然日第二次发奖返回 `409 QUOTA_EXCEEDED`，错误体包含 `quotaType=DAILY_LIMIT`。
5. 缺少 USER_LIMIT / DAILY_LIMIT quota 行：按无限制处理，不能因为 quota 行缺失拒绝旧活动。
6. BUDGET 不足：预算不足返回 `409 QUOTA_EXCEEDED`，不保留 `REWARDABLE` participation，不生成 grant_order/outbox，写 `QUOTA_EXCEEDED` audit。
7. STOCK 不足：任一 reward 库存不足时整活动失败，多个 reward 不允许部分发奖或部分扣减。
8. 非金额 reward：`reward_amount=0.0000` 但数量有效时仍生成 grant_order/outbox，并扣 STOCK、USER_LIMIT、DAILY_LIMIT。
9. 零金额无实际 reward：不生成 grant_order/outbox，不扣 BUDGET/STOCK/USER_LIMIT/DAILY_LIMIT。
10. 同一事件互斥：同租户、同 subject、同非空 exclusiveGroup 且存在 `stackable=false` 时，只保留稳定排序第一活动；落选活动写 `EXCLUSIVE_GROUP_LOST` participation 或等价审计解释，不扣 quota。
11. 跨事件非互斥：用户先参加活动 A，再通过新事件命中同 exclusiveGroup 活动 B，不得因历史 participation 自动拒绝 B。
12. 幂等重复事件：完全重复事件返回 duplicate，不重复创建 participation/grant_order/outbox，不重复扣 quota。
13. 幂等冲突：同 Idempotency-Key 但 payload 不一致返回 `INVALID_IDEMPOTENCY_KEY`，不得进入发奖链路。
14. 模拟运行：只输出 RISK/LIMIT/BUDGET 和 quota 风险预览，不产生 inbox、participation、grant_order、outbox 或 quota 扣减副作用。

## Required Commands

~~~bash
go test ./...
go build ./...
~~~

## Data / Smoke

- 使用内存仓储构造活动、规则、quota 和标准事件，覆盖 repository 原子扣减与 rollback。
- 使用 `httptest` 通过 `/openapi/v1/events` 验证对外错误结构和 audit 查询。
- 不接入外部风控、外部业务库或真实发奖系统。

## Decision

~~~text
PASS
~~~

需求：2026-05-24-risk-limit-budget
