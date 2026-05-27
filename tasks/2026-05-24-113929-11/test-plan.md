# Test Plan

## Agent 指令

你是 Test Plan Agent。请在 Review PASS 后、Dev 开发前，根据 task.md、analysis.md、review.md 设计测试用例。

只写 test-plan.md，不改业务代码，不执行提交。

## Test Strategy

- 以 `review.md` 的 `Decision PASS` 为前置条件，本计划允许进入 Dev；后续 Test Agent 只在 Dev 完成后执行。
- 测试范围聚焦 `FIRST_DEPOSIT` 插件的可选增强语义：插件只构建 `state.plugin.FIRST_DEPOSIT.*` 上下文和预检查解释，不直接发奖、不直接扣 quota、不绕过 participation / grant_order / outbox / callback。
- 分层覆盖：
  - 插件单元测试：字段归一化、非法输入、插件与活动类型兼容性。
  - 活动配置/HTTP 测试：创建活动、非法插件、非法绑定、模拟运行和正式事件入站。
  - 引擎/仓储测试：正常发奖链路、额度不足、幂等重复、同用户不同充值单 `USER_LIMIT=1`、并发竞争。
  - PostgreSQL smoke：使用 Docker PostgreSQL 验证事务、唯一键、quota 原子更新和无脏 grant/outbox。
- 必须同时验证“绑定 `pluginCode=FIRST_DEPOSIT`”与“不绑定插件仍可通过 `attributes.*` 配置首存活动”的兼容边界。
- 对 review 的 P2 风险做定向覆盖：`USER_LIMIT=1` 阻止同一用户多个不同 `bizId` 的重复首存奖励；未配置 `USER_LIMIT` 时测试应记录行为为可审计风险而非插件绕过。

## Test Cases

### 1. 插件上下文正常路径

- 构造标准事件：
  - `eventType=DEPOSIT_SUCCESS`
  - `sourceSystem=payment`
  - `biz.bizType=DEPOSIT`
  - `biz.bizId=pay_1001`
  - `subject.type=USER`
  - `subject.id=user_001`
  - `attributes.amount=299.00`
  - `attributes.currency=CNY`
  - `attributes.isFirstDeposit=true`
  - `attributes.paymentMethod=BANK_CARD`
- 断言绑定 `pluginCode=FIRST_DEPOSIT` 的活动在规则上下文中暴露：
  - `state.plugin.FIRST_DEPOSIT.amount=299.00`
  - `state.plugin.FIRST_DEPOSIT.currency=CNY`
  - `state.plugin.FIRST_DEPOSIT.isFirstDeposit=true`
  - `state.plugin.FIRST_DEPOSIT.paymentMethod=BANK_CARD`
  - `state.plugin.FIRST_DEPOSIT.depositBizId=pay_1001`
- 断言字段来源只来自事件 attributes/biz，不查询外部存款表、不写插件私有状态。

### 2. 活动配置与兼容性

- 可以创建 `activityType=FIRST_DEPOSIT`、`pluginCode=FIRST_DEPOSIT`、`triggerEvents=["DEPOSIT_SUCCESS"]` 的活动。
- 可以创建不绑定 `pluginCode` 的 `FIRST_DEPOSIT` 活动，并通过 `attributes.isFirstDeposit=true`、`attributes.amount >= 100.00` 正常命中。
- 未知插件应创建或上线校验失败。
- 非 `FIRST_DEPOSIT` 活动绑定 `pluginCode=FIRST_DEPOSIT` 应失败。
- `FIRST_DEPOSIT` 活动配置非法字段路径、非法 Decimal、金额类奖励缺失 `currency`、非法 `targetRef` 应失败。
- 首存活动配置 `RATIO` 奖励时，按 `attributes.amount * rate` 计算并受 `maxAmount` 封顶，结果保持 Decimal 4 位语义。

### 3. 模拟运行

- 合法首存事件模拟运行应命中活动，输出插件上下文、插件预检查通过、规则通过、奖励预览、quota 影响预览、grant/outbox 预览。
- 模拟运行不得写入 `marketing_event_inbox`、`marketing_participation`、`marketing_grant_order`、`marketing_quota_reservation`、`marketing_outbox` 或真实 quota 聚合。
- `isFirstDeposit=false` 应拒绝，返回可识别的非首存原因，不展示可执行 grant/outbox。
- 缺少 `amount`、金额为 0、金额为负数、金额不可解析、缺少 `currency`、`isFirstDeposit` 缺失或非 boolean 均应拒绝，并返回可识别原因。
- `paymentMethod` 缺失时，未配置支付方式限制的活动可继续；配置支付方式限制的活动应拒绝。

### 4. 正常正式链路

- 创建活动：`FIRST_DEPOSIT` + `pluginCode=FIRST_DEPOSIT` + `USER_LIMIT=1` + 可选 `BUDGET` / `STOCK` + `RATIO` 奖励 + 有效 `targetRef`。
- 上报首次合法 `DEPOSIT_SUCCESS` 事件，断言核心链路完成：
  - inbox 记录创建。
  - participation 创建且可追踪活动、用户、事件快照。
  - quota reservation 创建或聚合冻结正确。
  - grant_order 创建，状态为待发奖类状态。
  - outbox 创建，payload 中是发奖 command，不是插件直接发钱。
  - audit 中能看到活动命中、规则/插件解释和发奖链路。
