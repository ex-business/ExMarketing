# Test Plan

## Agent 指令

你是 Test Agent。`review.md` 已为 `PASS`，本文件为后续开发完成后的测试计划。

执行测试时必须按 `task.md`、`analysis.md`、`review.md` 和本文件验收，不得扩大需求范围。重点确认所有金额、预算、库存金额、奖励计算、费率、流水倍数和相关数值比较均使用 Decimal 语义，新增金额逻辑不得使用 `float64`。

本 Test Plan Agent 未执行任何测试、构建、格式化或 git 命令。

## Test Strategy

- 单元测试优先覆盖 `internal/decimal`、`internal/reward`、`internal/rule`、`internal/marketing`、`internal/engine` 中所有 Decimal 解析、计算、量化、封顶、阶梯和 quota 影响逻辑。
- 集成测试覆盖 `internal/postgres`，验证 PostgreSQL `DECIMAL(18,4)` 写入、读取、quota 原子扣减、幂等和回滚行为。
- 静态扫描覆盖新增金额相关代码，确认金额解析、计算、比较、JSON decode 和格式化路径不使用 `float64`；非金额用途如 worker jitter 可保留，但必须在测试报告中说明不属于金额逻辑。
- 所有金额输出断言必须使用字符串精确比较，例如 `59.8000`、`500.0000`，不得用浮点近似比较。
- 模拟运行和真实事件处理必须使用同一 Decimal 口径，`rewardPreviews`、`quotaImpactPreviews`、`grant_order.reward_amount`、outbox payload、snapshot 解释字段金额格式保持一致。

## Test Cases

### 1. 正常路径

- FIXED 奖励：`amount="18.88888"` 应输出 `18.8889`，payload 中金额字段保持字符串或 4 位 Decimal 表达。
- RATIO 比例赠送：`attributes.amount="299.00"`、`rate="0.20"` 应输出 `59.8000`，不得出现 `59.800000000000004`。
- RATIO 返水：`attributes.rebateBaseAmount="333.33"`、`rate="0.015"` 应输出稳定 4 位 Decimal；重复计算结果完全一致。
- RATIO tie-breaking：补边界样例 `baseAmount="1"`、`rate="0.00005"`，最终 4 位量化应固定为四舍五入口径的 `0.0001`，用于锁定 `.00005` 行为。
- RATIO 非封顶：`baseAmount="100.00"`、`rate="0.20"`、`maxAmount="50.00"` 应输出 `20.0000`，解释字段 `capped=false` 或等价未封顶语义。
- RATIO 封顶：`baseAmount="9999.99"`、`rate="0.20"`、`maxAmount="500.00"` 应输出 `500.0000`，解释字段体现 `capped=true`。
- LADDER 阶梯奖励：覆盖命中 `min`、命中 `max`、相邻边界和小数边界；命中后金额统一 4 位，例如 `58.0000`。
- 多 reward 合计：同一 activity 命中多个金额 reward 时，`BUDGET` delta 等于所有最终 4 位 `reward_amount` 的 Decimal 合计。
- 零金额：计算结果量化为 `0.0000` 时，应为 QUALIFIED/跳过语义，不生成 grant_order/outbox，不扣 BUDGET/STOCK/USER_LIMIT/DAILY_LIMIT。
- JSON number 兼容：历史 JSON number 输入如 `12.34` 必须通过 `UseNumber` 或等价方式保留文本语义，不能先转 `float64` 再计算。

### 2. 额度不足

- BUDGET 不足：预算剩余小于本次最终 reward 合计时，返回或记录 `QUOTA_EXCEEDED`，不得生成 participation rewardable 脏记录、grant_order 或 outbox，quota used/frozen 不应变化。
- STOCK 不足：多个 reward 中任一 rewardCode 库存不足时，整个原子命令回滚；已足够库存的 reward 也不得留下部分 grant_order/outbox。
- USER_LIMIT/DAILY_LIMIT 不足：重复用户或当日次数超限时，不生成发奖结果，quota aggregate 不被部分扣减。
- 模拟不足：simulate 只返回 quota risk / unavailable preview，不锁定 quota、不改变数据库。
- PostgreSQL 不足回滚：复用或扩展 `TestPostgresAtomicGrantRollsBackOnStockExceeded` 口径，额外断言 Decimal 金额 quota 行 `used_value` 仍为 `0.0000` 或数据库等价零值。

### 3. 幂等重复

- 同一标准事件重复处理：第二次应识别 duplicate/idempotent，不重复生成 participation、grant_order、outbox，不重复扣 quota；第一次与第二次返回的金额解释一致。
- 同一 grant idempotency key 重复创建：只能存在一条 grant_order，`reward_amount` 保持第一次 Decimal 字符串。
- 同一 inbox idempotency key、相同事件但 JSON number/string 表达不同：应按现有兼容口径识别为重复或稳定处理，不能因浮点格式化差异产生冲突。
- 同一 idempotency key 但业务事实不同：应返回 idempotency conflict，且不产生额外 quota 扣减。
- 重复模拟：相同输入连续 simulate，`rewardPreviews`、`quotaImpactPreviews`、snapshot 解释字段完全一致。

