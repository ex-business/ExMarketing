# Review

## Findings

```text
P0:
无。

P1:
无。

P2:
1. PROBABILITY 允许 item.rewardType 覆盖外层 rewardType，但 marketing_reward / marketing_grant_order 都有单一 reward_type 字段。建议 Dev 阶段明确最终 grant_order.reward_type 使用命中奖项 rewardType 还是外层 rewardType，并补充测试；否则概率奖励混合 BONUS/CASH/POINT 时容易出现 payload 与发奖单字段不一致。
2. RANDOM / PROBABILITY 的 seed 口径已要求稳定，但 seed material 的最终拼接字段仍偏描述性。建议实现时固定包含 tenantId、activityId、rewardCode、idempotencyKey 或 fallback eventId、rewardConfig version、seedSalt，并在测试中断言同版本稳定、换 seedSalt 可变。
3. POOL_SPLIT 的即时分摊边界已写清楚，但运行时按单次 rewardAmount 扣 BUDGET，可能被误用为“活动总奖池只扣一次”。建议创建校验或 API 文档中显式标注该模式不是结算型奖池，避免运营配置期理解偏差。
```

## Open Questions

```text
1. PROBABILITY 是否真的需要支持单个 rewardConfig 内混合不同 rewardType？如果没有强需求，建议 item.rewardType 仅允许缺省或等于外层 rewardType。
2. Decimal 输出“四舍五入到 4 位”的 rounding mode 是否统一为 half-up，还是沿用 Go decimal 库默认语义？
```

## Score

```text
88 / 100
```

## Decision

```text
PASS
```

理由：Score >= 80，且未发现 P0/P1。新版 analysis.md 已按 task.md 覆盖 FIXED / RATIO / LADDER / RANDOM / PROBABILITY / POOL_SPLIT 六类计算方式，并补齐最小普通活动口径、0 金额跳过、稳定 reasonCode、Decimal 精度、确定性随机、预算/库存/用户次数语义和验收标准。上述 P2 不阻断进入 Dev/Test，但应在实现和文档同步时收敛。

需求：2026-05-24-113400-4
