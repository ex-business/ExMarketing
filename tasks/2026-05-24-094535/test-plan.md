# Test Plan

## Scope

需求：`2026-05-24-094535` 预算库存次数原子扣减。

本测试计划在 `review.md` Decision 为 `PASS` 后设计，用于指导 Dev 开发后的验证。只覆盖测试设计，不修改业务代码。

核心验证目标：

- 活动预算 `BUDGET`、奖励库存 `STOCK`、用户参与次数 `USER_LIMIT` 在同一 activity 发奖事务内原子校验和扣减。
- 额度不足时不生成 `grant_order`，不生成 outbox，不留下 `REWARDABLE` 脏 participation。
- 同一幂等事件重复请求不重复扣减预算、库存、次数。
- 多奖励活动中任一 reward 失败时，全部 quota、grant、outbox 一起 rollback。
- PostgreSQL 条件更新在并发下不超扣。

## Test Strategy

测试分三层：

1. Engine / repository 行为测试
   - 使用内存仓储或测试仓储快速覆盖编排语义。
   - 重点断言 participation、grant_order、outbox、quota used_value、audit 的最终状态。

2. HTTP 行为测试
   - 通过事件接入 API 验证外部可见状态码、重复事件响应和错误表达。
   - quota 不足统一验证 `409 Conflict`，并检查错误 body 能区分 `BUDGET`、`STOCK`、`USER_LIMIT` 中至少一种失败原因。

3. PostgreSQL smoke / concurrency 测试
   - 使用真实 PostgreSQL schema 验证事务、唯一键、条件更新和 rollback。
   - 并发测试必须以 PostgreSQL 为准，不能只依赖内存仓储。

## Test Cases

### TC01 Happy Path: 单奖励正常发奖

前置数据：

- 活动启用，规则匹配。
- 配置一个 reward，`rewardCode=RWD_A`，奖励金额 `100`。
- quota：
  - `BUDGET + ACTIVITY + TOTAL` total 足够。
  - `STOCK + ACTIVITY + RWD_A` total 足够。
  - `USER_LIMIT + USER + <subjectID>` total 足够。

步骤：

- 发送一个新的幂等事件。

断言：

- 返回成功。
- 创建一条 `REWARDABLE` participation。
- 创建一条 `grant_order`。
- 创建一条 outbox。
- `BUDGET.used_value` 增加 `100`。
- `STOCK(RWD_A).used_value` 增加 `1`。
- `USER_LIMIT(subjectID).used_value` 增加 `1`。
- 不写 quota 失败审计。

### TC02 Happy Path: 缺少 quota 行按无限制处理

前置数据：

- 活动启用，规则匹配。
- 不创建 `BUDGET`、`STOCK` 或 `USER_LIMIT` quota 行。

步骤：

- 发送一个新的幂等事件。

断言：

- 返回成功。
- 创建 `REWARDABLE` participation、`grant_order`、outbox。
- 不因为 quota 行缺失返回 `409`。
- 不写 quota 失败审计。

### TC03 规则拒绝

前置数据：

- 活动启用，但事件不满足活动规则。
- 配置足够 quota。

步骤：

- 发送一个新的幂等事件。

断言：

- 返回成功或业务定义的 no reward 结果。
- 只写 rejected participation。
- 不扣 `BUDGET`、`STOCK`、`USER_LIMIT`。
- 不创建 `grant_order`。
- 不创建 outbox。
- 不写 quota 失败审计。

### TC04 预算不足

前置数据：

- 活动启用，规则匹配。
- reward 金额 `100`。
- `BUDGET + ACTIVITY + TOTAL` 剩余额度小于 `100`。
- `STOCK` 和 `USER_LIMIT` 均足够。

步骤：

- 发送一个新的幂等事件。

断言：

- HTTP 返回 `409 Conflict`。
- 错误 body 或内部错误原因能定位为 `BUDGET` 不足。
- 不创建 `grant_order`。
- 不创建 outbox。
- 不创建 `REWARDABLE` participation；也不能留下 rewardable 脏状态。
- `STOCK.used_value` 不变。
- `USER_LIMIT.used_value` 不变。
- 写入 quota 失败审计，包含 tenant、activityId、eventInboxId、subject、quotaType=`BUDGET`、reason。

### TC05 用户参与次数不足

前置数据：

- 活动启用，规则匹配。
- `USER_LIMIT + USER + <subjectID>` 已达到 total。
- `BUDGET` 和 `STOCK` 均足够。

步骤：

- 发送一个新的幂等事件。

断言：

- HTTP 返回 `409 Conflict`。
- 错误原因能定位为 `USER_LIMIT` 不足。
- 不创建 `grant_order`。
- 不创建 outbox。
- 不创建 `REWARDABLE` participation。
- `BUDGET.used_value` 不变。
- `STOCK.used_value` 不变。
- 写入 quota 失败审计，quotaType=`USER_LIMIT`。

### TC06 单奖励库存不足

前置数据：

- 活动启用，规则匹配。
- `STOCK + ACTIVITY + RWD_A` 已达到 total。
- `BUDGET` 和 `USER_LIMIT` 均足够。

步骤：

- 发送一个新的幂等事件。

断言：

- HTTP 返回 `409 Conflict`。
- 错误原因能定位为 `STOCK` 不足，scopeKey 为 `RWD_A`。
- 不创建 `grant_order`。
- 不创建 outbox。
- 不创建 `REWARDABLE` participation。
- `BUDGET.used_value` 不变。
- `USER_LIMIT.used_value` 不变。
- 写入 quota 失败审计，quotaType=`STOCK`。

### TC07 多奖励 all-or-nothing: 第二个 reward 库存不足

前置数据：

