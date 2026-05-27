# Test Plan

## Agent 指令

你是 Test Agent。只有 `review.md` 为 `PASS` 且本文件 Decision 为 `PASS` 后才能进入测试实现和执行。

本计划只定义后续测试用例和命令清单。Test Agent 可按本计划补充测试代码并执行命令；Test Plan Agent 本轮未执行测试、构建、格式化或 git 命令。

## Test Strategy

覆盖 ActivityPlugin 注册表、活动配置校验、规则字段路径、插件生命周期、主流程隔离、幂等、并发额度扣减和 PostgreSQL 持久化语义。

优先级：

1. 单元测试：插件注册表、重复编码、未知插件、不兼容 activityType、字段路径校验、生命周期返回结构。
2. 服务/处理链路测试：绑定插件活动的正常路径、预检查拒绝、插件上下文失败、状态变更意图仅预览、不影响普通活动。
3. 幂等和并发测试：同一事件重复请求不重复发奖；并发请求在 quota 不足时只允许有限成功，失败无脏写。
4. PostgreSQL smoke：使用 Docker PostgreSQL 和真实仓储验证 schema、事务、quota、participation、grant_order、outbox 的最终状态。

测试必须显式断言插件不负责通用链路：不得直接创建 participation、grant_order、outbox，不得直接扣 quota，不得绕过事件幂等。插件输出只能进入 `state.plugin.<pluginCode>.*` 和解释摘要。

## Test Cases

### 1. 注册表

- 注册一个合法插件后，可通过 `pluginCode` 精确查找，返回唯一插件。
- 注册多个不同编码插件后，查找互不串扰。
- 重复注册同一 `pluginCode` 必须失败，或保持明确唯一结果；测试需按实现口径断言错误或不可覆盖。
- 查找未知 `pluginCode` 返回可识别的 not found 错误。
- 插件兼容 activityType 校验：
  - `pluginCode=RED_ENVELOPE` 兼容 `RED_ENVELOPE` 或声明集合内类型时通过。
  - `pluginCode=RED_ENVELOPE` 绑定 `FIRST_DEPOSIT` 等不兼容类型时拒绝。

### 2. 活动配置校验

- 普通活动未配置 `pluginCode` 时，创建/更新/上线校验保持原行为，不要求插件注册表参与。
- 配置未知 `pluginCode` 的活动在创建或上线前被拒绝，错误包含机器可识别 code 和可读 reason。
- 配置已注册但不兼容 activityType 的插件被拒绝。
- 配置合法插件的活动通过校验，保存后 `pluginCode` 不丢失。
- 规则字段路径校验：
  - 绑定插件活动允许引用 `state.plugin.<pluginCode>.<path>`。
  - 未绑定插件活动引用 `state.plugin.*` 被拒绝。
  - 绑定插件活动引用其他插件编码被拒绝。
  - `plugin.*`、`state.<activityType>.*`、短路径别名被拒绝。

### 3. 正常路径

- 构造一个测试插件：
  - `BuildContext` 返回确定性上下文，例如 `{"roundId":"round_001","chance":1}`。
  - `Precheck` 返回通过和 `reasonCode=OK`。
  - `AfterQualified` 返回状态变更意图预览，例如 `CONSUME_CHANCE`。
- 绑定插件活动处理标准事件：
  - 规则能读取 `state.plugin.<pluginCode>.chance` 并通过。
  - 奖励计算、Quota 预留、participation、grant_order、outbox 仍由主流程生成。
  - 运行解释包含 `pluginCode`、`pluginContext`、`precheckResult`、`explain`、`stateChangeIntents`。
  - 状态变更意图只出现在 snapshot/simulate/解释输出，不落真实插件状态表，不产生额外业务写入。
- 模拟运行同一活动：
  - 输出插件摘要和 quota/outbox/grant 预览。
  - 不写 event_inbox、participation、grant_order、outbox、quota reservation 或真实插件状态。

### 4. 插件拒绝和错误路径

