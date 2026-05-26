# Test Plan

## Agent 指令

你是 Test Plan Agent。请在 Review PASS 后、Dev 开发前，根据 task.md、analysis.md、review.md 设计测试用例。

只写 test-plan.md，不改业务代码，不执行提交。

## Test Strategy

- 以规则引擎单元测试覆盖 REWARD 阶段的条件判断、Decimal 比较、字段缺失/配置错误解释，以及 `Detail.conditions` 条件级输出。
- 以 engine 服务测试覆盖事件处理和模拟运行链路，确认 REWARD 条件失败不会进入奖励计算，不预览或生成 grant_order/outbox，不扣 quota。
- 覆盖 Review P2：非金额类 reward 的 `reward_amount=0.0000` 只表示账务金额为 0；只要数量类权益有效，REWARD 条件通过后仍应生成 grant_order/outbox。
- 保持无外部业务库、无直接发奖、无业务专用订单/充值/投注/签到/邀请表的边界。

## Test Cases

- `rule.Execute` 按 `MATCH -> QUALIFY -> REWARD -> RISK_LIMIT_BUDGET` 执行，并在 REWARD 失败时设置 `FailedStage=REWARD`、`CanReward=false`、`CanReserveQuota=false`。
- REWARD 条件 `attributes.amount >= 100.00` 在实际值 `99.99` 时失败，输出字段、期望值、实际值和失败原因。
- REWARD 规则为空 `conditions` 时运行期返回 `INVALID_RULE_CONFIG`，不得默认通过。
- 多条件 REWARD 规则在 `Detail.conditions` 中输出每个条件的 `field/op/expected/actual/passed/reason`。
- 模拟运行中 REWARD 条件失败时，participation 预览为 `REJECTED`，不展示 grant order/outbox command 预览。
- 事件处理中 REWARD 条件通过且 reward 为 COUPON 时，即使 `reward_amount=0.0000`，仍生成 grant_order/outbox。
- 回归既有规则：disabled rule 跳过、Fact 缺失失败、数组 `IN/NOT_IN` 匹配、QUALIFY 失败阻断奖励和 quota。

## Required Commands

~~~bash
go test ./...
go build ./...
~~~

## Data / Smoke

- 使用内存仓储构造活动、标准事件和奖励配置，不依赖外部服务。
- 金额字段使用字符串 Decimal，例如 `attributes.amount="99.99"`、阈值 `"100.00"`。
- 非金额 reward 使用 COUPON：`couponTemplateId=WELCOME_10`、`quantity=1`、`targetRef=https://coupon.example.com/grant`。

## Decision

~~~text
PASS
~~~

需求：2026-05-24-reward