### 4. 并发

- 并发抢同一 BUDGET：并发 N 次下单或发奖，预算只允许成功可覆盖的次数，失败请求返回 `QUOTA_EXCEEDED`，最终 `used_value + frozen_value` 不超过 `total_value`。
- 并发抢同一 STOCK：库存为 `1.0000` 时，多 goroutine 只有一次成功，其余失败；不能出现两条成功 grant_order。
- 并发同一幂等键：多 goroutine 使用同一 event/grant idempotency key，最终只保留一份 participation/grant/outbox，quota 只扣一次。
- 并发不同幂等键但同一用户 USER_LIMIT：用户限制为 `1.0000` 时只有一次成功，失败无部分写入。
- PostgreSQL 并发测试必须在真实 PostgreSQL 上跑，不能只依赖 memory repository。

### 5. PostgreSQL Smoke

- 启动 PostgreSQL 后运行迁移，确认 `marketing_grant_order.reward_amount`、`marketing_quota.total_value/used_value/frozen_value` 是 `DECIMAL(18,4)` 语义。
- 写入 `reward_amount="59.8000"`、`quota total/used/frozen` 小数值后读取，返回文本应保持 4 位或数据库等价 Decimal，不得经 float 格式化。
- 真实执行 `CreateParticipationGrantOutboxesWithQuota`，确认 participation、grant_order、outbox、quota reservation 同事务成功；金额和 quota delta 都是 Decimal 字符串。
- 触发 quota 不足，确认事务回滚：participation、grant_order、outbox、quota_reservation 均无残留，quota aggregate 未变化。
- PostgreSQL JSON payload smoke：`reward_payload_json.amount`、snapshot/outbox payload 中金额字段应为字符串 Decimal；历史 JSON number 输入路径应经过 `UseNumber`。

## Required Commands

以下命令由后续 Test Agent 在 Dev Agent 完成实现后执行。本 Test Plan Agent 不执行。

~~~bash
# 静态扫描：金额相关新增逻辑不得使用 float64；允许非金额用途如 worker jitter，但需在测试报告中标注排除原因
rg -n "float64|float32|strconv\\.ParseFloat|FormatFloat|json\\.NewDecoder|UseNumber" internal cmd

# 核心单元测试
go test ./internal/decimal ./internal/reward ./internal/rule ./internal/marketing ./internal/engine

# 全量单元/集成可跳过测试
go test ./...

# 构建验证
go build ./...

# PostgreSQL smoke 环境
docker compose up -d postgres
export EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable'

# PostgreSQL 集成测试
go test ./internal/postgres -run 'TestPostgres' -count=1

# 并发/幂等相关回归，建议至少跑 10 轮暴露竞态
go test ./internal/marketing ./internal/postgres -run 'Concurrent|Idempot|Duplicate|Atomic|Quota' -count=10
~~~

## Data / Smoke

建议 Test Agent 准备以下最小数据集：

- tenant：`site_001`
- user：`user_decimal_001`
- activity：`act_decimal_001`
- reward codes：`RATIO_BONUS`、`REBATE_BONUS`、`LADDER_BONUS`
- quota：
  - `BUDGET / ACTIVITY / TOTAL / totalValue=100.0000`
  - `STOCK / ACTIVITY / RATIO_BONUS / totalValue=1.0000`
  - `USER_LIMIT / USER / user_decimal_001 / totalValue=1.0000`
  - `DAILY_LIMIT / USER / user_decimal_001:<date> / totalValue=1.0000`
- events：
  - normal ratio：`amount="299.00"`
  - cap ratio：`amount="9999.99"`
  - rebate：`rebateBaseAmount="333.33"`
  - insufficient：预算或库存小于最终 reward delta
  - duplicate：同一 `eventId` / idempotency key 重放
  - concurrent：多个不同 idempotency key 同时抢同一 quota

## Exit Criteria

- 正常路径、额度不足、幂等重复、并发、PostgreSQL smoke 全部有自动化测试或明确人工 smoke 记录。
- 所有金额断言均为字符串精确断言，输出统一 4 位小数。
- `go test ./...`、`go build ./...` 通过。
- PostgreSQL 集成测试在 `EX_MARKETING_DATABASE_URL` 指向真实 PostgreSQL 时通过。
- 静态扫描未发现新增金额逻辑使用 `float64`；若存在非金额 `float64`，测试报告必须说明文件、用途和排除理由。

## Decision

~~~text
PASS
~~~

需求：2026-05-24-decimal
