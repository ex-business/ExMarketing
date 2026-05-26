# Test Plan

## Agent 指令

你是 Test Agent。请在 Dev 完成 `2026-05-24-reward-schema` 后，按本文件执行测试。

约束：

- 只执行本计划列出的测试、构建和 smoke 命令。
- 若发现失败，只记录失败现象、复现命令、关键日志和疑似责任模块，不直接修改业务代码。
- 必须覆盖正常路径、额度不足、幂等重复、并发、PostgreSQL smoke。
- 测试结论写入 `tasks/2026-05-24-reward-schema/test.md`。

## Test Strategy

本需求验证重点是 `marketing_reward` 的配置 Schema、运行期发奖链路和存储一致性，而不是外部钱包、优惠券、积分、礼品、抽奖或会员系统的真实发放。

测试分层：

1. Schema / validation 单元测试：验证 7 种 `rewardType`、6 种 `calculateType`、`grantMode`、`grantChannel`、`targetRef`、`flowMultiple`、Decimal、未知字段和非法组合。
2. Reward calculator 单元测试：验证合法配置可计算出稳定且类型正确的 preview / payload，非法配置不会进入计算链路。
3. HTTP API / prelaunch validation 测试：验证 Admin 创建、更新、上线前校验对非法配置返回可定位错误。
4. Workflow / repository 集成测试：验证 participation -> quota -> grant_order -> outbox 链路，不绕过幂等，不直接发奖。
5. PostgreSQL smoke：验证迁移、约束、JSON payload、幂等键、quota 扣减和并发事务在真实 PostgreSQL 下可运行。

## Test Cases

### 1. 正常路径 - rewardType 覆盖

为每个 `rewardType` 准备至少 1 个合法 `AUTO + HTTP` 或 `AUTO + WEBHOOK` 配置，调用 Admin 创建/更新和上线前校验，预期全部通过，并在模拟/运行 preview 中保留 `rewardCode`、`rewardType`、`calculateType`、`grantChannel`、`targetRef`、`flowMultiple`、计算解释字段。

覆盖矩阵：

| rewardType | calculateType | 核心断言 |
|---|---|---|
| `BONUS` | `FIXED` | 金额 `amount + currency` 正确；`flowMultiple >= 0` 可通过；payload 保留流水倍数。 |
| `CASH` | `RATIO` | 按 Decimal `baseField * rate` 计算，封顶生效；`flowMultiple=0`；不得产生二进制浮点误差。 |
| `POINT` | `FIXED` | 输出正整数 `points` 和 `unit/pointType`；不得要求 `currency`。 |
| `COUPON` | `FIXED` | `couponTemplateId + quantity` 必填；输出券 payload，不把券数量写成金额。 |
| `GIFT` | `FIXED` | 输出 `giftSkuId/giftCode + quantity`；如配置估值，只作为预算估值字段。 |
| `DRAW_CHANCE` | `FIXED` | 输出正整数 `chanceCount` 和外部玩法引用；不得包含金额输出。 |
| `MEMBERSHIP` | `FIXED` | 输出 `membershipTier/membershipPlanId/benefitPackageId` 与 `durationDays` 或 `durationType=PERMANENT`。 |

### 2. 正常路径 - calculateType 覆盖

为每个 `calculateType` 准备合法配置，验证计算结果、preview 和 grant payload。

- `FIXED`：金额类和数量类各 1 例；金额类 0 值配置应返回 skipped preview，不生成 grant_order/outbox；非金额类正整数权益应生成 `reward_amount=0.0000` 的 grant_order/outbox。
- `RATIO`：金额类 `baseField` 存在、rate 为 Decimal 字符串、`maxAmount` 封顶；非金额类当前应拒绝。
- `LADDER`：边界值命中最小阶梯、最大阶梯和阶梯交界点；闭区间、排序和输出字段正确。
- `RANDOM`：同一 tenant、activity、rewardCode、idempotency key、version 下结果稳定；不同 key 可产生不同结果；范围和 step 合法。
- `PROBABILITY`：金额类权重总和大于 0；item `rewardType` 缺省或等于外层 rewardType；0 金额 item 不生成发奖单。
- `POOL_SPLIT`：金额类使用 `poolAmount + splitBy/splitMode + currency`；按 COUNT/WEIGHT 分摊，余数处理稳定且可解释。

### 3. 非法配置拒绝

对 Admin 创建/更新和上线前校验分别执行以下负向用例，预期返回 400 类校验错误或上线阻断结果；错误必须包含 `rewards[index]`、字段名、错误码和原因；不得保存为可运行 ACTIVE 配置。

