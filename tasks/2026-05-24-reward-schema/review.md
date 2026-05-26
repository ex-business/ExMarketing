# Review

## Findings

```text
P0:
无。

P1:
无。

P2:
1. tasks/2026-05-24-reward-schema/analysis.md:102,270 使用 `splitMode=COUNT|WEIGHT` 描述 POOL_SPLIT，但现有代码模型字段为 `rewardConfig.splitBy`，已有计算器也按 `splitBy` 解析。Dev/Test 阶段需要明确是否兼容旧字段或统一改名，并补充 API/数据模型文档与回归测试，避免同一 Schema 出现两个字段名。

2. tasks/2026-05-24-reward-schema/analysis.md:81-83,134-183 要求新增 COUPON/GIFT/DRAW_CHANCE/MEMBERSHIP 等非金额奖励输出，但当前主链路仍以 `marketing_grant_order.reward_amount` 加 `reward_payload_json` 为核心。analysis 已要求 payload 保留类型、单位和解释字段，不构成阻断；实现时仍应固定标准发奖 payload 的字段名、预算估值字段和非金额 reward 的 `reward_amount` 语义，避免 outbox 对接方和对账接口理解不一致。

3. tasks/2026-05-24-reward-schema/analysis.md:261-264 允许 PROBABILITY item 产生与 rewardType 匹配的 payload，但未明确是否允许单个 probability 配置内混合不同 rewardType。此前 reward calculator 评审已指出混合类型会影响 grant_order.reward_type 与 payload 一致性；建议本任务默认 item rewardType 缺省或等于外层 rewardType，若要混合必须单独补测试矩阵。
```

## Open Questions

```text
- 无阻断性待确认问题。analysis 已对 7 种 rewardType、6 种 calculateType、AUTO + HTTP/WEBHOOK、MQ/MANUAL_EXPORT/MANUAL 拒绝上线、targetRef、flowMultiple 和非法配置拒绝口径给出可验收默认规则。
```

## Score

```text
88 / 100
```

评分：

```text
需求清晰度        18/20
范围可控          17/20
风险可控          18/20
验收标准明确      18/20
架构一致性        17/20
```

扣分原因：analysis 覆盖了 task.md 要求的 7 类奖励边界，并把不支持发奖方式收敛为拒绝上线；扣分主要来自 POOL_SPLIT 字段命名与现有实现存在兼容风险，以及非金额奖励的标准 payload / reward_amount 语义需要在 Dev/Test 阶段进一步固化。

## Decision

```text
PASS
```

理由：未发现 P0/P1，Score >= 80。analysis.md 已补齐 reward schema 的字段契约、rewardType/calculateType 组合边界、非法配置拒绝、当前可上线 grant mode/channel、风险与验收标准，可进入后续 Test Plan / Dev。

规则：

```text
存在 P0/P1 -> BLOCK
Score < 80 -> BLOCK
无 P0/P1 且 Score >= 80 -> PASS
```

需求：2026-05-24-reward-schema