- 活动启用，规则匹配。
- 配置两个 reward：`RWD_A` 和 `RWD_B`。
- `BUDGET` 足够覆盖两个 reward 金额合计。
- `USER_LIMIT` 足够。
- `STOCK(RWD_A)` 足够。
- `STOCK(RWD_B)` 不足。

步骤：

- 发送一个新的幂等事件。

断言：

- HTTP 返回 `409 Conflict`。
- 不创建任何 `grant_order`。
- 不创建任何 outbox。
- 不创建 `REWARDABLE` participation。
- `BUDGET.used_value` 不变。
- `USER_LIMIT.used_value` 不变。
- `STOCK(RWD_A).used_value` 不变，证明第一个 reward 的库存扣减 rollback。
- `STOCK(RWD_B).used_value` 不变。
- 写入 quota 失败审计，quotaType=`STOCK`，scopeKey=`RWD_B`。

### TC08 重复幂等事件: 成功后重复

前置数据：

- 活动启用，规则匹配。
- quota 足够。
- 使用同一个 tenant、biz、idempotency key 或外部事件 ID。

步骤：

- 第一次发送事件。
- 第二次发送完全相同幂等事件。

断言：

- 第一次成功。
- 第二次返回 duplicate 或等价的幂等响应。
- participation 数量不增加。
- `grant_order` 数量不增加。
- outbox 数量不增加。
- `BUDGET.used_value`、`STOCK.used_value`、`USER_LIMIT.used_value` 在第二次后不再增加。

### TC09 重复幂等事件: 额度不足失败后重复

前置数据：

- 活动启用，规则匹配。
- `BUDGET` 不足。
- 使用同一个幂等事件。

步骤：

- 第一次发送事件。
- 第二次发送相同幂等事件。

断言：

- 第一次返回 `409 Conflict`。
- 第二次按 inbox 幂等策略返回 duplicate 或同一失败结果，必须不触发新的 quota 扣减。
- 不创建 `grant_order`。
- 不创建 outbox。
- 不创建 `REWARDABLE` participation。
- quota used_value 均不变。
- 审计记录数量符合实现约定：允许只记录首次失败；若重复请求也记录，必须能通过 eventInboxId 或 duplicate 标识区分，不能伪造成新的发奖失败。

### TC10 PostgreSQL 并发: 预算只剩 1 份

前置数据：

- 使用 PostgreSQL。
- 活动启用，规则匹配。
- reward 金额 `100`。
- `BUDGET` total 只允许一次成功。
- `STOCK` 和 `USER_LIMIT` 足够。
- 准备两个不同幂等事件，subject 可相同或不同。

步骤：

- 并发提交两个事件。

断言：

- 最终只能一个成功，另一个返回 `409 Conflict`。
- `BUDGET.used_value` 不超过 total。
- 只创建成功事件对应的 `grant_order` 和 outbox。
- 失败事件不留下 `REWARDABLE` participation。
- 有且只有失败路径对应的 quota 失败审计。

### TC11 PostgreSQL 并发: 库存只剩 1 份

前置数据：

- 使用 PostgreSQL。
- 活动启用，规则匹配。
- `STOCK + ACTIVITY + RWD_A` total 只允许一次成功。
- `BUDGET` 和 `USER_LIMIT` 足够。
- 准备两个不同幂等事件。

步骤：

- 并发提交两个事件。

断言：

- 最终只能一个成功。
- `STOCK(RWD_A).used_value` 不超过 total。
- `BUDGET.used_value` 只增加一次成功事件的奖励金额。
- `USER_LIMIT.used_value` 只增加一次。
- 只创建一组 `grant_order` 和 outbox。
- 失败事件不留下 `REWARDABLE` participation。

### TC12 PostgreSQL 并发: 同一幂等事件双提交

前置数据：

- 使用 PostgreSQL。
- 活动启用，规则匹配。
- quota 足够。
- 两个 goroutine 使用完全相同的幂等事件。

步骤：

- 并发提交相同事件。

断言：

- 最终最多一个进入发奖事务并成功。
- 另一个返回 duplicate 或唯一键幂等结果。
- quota 只扣一次。
- `grant_order` 和 outbox 只创建一次。
- 无脏 `REWARDABLE` participation。

## Required Commands

开发完成后最低必须执行：

~~~bash
go test ./...
go build ./...
~~~

## PostgreSQL Smoke

必须使用真实 PostgreSQL 执行 smoke：

~~~bash
docker compose up -d postgres
docker compose exec -T postgres psql -U exmarketing -d exmarketing -f /docker-entrypoint-initdb.d/001_marketing_core.sql
docker compose exec -T postgres psql -U exmarketing -d exmarketing -f /docker-entrypoint-initdb.d/002_audit_fact_outbox_indexes.sql
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./...
~~~

PostgreSQL smoke 必须至少验证：

- 正常路径能落库并写 outbox。
- `BUDGET` 不足不会生成 `grant_order` / outbox / `REWARDABLE` participation。
- `STOCK` 不足不会扣其他 quota。
- 多 reward rollback 后所有 quota used_value 保持不变。
- 并发条件更新不会让 used_value 超过 total_value。

## Test Data Notes

- `STOCK` scopeKey 必须固定使用实现约定的 reward 标识。根据 review 建议，优先使用运营可感知的 `rewardCode`，例如 `RWD_A`、`RWD_B`。
- quota 行不存在时代表无限制，应单独覆盖，避免新增 STOCK 扣减破坏历史活动。
- 失败审计建议使用独立查询断言 action、quotaType、scope、scopeKey、subject、eventInboxId、reason。
- 所有失败测试都必须显式查询 participation，确保不存在 `status=REWARDABLE` 的脏记录。

## Decision

~~~text
PASS
~~~
