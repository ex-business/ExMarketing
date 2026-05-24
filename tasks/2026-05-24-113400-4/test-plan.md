# Test Plan

## Agent 指令

你是 Test Plan Agent。请在 Review PASS 后、Dev 开发前，根据 task.md、analysis.md、review.md 设计测试用例。

只写 test-plan.md，不改业务代码，不执行提交。

## Test Strategy

Review Decision 已确认 PASS。本测试计划面向后续 Dev/Test Agent，用于验证统一 RewardCalculator 的业务口径，不要求 Test Plan Agent 执行任何命令。

测试分层：

1. 单元测试优先覆盖 RewardCalculator 纯计算逻辑，要求使用 Decimal 断言，禁止以 float64 结果作为期望值。
2. 服务/事务测试覆盖活动命中后“先全部计算、再写 grant_order/outbox/quota”的原子性。
3. 幂等、额度不足、并发测试覆盖 event_inbox、quota、grant_order、outbox 的一致性。
4. PostgreSQL smoke 使用真实 PostgreSQL 或项目既有集成测试环境验证事务回滚、唯一约束、Decimal 持久化精度和重复事件行为。
5. 模拟运行测试必须确认只返回预计结果、skipped 或失败原因，不写业务表。

通过标准：

- 覆盖 `FIXED / RATIO / LADDER / RANDOM / PROBABILITY / POOL_SPLIT` 六类 `calculateType`。
- 覆盖正常路径、额度不足、幂等重复、并发、PostgreSQL smoke。
- 覆盖 `0` 金额跳过、Decimal 4 位小数、确定性随机、配置错误、字段错误和多奖励原子失败。
- 所有失败断言使用稳定 `reasonCode`，不得只断言自然语言错误。

## Test Cases

### A. RewardCalculator 正常路径

1. `FIXED` 固定金额：`amount=18.88`，`rewardType=BONUS`，`currency=CNY`，期望 `rewardAmount=18.88`、`reasonCode=OK`、payload 含 `source=FIXED` 和 `currency`。
2. `RATIO` 普通比例：`attributes.amount=100.00`、`rate=0.20`，期望 `20.00`。
3. `RATIO` 封顶：`attributes.amount=9999`、`rate=0.20`、`maxAmount=500.00`，期望 `500.00`，payload 断言 `capped=true`。
4. `LADDER` 阶梯命中：分别用首档、中间档、末档输入，断言命中金额和 payload 中的 `ladderIndex/min/max`。
5. `LADDER` 闭区间边界：字段值等于某档 `min`、等于某档 `max` 均命中该档。
6. `RANDOM` 默认 step：`minAmount=1.00`、`maxAmount=10.00`，同一 tenant/activity/reward/idempotencyKey/version 下重复计算金额一致，payload 含 `selectedIndex`、`candidateCount`。
7. `RANDOM` 自定义 step 和非整除区间：例如 `min=1.00`、`max=1.05`、`step=0.02`，候选集合不得包含超过 `1.05` 的值。
8. `RANDOM` `minAmount=maxAmount`：直接返回固定值。
9. `PROBABILITY` 多奖项：配置 `NO_PRIZE/SMALL/BIG` 权重，使用稳定 seed 断言同一输入命中同一 `itemCode`，payload 含 `selectedIndex`、`weight`、`totalWeight`。
10. `PROBABILITY` 命中奖励项：命中 `amount>0` 时生成实际奖励，`rewardType` 缺省使用外层 `rewardType`。
11. `POOL_SPLIT` COUNT：`poolAmount=1000.00`、`participantCount=4`，期望 `250.00`。
12. `POOL_SPLIT` WEIGHT：`poolAmount=1000.00`、`weight=3`、`totalWeight=10`，期望 `300.00`。

### B. Decimal 与精度

1. 比例精度：`baseAmount=0.30`、`rate=0.10`，期望 `0.03`，不得出现 `0.029999...`。
2. 四舍五入：构造 `rewardAmount` 超过 4 位小数的 RATIO/POOL_SPLIT 用例，期望最多 4 位小数。
3. Decimal 解析：金额、比例、权重、奖池金额均用字符串配置并按 Decimal 断言。
4. 负数禁止：固定金额、比例、maxAmount、阶梯 amount、随机区间、概率 amount/weight、poolAmount 出现负数时返回对应错误，不生成奖励。

