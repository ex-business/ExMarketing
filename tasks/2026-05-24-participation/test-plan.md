# Test Plan

## Scope

需求：2026-05-24-participation

前置条件：`review.md` Decision 为 `PASS`。

本计划供后续 Test Agent 在 Dev Agent 完成实现后执行。Test Agent 可新增或调整测试代码、测试 fixture 和测试脚本；本 Test Plan Agent 本轮不执行任何测试、构建、格式化或 git 命令。

## Test Strategy

覆盖三层：

1. 单元/仓储语义：验证 participation 状态、幂等键、snapshot 最小字段、grant order/outbox 与 participation 的关联关系。
2. API/业务流程：通过标准事件入口或现有 handler/service 测试正常发奖、规则拒绝、零奖励、非金额奖励、重复事件、同 key 不同 payload、模拟运行无副作用。
3. PostgreSQL smoke：使用真实 PostgreSQL 事务验证成功发奖、规则拒绝、零奖励、重复事件、同 key 不同 payload、quota 不足、并发重复事件。

重点断言：

- 任何 grant order/outbox 不得绕过 participation。
- `REWARDABLE`、quota 扣减、grant order、outbox 必须活动级事务一致。
- `REJECTED` 和 `QUALIFIED` 必须可追溯，且不得生成 grant order/outbox 或扣减 quota。
- participation 幂等键必须是 `tenantId + ":" + activityId + ":" + eventInbox.idempotency_key`。
- 完全重复事件不新增 participation、grant order、outbox。
- 同 `Idempotency-Key` 不同 payload 返回 `INVALID_IDEMPOTENCY_KEY`，不新增 participation。
- quota 不足返回 `QUOTA_EXCEEDED`，不保留 `REWARDABLE` participation、grant order 或 outbox。
- Webhook Receiver 缺省 `Idempotency-Key` 时，应由映射后的标准事件派生幂等键，并仍走相同 participation 链路。

## Test Cases

### TC-01 正常路径：金额奖励发奖

准备一个 enabled 活动、enabled MATCH/QUALIFY/REWARD 规则、正金额奖励、充足 quota。

步骤：

- 上报满足规则的标准事件，Header `Idempotency-Key` 等于 `tenantId:sourceSystem:eventType:bizType:bizId`。
- 查询 `marketing_event_inbox`、`marketing_participation`、`marketing_grant_order`、`marketing_outbox`、quota 余额。

断言：

- 事件处理成功。
- 仅生成 1 条 `REWARDABLE` participation。
- participation `idempotency_key` 为 `tenantId:activityId:eventInbox.idempotency_key`。
- `snapshot_json.activity`、`snapshot_json.event`、`snapshot_json.ruleResult`、`snapshot_json.rewards` 存在且包含本次活动、事件、规则和奖励摘要。
- grant order 关联 participation，且 outbox 关联 grant order 或能追踪到同一发奖链路。
- quota 按奖励金额或配置扣减。
- 不存在没有 participation 的 grant order/outbox。

### TC-02 正常路径：非金额奖励金额为 0 但权益可发

准备一个非金额权益奖励，例如 coupon/points/right，`reward_amount = 0.0000`，但权益数量或权益引用有效。

断言：

- participation 状态为 `REWARDABLE`。
- grant order/outbox 正常生成。
- snapshot/reward payload 记录权益数量、权益类型或权益引用。
- 不按零金额奖励误判为 `QUALIFIED`。

### TC-03 规则拒绝：REJECTED 可追溯

准备一个活动，使事件进入候选活动但 QUALIFY 或业务规则不通过。

断言：

- 生成 1 条 `REJECTED` participation。
- `reject_reason` 非空，原因码能解释拒绝。
- `snapshot_json.ruleResult` 包含失败规则、阶段、字段、期望值、实际值或可解释原因。
- 不生成 grant order/outbox。
- 不扣减 quota。

### TC-04 零奖励：QUALIFIED 不发奖

准备规则全部通过，但所有奖励金额为 `0.0000` 且无非金额权益。

断言：

- 生成 1 条 `QUALIFIED` participation。
- `reject_reason` 为或包含 `ZERO_REWARD_SKIPPED: no payable reward`。
- snapshot 中 rewards 说明奖励被跳过。
- 不生成 grant order/outbox。
- 不扣减 quota。

### TC-05 额度不足：quota 不足回滚

准备规则通过、奖励可发，但活动 quota 小于本次奖励所需额度。

断言：

- API 或 service 返回 `QUOTA_EXCEEDED`。
- 不保留 `REWARDABLE` participation。
- 不生成 grant order/outbox。
- 不扣减 quota。
- 存在可审计 quota 拒绝信息。若系统已有 audit 表或日志捕获测试能力，断言原因码；否则在 PostgreSQL smoke 里至少断言业务表无脏写。

### TC-06 幂等重复：完全相同事件重放

步骤：

- 先执行 TC-01 正常发奖事件。
- 使用相同 `Idempotency-Key` 和完全相同 payload 再次上报。
- 可重复上报 3 次，覆盖 duplicate 语义稳定性。

断言：

- 重放返回 duplicate 或现有约定的幂等成功语义。
- participation 仍只有 1 条。
- grant order/outbox 仍只有第一轮生成的记录。
- quota 只扣减一次。

### TC-07 幂等冲突：同 key 不同 payload

步骤：

- 先执行一条合法事件。
- 使用相同 `Idempotency-Key`，修改 payload 中 `attributes`、`subject` 或 `biz` 的任一业务字段后再次上报。

断言：

- 返回 `INVALID_IDEMPOTENCY_KEY`。
- 不新增 participation。
- 不新增 grant order/outbox。
- 不新增 quota 扣减。
- 不覆盖第一条 inbox 或 participation snapshot。