- 未知 `rewardType`、`calculateType`、`grantMode`、`grantChannel`。
- `rewardConfig` 为数组、字符串、数字、布尔或 null。
- `rewardConfig` 出现未允许字段，尤其是 secret、token、password、private key 类字段。
- `rewardCode` 为空、重复、含不可追踪字符。
- Decimal 字段为空、非法、负数、科学计数法不符合约定、超过 4 位小数。
- 金额类缺少 `currency`；非金额类误传实际发奖金额或 `currency`。
- `targetRef` 缺失、空串、相对路径、非 http(s) 协议、内联 secret。
- `flowMultiple` 为负、非法 Decimal、非 `BONUS` 大于 0。
- 顶层 `flowMultiple` 与兼容旧字段 `rewardConfig.flowMultiple` 不一致。
- `enabled=false` 但 Schema 非法，仍应拒绝保存。
- `MQ`、`MANUAL_EXPORT`、`MANUAL` 在当前阶段拒绝上线，不得降级为 HTTP 或运行时静默跳过。

### 4. rewardType 专项负向

- `CASH` 配置 `flowMultiple > 0`：拒绝。
- `POINT` 配置 `currency` 或小数 points：拒绝。
- `COUPON` 缺 `couponTemplateId`、`quantity <= 0`、`faceValue` 与 `discountRate` 混用为不可解释配置：拒绝。
- `GIFT` 缺 `giftSkuId/giftCode`、`quantity <= 0`、配置 `valuationAmount` 但缺 `currency`：拒绝或阻断。
- `DRAW_CHANCE` 缺 `drawPoolCode/lotteryCode`、`chanceCount <= 0`、包含金额输出：拒绝。
- `MEMBERSHIP` 缺权益引用、`durationDays <= 0` 且非 `PERMANENT`：拒绝。

### 5. calculateType 专项负向

- `RATIO`：`baseField` 不在白名单、缺失、不可解析 Decimal；`rate < 0`；`maxAmount < 0` 或小于计算下限。
- `LADDER`：空数组、`min > max`、区间重叠、边界不可解析、输出字段与 `rewardType` 不匹配。
- `RANDOM`：`min > max`、`step <= 0`、step 无法解释覆盖范围、缺稳定 seed 口径。
- `PROBABILITY`：空 items、权重总和为 0、负权重、item rewardType 与外层 `rewardType` 不一致。
- `POOL_SPLIT`：缺 `poolAmount`、缺 `splitBy/splitMode`、非法分摊模式、金额类缺 `currency`、非金额 rewardType 使用 POOL_SPLIT。

### 6. 额度不足 / quota 不足

准备预算、库存、用户次数不足的活动配置，触发同一奖励链路。

预期：

- BUDGET 不足时不生成 grant_order/outbox，不扣减库存和用户次数，participation 或 preview 记录额度不足原因。
- STOCK 不足时不生成 grant_order/outbox，不扣预算和用户次数。
- USER_LIMIT 不足时不生成 grant_order/outbox，不扣预算和库存。
- 多个 quota 同时不足时错误原因稳定可解释，不产生部分扣减。
- 零奖励、空奖和额度不足都不得写入可执行 outbox。

### 7. 幂等重复

使用同一 tenant、activity、user、event/idempotency key 重复提交相同事件。

预期：

- 第一次成功时最多生成 1 条 participation、1 组 quota 扣减、1 条 grant_order、1 条 outbox。
- 后续重复请求返回已有结果或幂等成功，不重复扣 BUDGET/STOCK/USER_LIMIT。
- `RANDOM` 和 `PROBABILITY` 在重复请求下返回相同结果。
- 若第一次命中空奖或 0 奖励，重复请求仍不生成 grant_order/outbox。
- 若第一次因 quota 不足失败，重复请求不得通过创建新 key 绕过同一业务幂等口径。

### 8. 并发

使用并发请求同时触发同一 activity/reward/quota。

场景：

- BUDGET 只够 1 次，10 个并发请求竞争。
- STOCK 只够 1 件，10 个并发请求竞争。
- USER_LIMIT 为 1，同一用户 10 个并发重复/不同事件竞争。
- 不同用户并发消费同一活动总预算。

预期：

- 成功数量不超过 quota。
- quota 扣减、grant_order、outbox 数量与成功 participation 一致。
- 无负库存、负预算、重复 grant_order、重复 outbox。
- 失败请求返回可解释的 quota 不足或幂等结果。
- PostgreSQL 集成测试下无死锁；若出现事务冲突，应被重试或明确失败，不产生脏数据。

### 9. Outbox / grant payload

对可发奖配置验证生成的 grant_order 和 outbox。

- `reward_payload_json` 保留 `rewardCode`、`rewardType`、`calculateType`、`currency/unit`、`flowMultiple`、`targetRef`、计算来源和原因字段。
- 非金额奖励的 `reward_amount` 语义固定为 `0.0000`；coupon/gift/draw/membership 的实际权益字段必须在 payload 中，`valuationAmount` 只作为可区分估值字段。
- `commandId` 或发奖幂等 key 稳定，重复请求不变。
- `AUTO + HTTP/WEBHOOK` 的 outbox 指向 `targetRef`，不直接调用外部系统完成余额或权益修改。

