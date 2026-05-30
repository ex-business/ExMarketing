# Test Plan

## Agent 指令

你是 Test Plan Agent。请在 Review PASS 后根据 task.md、analysis.md、review.md 设计测试用例。

只允许写 test-plan.md。
禁止修改业务代码、测试代码、文档代码、配置文件或任何其他文件。
禁止执行测试命令、构建命令、格式化命令或 git 命令。
你的职责只是写清楚后续 Test Agent 应执行的测试用例和命令清单。

## Test Strategy

本测试计划面向后续 Test Agent 执行。当前 Test Plan Agent 不执行任何测试、构建、格式化或 git 命令。

测试分三层：

1. Schema / validator 单元测试：覆盖规则、奖励、quota 的合法枚举、字段路径、Decimal、概率、阶梯和 `rewardType × calculateType` 兼容矩阵。
2. HTTP / handler 集成测试：覆盖活动保存、quota 创建、模拟运行、后端错误路径返回和幂等/重复请求语义。
3. PostgreSQL smoke：在真实或测试库中验证保存后的 `marketing_rule`、`marketing_reward`、`marketing_quota` 持久化形态、quota 消耗边界、并发扣减一致性。

重点风险：

- 前端/后端普通表单与 JSON 高级模式校验口径不一致。
- 非金额奖励错误开放 `RATIO / LADDER / RANDOM / PROBABILITY / POOL_SPLIT`。
- quota 不足、重复模拟/重复事件、并发参与时出现超发、重复发奖或数据库状态漂移。
- PostgreSQL Decimal / JSONB 持久化后类型或精度变化，导致引擎不可执行。

## Test Cases

### 1. 正常路径：首存 BONUS RATIO + 预算 + 用户限制

前置数据：

- 活动事件类型：`DEPOSIT_SUCCESS`。
- 规则：`MATCH` 或 `QUALIFY` 中配置 `attributes.isFirstDeposit = true`、`attributes.amount >= 100.00`。
- 奖励：`BONUS + RATIO`，`baseField=attributes.amount`，`rate=0.20`，`maxAmount=500.00`，`currency=CNY`，`flowMultiple=3`，`grantMode=AUTO`，`grantChannel=HTTP`，合法 `targetRef`。
- Quota：`BUDGET / ACTIVITY / TOTAL / 10000.00`，`USER_LIMIT / USER / subject.id / 1`。

步骤：

- 通过活动保存接口或 handler 测试保存上述配置。
- 调用模拟运行接口，事件金额使用 `1000.00`。
- 查询返回的规则解释、奖励解释、quota 解释和预览 JSON。

期望：

- 保存成功。
- 模拟结果为通过，奖励金额为 `200.0000 CNY`，流水倍数为 `3`。
- 配置预览包含 `rules[]`、`rewards[]` 和 quota 摘要。
- PostgreSQL 中 `marketing_rule.rule_config`、`marketing_reward.reward_config`、`marketing_quota.total_value` 与输入一致，金额精度不丢失。

### 2. 正常路径：签到 BONUS LADDER + DAILY_LIMIT

前置数据：

- 活动事件类型：`SIGN_IN_SUCCESS`。
- 规则：`attributes.continueDays >= 1`。
- 奖励：`BONUS + LADDER`，按 `attributes.continueDays` 分层，例如 `1-3 => 1.00`、`4-7 => 3.00`、`8-30 => 5.00`，`currency=CNY`。
- Quota：`DAILY_LIMIT / USER / subject.id:activityLocalDate / 1`。

步骤：

- 保存合法配置。
- 用 `continueDays=5` 模拟。
- 同一用户同一活动日再次模拟或执行参与链路。

期望：

- 首次模拟命中第二档，金额为 `3.0000`。
- 第二次在真实执行链路中应被 `DAILY_LIMIT` 拒绝；如果模拟不扣 quota，则模拟响应必须明确为预览且不修改 quota。
- 阶梯区间摘要和 JSON path 正确。

### 3. 正常路径：返水 CASH POOL_SPLIT / PROBABILITY 可保存合法金额类配置

前置数据：

- 活动事件类型：`REBATE_SETTLE` 或现有返水标准事件。
- 奖励类型使用金额类 `CASH`。
- 分别构造合法 `PROBABILITY` 和 `POOL_SPLIT` 配置，均包含 `currency`，且 `flowMultiple=0` 或不传。

步骤：

- 参数化保存 `CASH + PROBABILITY`、`CASH + POOL_SPLIT`。
- 对每个配置运行模拟。

期望：

- 两种金额类组合均可保存。
- `CASH` 不允许正数 `flowMultiple`；正数流水倍数应返回校验错误。
- 概率权重总和大于 0，池分摊摘要明确为事件即时分摊。