- 插件预检查拒绝：
  - 处理结果为活动/规则拒绝，不进入可发奖状态。
  - 不生成 grant_order。
  - 不生成 outbox。
  - 不扣 BUDGET/STOCK/USER_LIMIT/DAILY_LIMIT。
  - 按 review 反馈确认审计语义：应写 `REJECTED` participation，`reject_reason` 包含插件 `reasonCode` 和 reason；若实现明确选择不写，测试必须标记为 BLOCK 并回传 Dev/Review 重新确认。
- 插件构建上下文失败：
  - 不默认放行。
  - 返回可解释错误 code/reason。
  - 不生成 grant_order/outbox，不扣 quota。
- 插件预检查返回错误：
  - 行为应等同不可放行错误，不进入奖励链路。
  - 错误摘要不得包含 secret、token 或完整 Authorization。
- 插件返回状态变更意图但后续奖励金额为 0：
  - 按零奖励语义处理，不生成 grant_order/outbox，不扣 quota。
  - 插件状态变更仍不执行真实写入。

### 5. 额度不足

- 为绑定插件活动配置 STOCK 或 BUDGET quota，总额度小于奖励需求。
- 插件预检查通过，通用规则和奖励计算通过，但 quota 扣减阶段返回额度不足。
- 断言：
  - 返回明确 `QUOTA_EXCEEDED` 或等价结构化错误。
  - 不保留 `REWARDABLE` 脏 participation。
  - 不生成 grant_order。
  - 不生成 outbox。
  - quota `used_value`、`frozen_value`、reservation 不发生错误增加。
  - 插件状态变更意图不执行真实写入。

### 6. 幂等重复

- 同一标准事件、同一 `Idempotency-Key`、同一绑定插件活动连续提交两次。
- 首次正常生成 participation、grant_order、outbox 和 quota reservation。
- 第二次返回 duplicate 或等价幂等结果。
- 断言：
  - participation 数量不增加。
  - grant_order 数量不增加。
  - outbox 数量不增加。
  - quota 不重复冻结或扣减。
  - 插件上下文不得引入随机数、当前时间或外部状态导致第二次结果变化。
- 相同 `Idempotency-Key` 但 payload 不一致时返回 `INVALID_IDEMPOTENCY_KEY`，且不产生新业务副作用。

### 7. 并发

- 使用真实 PostgreSQL 仓储或实现支持的并发测试仓储，为同一绑定插件活动配置有限 STOCK/BUDGET，例如总量 1 或 2。
- 并发提交 N 个不同幂等键的标准事件，N 大于 quota 总量。
- 断言：
  - 成功数不超过 quota 总量。
  - grant_order/outbox 数量等于成功数。
  - quota `frozen_value + used_value` 不超过 total。
  - 失败请求返回 quota 不足或等价结构化错误。
  - 失败请求不保留 grant_order/outbox，不保留 `REWARDABLE` 脏 participation。
  - 无 data race；如果测试使用 Go race detector，必须通过。

### 8. PostgreSQL Smoke

- 启动 Docker PostgreSQL，应用 migration，使用 `EX_MARKETING_DATABASE_URL` 运行服务或集成测试。
- 创建普通活动，确认未配置插件时普通链路可创建、模拟和处理，不出现伪造插件摘要。
- 创建绑定测试插件的活动，执行正常事件：
  - 查询 `marketing_activity.plugin_code` 正确保存。
  - 查询 `marketing_participation` 存在预期状态和快照。
  - 查询 `marketing_grant_order`、`marketing_outbox` 数量与奖励一致。
  - 查询 `marketing_quota` 和 `marketing_quota_reservation` 与冻结结果一致。
- 执行插件预检查拒绝事件：
  - 查询 `marketing_participation` 为 `REJECTED`，`reject_reason` 含插件拒绝 reason。
  - 查询无对应 grant_order/outbox/quota reservation。
- 执行 quota 不足事件：
  - 查询没有 grant_order/outbox 脏写。
  - 查询 quota 聚合未超过 total。
- 执行重复幂等事件：
  - 查询各业务表行数不重复增加。

### 9. 文档和边界回归