### 10. PostgreSQL Smoke

在真实 PostgreSQL 环境执行迁移和仓储集成测试。

准备：

- 使用 `docker-compose.yml` 启动 PostgreSQL 依赖，或使用 Test Agent 当前环境提供的 PostgreSQL DSN。
- 清理测试 tenant/activity 相关数据，避免污染其他任务数据。

验证：

- `marketing_reward` 能保存 7 类合法配置的 JSON。
- 非法配置不会保存为 ACTIVE 可运行记录。
- `marketing_grant_order.reward_payload_json` 能保存非金额 reward payload。
- 幂等唯一约束阻止重复 grant_order/outbox。
- 并发 quota 扣减在事务下保持一致。
- migration 从空库可完整应用。

## Required Commands

Test Agent 在 Dev 完成后执行以下命令；本 Test Plan Agent 不执行。

~~~bash
go test ./internal/reward -run 'Test.*Reward.*|Test.*Calculator.*|Test.*Schema.*' -count=1
go test ./internal/marketing -run 'Test.*Prelaunch.*|Test.*Reward.*|Test.*Quota.*|Test.*Idempot.*|Test.*Concurrent.*' -count=1
go test ./internal/httpapi -run 'Test.*Reward.*|Test.*Activity.*|Test.*Validation.*' -count=1
go test ./internal/worker -run 'Test.*Outbox.*|Test.*Grant.*' -count=1
go test ./internal/postgres -run 'Test.*Repository.*|Test.*Reward.*|Test.*Quota.*|Test.*Idempot.*|Test.*Concurrent.*' -count=1
go test ./... -count=1
go build ./...
~~~

PostgreSQL smoke 命令按项目实际 DSN 变量执行；若本地依赖未启动，先启动依赖后再执行：

~~~bash
docker compose up -d postgres
EXMARKETING_DATABASE_DSN='postgres://postgres:postgres@127.0.0.1:5432/exmarketing?sslmode=disable' go test ./internal/postgres -run 'Test.*Smoke.*|Test.*Repository.*' -count=1
~~~

若仓库使用的 DSN 环境变量名不是 `EXMARKETING_DATABASE_DSN`，Test Agent 应读取现有 `internal/postgres` 测试约定后替换为项目实际变量名，并在 `test.md` 记录实际命令。

## Data / Smoke

建议 Test Agent 准备以下最小 smoke 数据：

- tenant：`tenant_reward_schema_smoke`
- activity：`activity_reward_schema_smoke`
- users：`user_reward_schema_a`、`user_reward_schema_b`
- rewardCode：`bonus_fixed`、`cash_ratio`、`point_fixed`、`coupon_fixed`、`gift_fixed`、`draw_fixed`、`membership_fixed`、`pool_split_bonus`
- targetRef：`https://reward-smoke.example.test/grant`
- idempotency keys：`idem_reward_schema_001`、`idem_reward_schema_dup`、`idem_reward_schema_concurrent_%02d`

基础 quota：

- 正常路径：BUDGET 足够 100 次，STOCK 足够 100 次，USER_LIMIT 足够 10 次。
- 额度不足：BUDGET=0、STOCK=0、USER_LIMIT=0 分别独立验证。
- 并发：BUDGET/STOCK/USER_LIMIT 分别设置为 1，用 10 个并发请求竞争。

## Exit Criteria

通过标准：

- 所有 Required Commands 成功。
- 正常路径、额度不足、幂等重复、并发、PostgreSQL smoke 均有明确测试记录。
- 负向 Schema 校验覆盖非法枚举、非法组合、非法 Decimal、未知字段、非法 `targetRef`、非法 `flowMultiple`、概率/阶梯/随机/分摊错误。
- 未发现重复扣减、重复 grant_order、重复 outbox、负 quota、非金额 reward 被误当金额发放的问题。

阻断标准：

- 任一 P0/P1 数据一致性问题，例如重复发奖、负 quota、幂等失效、非法配置可上线。
- PostgreSQL smoke 无法通过，且不是环境缺失导致。
- `AUTO + HTTP/WEBHOOK` 无法生成可解释 grant payload 或 outbox。
- `MQ / MANUAL_EXPORT / MANUAL` 被允许上线。

## Decision

~~~text
PASS
~~~

理由：`review.md` Decision 为 PASS，需求边界可测试；本计划已覆盖正常路径、额度不足、幂等重复、并发、PostgreSQL smoke，以及 reward schema 的核心正负向矩阵。

需求：2026-05-24-reward-schema