- 断言插件没有直接修改余额、没有跳过 grant_order/outbox、没有生成外部发奖结果。

### 5. 额度不足

- BUDGET 总额小于本次奖励金额时，上报合法首存事件应被 quota 拒绝。
- STOCK 为 0 或不足时，上报合法首存事件应被 quota 拒绝。
- 拒绝后不得创建 rewardable grant_order/outbox，不得留下脏 quota reservation；如有 participation，应为拒绝态并包含明确原因。
- 同一事务内多奖励场景：任一奖励额度不足时，所有奖励的 quota/grant/outbox 都不得部分成功。

### 6. 幂等重复

- 重复提交完全相同 idempotency key 与相同 payload：
  - 返回 duplicate 或等价幂等响应。
  - 不新增 participation、grant_order、outbox。
  - 不重复冻结或扣减 quota。
- 复用相同 idempotency key 但 payload 不同：
  - 返回 `INVALID_IDEMPOTENCY_KEY` 或等价错误。
  - 不产生新的业务记录和 quota 变化。
- 同一用户第二个不同 `bizId` 且仍为 `isFirstDeposit=true`：
  - 配置 `USER_LIMIT=1` 时应被用户次数限制拒绝。
  - 未配置 `USER_LIMIT` 的兼容测试应证明行为可审计，并确认这不是插件绕过幂等或 quota 的结果。

### 7. 并发

- 并发提交同一 idempotency key 的合法首存事件，最终只允许一个请求创建参与和发奖链路，其余为 duplicate 或冲突响应；quota 只移动一次。
- 并发提交同一用户、不同 `bizId`、均 `isFirstDeposit=true` 的事件，且活动配置 `USER_LIMIT=1`，最终只允许一个事件成功，其他事件因用户次数限制失败。
- 并发提交多个不同用户合法首存事件，BUDGET/STOCK 接近耗尽时不得超额扣减；成功数量与 quota 聚合一致，失败请求无脏 grant/outbox。
- PostgreSQL 仓储并发测试必须验证条件更新/事务锁语义；内存仓储测试只作为快速回归，不代表最终事务验收。

### 8. PostgreSQL Smoke

- 启动 Docker PostgreSQL 后执行迁移初始化，使用 `EX_MARKETING_DATABASE_URL` 运行相关测试。
- 通过 HTTP 或仓储集成路径完成：
  - 创建 `FIRST_DEPOSIT` 活动和 quota。
  - 上报合法首存事件并查询表计数/状态。
  - 重复上报相同事件，确认唯一幂等语义。
  - 上报同用户不同 `bizId`，确认 `USER_LIMIT=1` 生效。
  - 构造 quota 不足，确认无脏 grant/outbox。
- 需要检查的表至少包括：
  - `marketing_activity`
  - `marketing_event_inbox`
  - `marketing_participation`
  - `marketing_quota`
  - `marketing_quota_reservation`
  - `marketing_grant_order`
  - `marketing_outbox`
  - `marketing_audit`

## Required Commands

~~~bash
go test ./internal/plugin ./internal/engine ./internal/httpapi ./internal/marketing ./internal/postgres -run 'Test.*FirstDeposit|Test.*Plugin|Test.*Quota|Test.*Idempot|Test.*Concurrent|TestPostgres.*' -count=1
go test ./...
go build ./...

docker compose up -d postgres
export EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable'
go test ./internal/postgres ./internal/httpapi ./internal/engine -run 'TestPostgres|Test.*FirstDeposit|Test.*Quota|Test.*Idempot|Test.*Concurrent' -count=1
~~~

## Data / Smoke

- 标准租户：`site_001`。
- 标准活动：
  - `activityCode=FIRST_DEPOSIT_BONUS`
  - `activityType=FIRST_DEPOSIT`
  - `pluginCode=FIRST_DEPOSIT`
  - `triggerEvents=["DEPOSIT_SUCCESS"]`
  - 规则：`attributes.isFirstDeposit == true` 且 `attributes.amount >= 100.00`
  - 奖励：`rewardType=BONUS`，`calculateType=RATIO`，`baseField=attributes.amount`，`rate=0.10`，`maxAmount=50.00`，`currency=CNY`，`grantMode=AUTO`，有效 `targetRef`
  - Quota：`USER_LIMIT=1`，并至少一组测试配置 `BUDGET` / `STOCK`
- 正常事件：
  - idempotency key：`site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_1001`
  - `biz.bizId=pay_1001`
  - `subject.id=user_001`
  - `attributes.amount=299.00`
  - `attributes.currency=CNY`
  - `attributes.isFirstDeposit=true`
- 重复事件：完全复用 `pay_1001` 和同一 idempotency key。
- 用户次数事件：`subject.id=user_001`，`biz.bizId=pay_1002`，idempotency key `site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_1002`，仍标记 `isFirstDeposit=true`。
- 非首存事件：`attributes.isFirstDeposit=false`。
- 非法金额事件：分别覆盖缺失、`0`、`-1.00`、`abc`。
- 缺币种事件：移除 `attributes.currency`。
- 额度不足事件：活动预算或库存设置为小于预期奖励/可用次数。
- PostgreSQL smoke 必须记录关键 SQL 查询结果或测试断言结果，证明真实 PG 路径下没有重复 participation/grant/outbox，没有 quota 超扣，没有模拟运行写入真实业务表。

## Decision

~~~text
PASS
~~~

需求：2026-05-24-113929-11