### TC-08 并发：同一事件并发重复上报

步骤：

- 并发 20 到 50 个 goroutine 或 HTTP client，同时提交相同 `Idempotency-Key` 和相同 payload。
- PostgreSQL 版本必须执行；内存仓储可作为补充。

断言：

- 所有请求最终为 1 个成功处理结果，其余为 duplicate 或幂等成功语义。
- `marketing_participation` 对同一 participation idempotency key 只有 1 条。
- grant order/outbox 只有 1 组。
- quota 只扣减一次。
- 无 unique constraint panic、死锁泄漏或部分写入。

### TC-09 并发：quota 临界竞争

准备活动 quota 只够 1 次发奖，使用不同 `Idempotency-Key` 的两个可发奖事件并发上报。

断言：

- 只有 1 个事件发奖成功并生成 `REWARDABLE` participation、grant order、outbox。
- 另一个事件返回 `QUOTA_EXCEEDED`。
- 失败事件不保留 `REWARDABLE` participation、grant order/outbox。
- quota 最终不为负数。

### TC-10 Webhook 缺省 Header 幂等

通过 Webhook Receiver 输入一条可映射为标准事件的 payload，不提供 `Idempotency-Key` Header。

断言：

- 系统由映射后的标准事件派生 `tenantId:sourceSystem:eventType:bizType:bizId`。
- 仍生成正确 participation 幂等键。
- 重放同一 Webhook payload 不新增 participation、grant order/outbox。
- 若提供 Header，则 Header 必须与派生 key 匹配；不匹配时拒绝且不新增 participation。

### TC-11 模拟运行无副作用

调用 `POST /admin/v1/simulate` 或对应 simulate service，使用会命中活动且可发奖的事件。

断言：

- 返回 preview 或模拟结果。
- 不写 `marketing_event_inbox`。
- 不写 `marketing_participation`。
- 不写 `marketing_grant_order`。
- 不写 `marketing_outbox`。
- 不扣减 quota。

### TC-12 历史 snapshot 不随配置变更

步骤：

- 执行 TC-01 生成 participation。
- 修改活动或奖励配置版本、奖励金额、规则阈值。
- 再读取历史 participation snapshot。

断言：

- 历史 `snapshot_json` 仍保留参与当时的 activity/rule/reward 摘要。
- 新配置不回写或污染历史 snapshot。

## Required Commands

后续 Test Agent 应根据项目现有测试约定执行以下命令。若仓库已有 Makefile、docker compose 或集成测试脚本，应优先使用仓库约定命令，并保留等价覆盖。

~~~bash
go test ./...
go build ./...
~~~

建议 PostgreSQL smoke 命令清单：

~~~bash
# 仅当仓库使用 docker compose 管理 PostgreSQL 时执行等价环境启动命令
docker compose up -d postgres

# 执行包含 PostgreSQL tag/profile 的集成测试；具体 tag 名称以实现为准
go test ./... -run 'Participation|Postgres|PostgreSQL' -count=1

# 若实现使用 build tag 区分 PostgreSQL 集成测试，执行等价命令
go test ./... -tags=integration -run 'Participation|Postgres|PostgreSQL' -count=1
~~~

建议并发/幂等专项命令清单：

~~~bash
go test ./... -run 'Participation.*Idempot|Idempot.*Participation|Participation.*Concurrent|Concurrent.*Participation|Quota.*Concurrent' -count=10
~~~

建议 race 检查命令清单：

~~~bash
go test ./... -race -run 'Participation|Idempot|Quota|Postgres|PostgreSQL' -count=1
~~~

## Data / Smoke

PostgreSQL smoke 最小数据集：

- tenant：`tenant_participation_smoke`
- sourceSystem：`test-source`
- eventType：`ORDER_PAID` 或仓库已有标准事件类型
- bizType：`ORDER`
- subject：`USER:user_001`
- 正常订单 bizId：`order_rewardable_001`
- 规则拒绝 bizId：`order_rejected_001`
- 零奖励 bizId：`order_zero_reward_001`
- 重复事件 bizId：`order_duplicate_001`
- 同 key 不同 payload bizId：`order_conflict_001`
- quota 不足 bizId：`order_quota_exceeded_001`
- 并发重复 bizId：`order_concurrent_duplicate_001`
- 并发 quota bizId：`order_concurrent_quota_a`、`order_concurrent_quota_b`

PostgreSQL smoke 必须覆盖：

- 成功发奖：`REWARDABLE` + grant order + outbox + quota 扣减。
- 规则拒绝：`REJECTED`，无 grant order/outbox，无 quota 扣减。
- 零奖励：`QUALIFIED`，无 grant order/outbox，无 quota 扣减。
- 重复事件：完全相同 key/payload 重放不新增业务记录。
- 同 key 不同 payload：`INVALID_IDEMPOTENCY_KEY`，不新增 participation。
- quota 不足：`QUOTA_EXCEEDED`，不保留 `REWARDABLE` participation、grant order/outbox。
- 并发重复：同 key 并发只有 1 条 participation 和 1 次发奖。
- 并发 quota：quota 临界竞争不会超扣或负数。

清理要求：

- 测试数据应使用独立 tenant、activityCode、rewardCode 和 bizId 前缀，避免污染开发数据。
- 每个 PostgreSQL smoke 用例应在事务回滚、测试 schema 清理或显式 delete 清理中选择一种稳定方式。
- 若使用 docker compose PostgreSQL，Test Agent 应在测试结束后按仓库约定停止或清理临时容器。

## Decision

~~~text
PASS
~~~