### 4. 非金额奖励兼容矩阵拒绝

参数化非法样例：

- `POINT + RATIO`
- `COUPON + LADDER`
- `GIFT + RANDOM`
- `DRAW_CHANCE + PROBABILITY`
- `MEMBERSHIP + POOL_SPLIT`

步骤：

- 分别通过普通保存 payload 和 JSON 高级模式 payload 提交非法组合。
- 若存在前端组件测试，覆盖普通表单选项不会展示非法 `calculateType`。

期望：

- 所有非法组合均被拒绝。
- 错误路径定位到 `rewards[n].calculateType` 或等价 JSON path。
- 错误信息表达“该 rewardType 当前只支持 FIXED”。
- 后端不得落库非法 `marketing_reward`。

### 5. 奖励字段边界与非法配置

用例：

- `POINT + FIXED` 缺少 `points`，应拒绝。
- `POINT + FIXED` 带 `currency`，应拒绝。
- `COUPON + FIXED` 缺少 `couponTemplateId` 或 `quantity`，应拒绝。
- `GIFT + FIXED` 缺少 `giftSkuId/giftCode` 或 `quantity`，应拒绝。
- `DRAW_CHANCE + FIXED` 缺少 `drawPoolCode/lotteryCode` 或 `chanceCount`，应拒绝。
- `MEMBERSHIP + FIXED` 缺少会员权益引用且无永久或天数配置，应拒绝。
- `HTTP / WEBHOOK targetRef` 非 http(s) URL，应拒绝。
- `MQ destination` 包含凭据、query 或 fragment，应拒绝。

期望：

- 每个错误均有明确字段路径。
- 不可解析 Decimal、负数金额、零权重、概率超界均不能被自动转为 `0` 或 float。

### 6. 规则编辑器合法与非法路径

合法样例：

- `attributes.amount >= "100.00"`
- `facts.vip.level IN ["GOLD","PLATINUM"]`
- `state.activity.joinCount < 10`

非法样例：

- `rulePhase=BUDGET`
- `ruleType=SCRIPT` 且后端无可执行 evaluator
- `field=orderAmount`
- `field=deposit.amount`
- `IN` 的 value 不是非空数组
- `conditions=[]`

期望：

- 合法 `CONDITION` 保存成功并可模拟解释。
- 非法字段路径和枚举被拒绝，错误路径定位到 `rules[n].ruleConfig.conditions[m].field` 或对应字段。
- `enabled=false` 规则仍需通过 Schema 校验，但模拟解释中显示跳过或禁用状态。

### 7. JSON 高级模式一致性

步骤：

- 用普通表单创建包含规则、奖励、quota 的配置。
- 切换到 JSON 模式，增加一个普通表单不展示但后端允许的合法字段。
- 格式化 JSON 后切回普通表单，再切回 JSON。
- 提交保存。
- 再构造 JSON 语法错误、未知字段、必填字段删除、类型改变等非法 payload。

期望：

- 合法高级字段不丢失、不被改类型、不被清空。
- JSON 格式化不改变语义。
- 语法错误显示行列；Schema 错误显示 JSON path。
- 未知字段或必填缺失不能保存。

### 8. Quota 正常、额度不足与无限制

正常样例：

- `BUDGET / ACTIVITY / TOTAL / 100.00`
- `STOCK / ACTIVITY / rewardCode / 2`
- `USER_LIMIT / USER / subject.id / 1`
- `DAILY_LIMIT / USER / subject.id:activityLocalDate / 1`

额度不足样例：

- 预算 `totalValue=100.00`，单次奖励需要 `120.00`。
- 库存 `totalValue=1`，同一 reward 真实执行两次。
- 用户限制 `totalValue=1`，同一用户重复真实参与。

无限制样例：

- 不创建 quota 行。

期望：

- 正常 quota 可保存，运行时扣减符合预算金额、库存 1 次、用户次数 1 次。
- 额度不足时真实执行链路拒绝发奖，不生成 grant/outbox 或不得生成成功状态的 grant/outbox。
- 不足响应包含 quota 类型、scope、scopeKey 和剩余额度信息或等价诊断。
- 缺少 quota 行表示“不限制”，不得自动创建 `totalValue=0`。
- 运营不可直接编辑 `used_value`、`frozen_value`、`version`。

### 9. 幂等重复

场景：

- 使用同一标准事件 `eventId`、同一 `subject.id`、同一活动，连续提交两次真实参与或执行接口。
- 使用相同 idempotency key 或系统既有去重字段重复请求。

期望：