- README、architecture、api、data-model 中 ActivityPlugin 说明与实现保持一致：
  - 普通活动优先配置化。
  - 插件只负责特殊上下文、预检查、状态变更意图预览。
  - 插件上下文路径唯一为 `state.plugin.<pluginCode>.*`。
  - 插件不得负责风控、幂等、预算、发奖、outbox、callback。
- 如果实现新增示例插件，仅用于测试或内部 demo；不得把签到、任务、VIP、邀请等普通可配置活动硬编码成必须插件化。

## Required Commands

后续 Test Agent 在完成测试实现后执行以下命令。若某个命令因环境缺失失败，需记录失败原因和替代验证结果。

~~~bash
go test ./...
go build ./...
go test -race ./...
docker compose up -d postgres
export EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable'
go test ./... -count=1
~~~

如实现提供更细粒度测试名称，优先追加定向命令，示例：

~~~bash
go test ./internal/... -run 'ActivityPlugin|Plugin|Quota|Idempotency|Concurrent' -count=1
go test ./internal/postgres/... -run 'ActivityPlugin|Quota|Idempotency|Concurrent' -count=1
~~~

PostgreSQL smoke 如需手工 API 验证，按实现后的端口和 README 启动方式执行：

~~~bash
docker compose up -d postgres
export EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable'
go run ./cmd/server
~~~

然后用 curl 或现有 API 测试脚本覆盖：

~~~bash
curl -sS -X POST http://localhost:8080/admin/v1/activities
curl -sS -X POST http://localhost:8080/admin/v1/quotas
curl -sS -X POST http://localhost:8080/openapi/v1/events
curl -sS -X POST http://localhost:8080/admin/v1/simulations
~~~

## Data / Smoke

建议准备以下最小数据集：

- `tenantId=site_001`。
- 普通活动：`activityCode=FIRST_DEPOSIT_NORMAL`，无 `pluginCode`。
- 插件活动：`activityCode=PLUGIN_ROUND_BONUS`，`activityType=RED_ENVELOPE` 或测试专用类型，`pluginCode=TEST_ROUND`。
- 不兼容活动：`activityCode=PLUGIN_INCOMPATIBLE`，`activityType=FIRST_DEPOSIT`，`pluginCode=TEST_ROUND`。
- 未知插件活动：`pluginCode=UNKNOWN_PLUGIN`。
- 标准事件：
  - 正常事件：`Idempotency-Key=site_001:payment:DEPOSIT:deposit:dep_001`。
  - 重复事件：复用同一 key 和同一 payload。
  - 冲突事件：复用同一 key 但变更 payload。
  - 并发事件：`dep_concurrent_001` 至 `dep_concurrent_N`。
- quota：
  - 正常路径额度足够：STOCK/BUDGET total 大于奖励需求。
  - 额度不足路径：STOCK 或 BUDGET total 小于奖励需求。
  - 并发路径：STOCK total 为 1 或 2。

PostgreSQL smoke 必查表：

- `marketing_activity`
- `marketing_participation`
- `marketing_grant_order`
- `marketing_outbox`
- `marketing_quota`
- `marketing_quota_reservation`

## Exit Criteria

测试通过条件：

- 正常路径、额度不足、幂等重复、并发、PostgreSQL smoke 均有自动化测试或明确 smoke 记录。
- 插件预检查拒绝的 participation 审计语义已按 `REJECTED` 验证。
- 普通活动无插件配置时行为不回归。
- 模拟运行无真实副作用。
- 所有 Required Commands 中适用命令通过；不适用命令必须说明原因。

阻塞条件：

- 插件可直接发奖、扣 quota、写 outbox、写 grant_order 或绕过 participation。
- 未知/不兼容插件不能在创建或上线前拦截。
- 插件预检查拒绝会生成 grant_order/outbox 或扣 quota。
- 重复事件会重复发奖或重复扣减。
- 并发下 quota 超扣。
- PostgreSQL smoke 与内存仓储语义不一致且影响事务或锁。
- 插件拒绝 participation 审计语义未实现且未有 Review/Analysis 明确豁免。

## Decision

~~~text
PASS
~~~

需求：2026-05-24-activityplugin
