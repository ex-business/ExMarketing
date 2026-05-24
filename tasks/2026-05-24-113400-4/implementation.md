# Implementation

## Agent 指令

你是 Dev Agent。只有 review.md 为 PASS 且 test-plan.md 为 PASS 时才能开发。

必须按 task.md、analysis.md、review.md、test-plan.md 执行，不得扩大范围。

完成开发后必须先做基础验证：

~~~bash
go test ./...
go build ./...
~~~

## Summary

已实现统一 RewardCalculator 的六类计算方式：

```text
FIXED
RATIO
LADDER
RANDOM
PROBABILITY
POOL_SPLIT
```

金额、比例、权重、分摊和步长计算均使用 `internal/decimal` 的 `big.Rat` Decimal 语义，未引入 float64 业务计算。计算结果保留现有 4 位小数输出口径。

## Changed Files

```text
internal/decimal/decimal.go
internal/marketing/model.go
internal/reward/calculator.go
internal/reward/calculator_test.go
internal/engine/service.go
docs/data-model.md
docs/api.md
tasks/2026-05-24-113400-4/implementation.md
```

## Behavior Changes

1. `marketing.CalculateType` 新增 `LADDER / RANDOM / PROBABILITY / POOL_SPLIT`，创建活动校验同步放行并校验对应 rewardConfig。
2. `RewardConfig` 新增阶梯、随机、概率奖项和奖池分摊字段；`BONUS / CASH` 要求 `currency`。
3. `ConfigCalculator` 返回稳定 reasonCode，并在 payload 中写入计算解释字段。
4. `RATIO` 支持 Decimal 比例计算和 `maxAmount` 封顶。
5. `LADDER` 支持乱序配置、闭区间命中，并拒绝重叠区间。
6. `RANDOM` 使用 tenantId、activityId、rewardCode、idempotencyKey 或 fallback、reward version、seedSalt 生成确定性结果。
7. `PROBABILITY` 使用 Decimal 权重确定性抽取；item.rewardType 缺省使用外层 rewardType。
8. `POOL_SPLIT` 支持 `COUNT` 均分和 `WEIGHT` 权重即时分摊。
9. 计算结果为 `0.0000` 时返回 `ZERO_REWARD_SKIPPED`，事件处理和模拟链路不生成 grant_order/outbox，也不扣 quota。
10. API 和数据模型文档已同步六类计算方式、Decimal 口径、0 金额跳过和 POOL_SPLIT 即时分摊边界。

## Notes

验证已执行：

```bash
go test ./...
go build ./...
```

结果均为 PASS。

需求：2026-05-24-113400-4