### C. 配置和输入错误

1. `rewardConfig` 为 `null`、空对象、数组、字符串，期望 `INVALID_REWARD_CONFIG`。
2. 未知 `calculateType`，期望 `UNSUPPORTED_CALCULATE_TYPE`。
3. 金额类 `BONUS/CASH` 缺 `currency`，期望明确配置错误。
4. `AUTO + HTTP/WEBHOOK` 缺 `targetRef` 或 `targetRef` 非 `http/https`，期望 `INVALID_REWARD_TARGET`。
5. 字段路径缺失、命名空间非法、字段不存在、字段值为 `null`、空字符串、布尔值、对象、数组、不可解析 Decimal，期望 `FIELD_MISSING`、`FIELD_TYPE_MISMATCH` 或 `DECIMAL_PARSE_ERROR`，不得按 0 计算。
6. `RATIO` 缺 `baseField`、缺/非法/负数 `rate`、非法/负数 `maxAmount`。
7. `LADDER` 空 ladders、缺 `min/max/amount`、`min>max`、阶梯重叠、未命中任何档位；未命中期望 `LADDER_NOT_MATCHED`。
8. `RANDOM` `minAmount>maxAmount`、`step=0`、负数 step、step 大于区间且 `min!=max`，期望 `RANDOM_RANGE_INVALID` 或 `RANDOM_STEP_INVALID`。
9. `PROBABILITY` 空 items、重复 `itemCode`、`weight=0`、负数 weight、非法 amount、非法 item `rewardType`、item currency 与外层 currency 不一致。
10. `POOL_SPLIT` 缺 split 字段、非法 `splitBy`、`participantCount=0`、`weight<0`、`totalWeight=0`、`weight>totalWeight`。

### D. 0 金额和 disabled

1. `FIXED amount=0` 返回 `ZERO_REWARD_SKIPPED` 或等价 skipped 状态，不生成 grant_order/outbox，不扣 BUDGET/STOCK/USER_LIMIT。
2. `RATIO` base 或 rate 导致结果为 0，按 0 金额跳过。
3. `LADDER` 命中 `amount=0` 阶梯，按 0 金额跳过。
4. `RANDOM` 结果为 0，按 0 金额跳过。
5. `PROBABILITY` 命中空奖 `amount=0`，按 0 金额跳过。
6. `POOL_SPLIT poolAmount=0` 或 WEIGHT `weight=0`，按 0 金额跳过。
7. 同一活动所有 enabled reward 都为 0，不能写 REWARDABLE 发奖链路。
8. `enabled=false` reward 不参与正式计算；模拟运行如展示则 reason 为 `SKIPPED_DISABLED_REWARD`，不写表、不扣 quota。

### E. 多奖励、额度不足和事务一致性

1. 多个 enabled reward 全部成功且金额大于 0：应为每个 reward 生成 grant_order/outbox；BUDGET 扣减成功金额合计；STOCK 按 rewardCode 各扣 1；USER_LIMIT 扣 1。
2. 多奖励中部分为 0、部分大于 0：只给大于 0 的 reward 生成 grant_order/outbox；BUDGET 只扣实际金额；STOCK 只扣实际发奖 reward；USER_LIMIT 因存在实际发奖扣 1。
3. 多奖励中任一 enabled reward 计算失败：整个 activity 不生成任何 grant_order/outbox，不扣 BUDGET/STOCK/USER_LIMIT。
4. 额度不足 BUDGET：奖励计算成功后 quota 扣减失败，应返回或记录 `409 QUOTA_EXCEEDED`，回滚 participation/grant_order/outbox，不产生 outbox。
5. 额度不足 STOCK：库存不足时同样回滚 grant_order/outbox/quota。
6. 额度不足 USER_LIMIT：用户次数不足时同样回滚 grant_order/outbox/quota。
7. 额度不足发生在多个 reward 的后一个 quota 维度时，前面已尝试的扣减必须全部回滚。

### F. 幂等重复

1. 完全相同事件重复上报：第二次返回 duplicate 或等价状态，不重新计算、不新增 participation/grant_order/outbox、不重复扣 quota。
2. 重复事件包含 `RANDOM`：重复执行结果和首次 grant_order 金额一致，不因当前时间或进程随机数变化。
3. 重复事件包含 `PROBABILITY`：重复执行命中同一 `itemCode`，不重复发奖。
4. 模拟运行和正式执行使用相同输入时，`RANDOM/PROBABILITY` 结果一致；模拟运行不得提前占用幂等记录或 quota。
5. 同一 eventId 但不同 idempotencyKey 的处理按项目既有幂等规则断言，避免重复扣减口径不明确。