- 只产生一次 reward/grant/outbox/quota 消耗。
- 第二次返回已处理、重复或同一结果，不应再次扣 quota。
- PostgreSQL 中相关参与记录、grant 记录、outbox 记录、quota `used_value/frozen_value` 无重复增长。

### 10. 并发

场景 A：库存并发。

- `STOCK totalValue=1`。
- 两个不同用户同时触发同一 reward。

期望 A：

- 最多一个成功发奖并扣减库存。
- 另一个明确失败为库存不足或 quota 不足。
- 数据库最终 `used_value <= total_value`，无负数可用量。

场景 B：预算并发。

- `BUDGET totalValue=100.00`。
- 两个用户同时触发各 `80.00` 奖励。

期望 B：

- 最多一个成功，不能超发到 `160.00`。
- 失败请求不生成成功 grant/outbox。

场景 C：用户限制并发。

- 同一用户并发触发两次，`USER_LIMIT totalValue=1`。

期望 C：

- 最多一次成功。
- 幂等与并发保护同时成立。

### 11. PostgreSQL Smoke

前置：

- 使用测试数据库，不连接生产库。
- 记录测试活动 ID、rewardCode、subject.id、eventId，确保可清理。

检查点：

- `marketing_rule`：`rule_phase`、`rule_type`、`rule_config` JSONB 可读，conditions 字段路径和 Decimal 字符串保持原样。
- `marketing_reward`：`reward_type`、`calculate_type`、`reward_config` JSONB 与合法矩阵一致，不存在非法非金额奖励非 `FIXED` 数据。
- `marketing_quota`：`quota_type`、`scope`、`scope_key`、`total_value` 正确，`used_value/frozen_value/version` 未被配置接口直接覆盖。
- 真实执行或集成测试后，quota 消耗不超过 `total_value`。
- 幂等重复后 grant/outbox/quota 消耗只增长一次。
- 并发测试后无超发、无负可用量、无重复 outbox 成功记录。

## Required Commands

~~~bash
# Unit / integration / repository tests
go test ./...

# Build verification
go build ./...

# Optional targeted package runs after Test Agent identifies packages
go test ./path/to/admin/package -run 'Test.*Rule|Test.*Reward|Test.*Quota' -count=1
go test ./path/to/engine/package -run 'Test.*Quota|Test.*Idempot|Test.*Concurrent' -count=1

# PostgreSQL smoke, replace env vars with the test database values only
psql "$TEST_DATABASE_URL" -v ON_ERROR_STOP=1 -c "select count(*) from marketing_rule where activity_id = :'activity_id';"
psql "$TEST_DATABASE_URL" -v ON_ERROR_STOP=1 -c "select reward_type, calculate_type, reward_config from marketing_reward where activity_id = :'activity_id' order by reward_code;"
psql "$TEST_DATABASE_URL" -v ON_ERROR_STOP=1 -c "select quota_type, scope, scope_key, total_value, used_value, frozen_value from marketing_quota where activity_id = :'activity_id' order by quota_type, scope_key;"
psql "$TEST_DATABASE_URL" -v ON_ERROR_STOP=1 -c "select quota_type, scope_key, total_value, used_value, frozen_value from marketing_quota where activity_id = :'activity_id' and used_value > total_value;"
~~~

## Data / Smoke

建议测试数据：

- 活动 A：首存活动，`DEPOSIT_SUCCESS`，`BONUS + RATIO`，预算 `10000.00`，用户限制 `1`。
- 活动 B：签到活动，`SIGN_IN_SUCCESS`，`BONUS + LADDER`，每日限制 `1`。
- 活动 C：返水活动，`REBATE_SETTLE`，`CASH + PROBABILITY` 和 `CASH + POOL_SPLIT`。
- 活动 D：非法矩阵活动，参数化覆盖非金额奖励非法 calculateType。
- 活动 E：quota 压测活动，预算或库存设置为刚好不足，用于额度不足、幂等重复和并发。

Smoke 执行原则：

- 所有测试数据必须使用唯一 activityCode / eventId 前缀，例如 `admin-rule-reward-quota-editors-smoke-*`。
- PostgreSQL smoke 只在测试库执行。
- 若系统提供清理脚本，Test Agent 可在测试结束后清理上述 activityCode 前缀的数据；清理命令应由 Test Agent 根据仓库既有脚本选择。
- 若当前仓库没有真实执行链路测试入口，Test Agent 至少应完成保存接口、模拟接口和 PostgreSQL 持久化 smoke，并在 `test.md` 中明确记录真实 quota 扣减、幂等和并发覆盖缺口。

## Decision

~~~text
PASS
~~~

需求：2026-05-29-admin-rule-reward-quota-editors
