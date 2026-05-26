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

已按 `task.md`、`analysis.md`、`review.md` 和 `test-plan.md` 做最小实现：

- 扩展 `marketing_reward` Schema 校验，明确支持 `BONUS / CASH / COUPON / POINT / GIFT / DRAW_CHANCE / MEMBERSHIP`。
- 金额类 `BONUS / CASH` 保持支持 `FIXED / RATIO / LADDER / RANDOM / PROBABILITY / POOL_SPLIT`。
- 非金额类当前最小可上线口径只支持 `FIXED`，其他 `calculateType` 组合在创建/上线前校验阶段拒绝，避免运行期产生不可解释 payload。
- 固化非金额 reward 的 `reward_amount` 语义：grant order 中为 `0.0000`，实际权益字段写入 `reward_payload_json`。
- 处理 Review P2：
  - `POOL_SPLIT` 支持现有 `splitBy`，兼容 `splitMode` 别名；两者同时存在且冲突时拒绝。
  - 标准 payload 文档明确非金额权益字段和估值字段。
  - `PROBABILITY.items[].rewardType` 若出现，必须等于外层 `rewardType`，拒绝混合类型。

## Changed Files

- `internal/marketing/model.go`
  - 扩展 `RewardConfig` 字段。
  - 新增 rewardType 专项 Schema 校验、非金额 FIXED 必填字段校验、非法字段拒绝、`flowMultiple` 规则、`splitMode` 兼容校验。
- `internal/reward/calculator.go`
  - 支持非金额 FIXED reward 生成 payable result，`amount=0.0000` 但不跳过。
  - `POOL_SPLIT` 读取 `splitBy` 或 `splitMode`。
- `internal/marketing/prelaunch_validation.go`
  - 非金额 reward 预算暴露按 `valuationAmount` 估值；未配置估值时预算暴露为 0 且确定。
- `internal/marketing/prelaunch_validation_test.go`
  - 覆盖 7 类 rewardType 正向 Schema。
  - 覆盖 `flowMultiple`、非金额误传字段、缺必填字段、非法数量、非法组合、概率混合类型、split alias 冲突。
- `internal/reward/calculator_test.go`
  - 覆盖非金额 FIXED reward 生成 payable payload。
- `docs/api.md`、`docs/data-model.md`、`README.md`
  - 更新 reward schema、计算组合、非金额 `reward_amount` / payload 语义和文档描述。
- `tasks/2026-05-24-reward-schema/test-plan.md`
  - 补齐并修正为与本轮实现一致的 PASS 测试计划。

## Behavior Changes

- 创建/上线前校验现在允许 7 种 rewardType。
- `POINT` 必须配置 `points` 正整数和 `unit` 或 `pointType`，且不允许 `currency`。
- `COUPON` 必须配置 `couponTemplateId` 和 `quantity` 正整数；`faceValue` 与 `discountRate` 不能混用。
- `GIFT` 必须配置 `giftSkuId` 或 `giftCode`，以及 `quantity` 正整数。
- `DRAW_CHANCE` 必须配置 `drawPoolCode` 或 `lotteryCode`，以及 `chanceCount` 正整数。
- `MEMBERSHIP` 必须配置权益引用，并配置 `durationDays` 正整数或 `durationType=PERMANENT`。
- 非 `BONUS` reward 的 `flowMultiple` 只能为 `0` 或不传。
- `PROBABILITY` 不再允许 item `rewardType` 与外层 rewardType 混合。
- `POOL_SPLIT` 可用 `splitMode` 兼容字段，但与 `splitBy` 冲突会拒绝。
- 非金额 FIXED reward 会生成 grant_order/outbox，`reward_amount=0.0000`，payload 包含权益字段。

## Notes

验证已执行：

```bash
go test ./...
go build ./...
```

结果：均通过。

需求：2026-05-24-reward-schema