### G. 并发

1. 并发提交同一 idempotencyKey 的同一事件，例如 20 个 goroutine：最终只生成 1 组 participation/grant_order/outbox/quota 扣减，其余为 duplicate 或冲突后的幂等结果。
2. 并发提交不同 idempotencyKey 但命中同一活动预算：总 BUDGET 扣减不得超过预算；超出请求返回 `QUOTA_EXCEEDED` 或等价失败，并且失败请求无 grant_order/outbox。
3. 并发提交不同用户但共享 rewardCode 库存：STOCK 不得扣成负数，grant_order 数量不得超过库存。
4. 并发触发 USER_LIMIT：同一用户在 limit=1 下多请求只允许 1 次实际发奖。
5. 并发包含 `RANDOM/PROBABILITY`：每个成功 idempotencyKey 的结果稳定可重放，失败请求不写发奖副作用。

### H. PostgreSQL Smoke

1. 使用真实 PostgreSQL 跑最小活动创建/事件处理链路，验证 `reward_amount` 按 Decimal 精度持久化，最多 4 位小数。
2. 正常 FIXED/RATIO 活动 smoke：事件处理后存在 participation、grant_order、outbox 和 quota 变更。
3. 额度不足 smoke：设置预算小于奖励金额，事件处理返回 `QUOTA_EXCEEDED`，查询确认无 grant_order/outbox 残留，quota 未被错误扣减。
4. 幂等 smoke：重复插入或重复处理同一事件，确认唯一约束/幂等逻辑生效，grant_order/outbox 数量不增加。
5. 并发 smoke：在 PostgreSQL 下并发处理同一事件和共享预算事件，确认事务隔离下不超扣、不重复发奖。
6. 模拟运行 smoke：调用模拟路径后查询 event_inbox、participation、grant_order、outbox、quota 均无新增。

## Required Commands

~~~bash
# Unit and package tests
go test ./...

# Race/concurrency focused tests, after Dev/Test Agent identifies the relevant package path.
go test -race ./...

# Build verification
go build ./...

# PostgreSQL integration smoke.
# Use the repository's existing integration-test flag/profile/env names if they already exist.
# If no convention exists yet, Test Agent should document the exact local PostgreSQL DSN it used.
EXMARKETING_TEST_POSTGRES_DSN='postgres://postgres:postgres@127.0.0.1:5432/exmarketing_test?sslmode=disable' go test ./... -run 'Postgres|Integration|Smoke'
~~~

## Data / Smoke

Minimum fixture set for Test Agent:

1. Tenant/activity/reward fixture with enabled rewards for all six calculate types.
2. Event fixture with `tenantId`、`eventId`、`idempotencyKey`、`subjectId` and Decimal-compatible attributes such as `amount`、`continueDays`、`participantCount`、`userWeight`、`totalWeight`。
3. Quota fixture with sufficient BUDGET/STOCK/USER_LIMIT for normal path.
4. Quota fixture with insufficient BUDGET, insufficient STOCK, and exhausted USER_LIMIT.
5. Duplicate fixture using the exact same idempotency key and payload.
6. Concurrency fixture with small budget/stock/user limit to expose over-deduction.
7. PostgreSQL fixture that starts from a clean schema or transactional cleanup, then verifies table counts and persisted amounts after each smoke.

PostgreSQL smoke assertions:

- Success path: participation count increases by 1, grant_order/outbox count matches actual non-zero rewards, quota deltas equal expected BUDGET/STOCK/USER_LIMIT.
- Zero reward path: no grant_order/outbox and no quota delta.
- Failure path: no partial grant_order/outbox/quota side effects.
- Duplicate path: second handling does not change counts or quota totals.
- Concurrency path: database state respects unique constraints and quota limits.

## Decision

~~~text
PASS
~~~

理由：`review.md` Decision 为 PASS；本计划已覆盖正常路径、额度不足、幂等重复、并发、PostgreSQL smoke，以及六类奖励计算和关键边界条件，可交给后续 Dev/Test Agent 执行。

需求：2026-05-24-113400-4
