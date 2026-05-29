# Test Plan

## Scope

需求：`2026-05-29-activityplugin-red-envelope-lottery`

本计划基于 `review.md` Decision 为 `PASS` 后制定。后续 Test Agent 负责执行，Test Plan Agent 本轮不执行测试、构建、格式化或 git 命令。

测试对象：

- `RED_ENVELOPE` 样板插件：注册、配置校验、场次窗口、红包池候选、用户抢包幂等、空包/命中原因、状态意图、模拟运行。
- `LOTTERY` 样板插件：注册、配置校验、抽奖窗口、抽奖次数、权重抽取、稳定随机审计、未中奖、状态意图、模拟运行。
- 主引擎边界：插件只输出玩法上下文、候选奖励、原因、审计和状态变更意图，不直接写 `participation`、`grant_order`、`outbox`、`quota`。
- PostgreSQL smoke：确认真实持久化链路中 participation/audit 可追溯，grant/outbox/quota 仍由主引擎控制。

## Test Strategy

- 优先补单元测试覆盖插件注册、配置校验和纯函数行为；这些测试应稳定、快速、无需外部依赖。
- 用集成测试覆盖 ActivityPlugin 与主活动引擎的协作，包括 snapshot/audit、quota 拒绝、幂等重复和插件失败隔离。
- 用并发测试验证同一用户、同一场次或同一抽奖池下的重复请求不会重复消费次数或生成多个候选结果。
- 用 PostgreSQL smoke 测试验证最小数据库链路，不要求覆盖全部业务排列，但必须验证关键表写入边界和可追溯字段。
- 模拟运行测试必须断言无真实副作用：不消费插件状态、不扣 quota、不写 grant/outbox。

## Test Cases

### 1. Plugin Registration

1. `RED_ENVELOPE` 和 `LOTTERY` 均可被注册表发现。
2. 两个插件均声明版本、支持的 `activityType`、能力、配置 Schema 和输出 Schema。
3. `activityType=RED_ENVELOPE` 绑定 `LOTTERY` 时校验失败。
4. `activityType=LOTTERY` 绑定 `RED_ENVELOPE` 时校验失败。
5. 未注册 `pluginCode`、禁用插件或版本范围不兼容时，上线前校验必须 BLOCK。
6. 插件 runtime status 枚举应按项目合同统一验证；若实现保留 `PASSED/REJECTED/SKIPPED/ERROR`，测试必须暴露与生命周期文档 `PASS/REJECT/SKIP/ERROR` 的不一致。

### 2. RED_ENVELOPE Normal Path

1. 给定合法场次、合法 `candidateRewards`、用户首次在 `[startAt, endAt)` 内参与时，插件返回通过状态。
2. 输出包含 `pluginCode=RED_ENVELOPE`、`pluginVersion`、`roundCode`、`poolCode`、`hit`、`reasonCode`、`pluginContext`、`audit`。
3. 命中红包时只返回候选 `rewardCode` 或候选红包编码，不直接创建发奖单。
4. `pluginContext` 必须挂在 `state.plugin.RED_ENVELOPE.*` 路径下。
5. `stateChangeIntents` 应表达用户本场已抢或抢包结果，但不直接落库业务结果。
6. participation snapshot 或 audit 中必须可追溯本次场次、命中/空包原因、活动版本和幂等键摘要。

### 3. RED_ENVELOPE Edge And Reject Paths

1. 事件发生在场次开始前时，输出未开始 reason，不产生候选奖励。
2. 事件发生在场次结束后时，输出已结束 reason，不产生候选奖励。
3. 同一活动内场次窗口重叠时，配置校验失败。
4. `rounds` 为空、`roundCode` 重复、`startAt >= endAt`、时间不可解析时，配置校验失败。
5. `poolTotalCount <= 0` 或 `perUserLimit <= 0` 时，配置校验失败。
6. `candidateRewards` 引用不存在或未启用的 `rewardCode` 时，配置校验失败。
7. 达到 `perUserLimit` 后再次参与，输出 `USER_LIMIT_REACHED` 或等价 reason，不产生新的候选奖励。
8. 红包池为空或抽中空包时，输出 `POOL_EMPTY`、`EMPTY_ENVELOPE` 或等价 reason，不产生候选发奖。
9. `emptyAllowed=false` 但配置无法保证非空候选能力时，上线前校验应拒绝或显式要求主引擎库存配置。

### 4. LOTTERY Normal Path

1. 给定合法抽奖配置、用户首次在窗口内抽奖时，插件返回通过状态。
2. 输出包含 `pluginCode=LOTTERY`、`pluginVersion`、`lotteryCode`、`itemCode`、`hit`、`reasonCode`、`pluginContext`、`audit`。
3. 命中奖项有 `rewardCode` 时，只返回候选奖励，不直接写 `grant_order`、`outbox` 或 `quota`。
4. 命中 `noPrize=true` 或 `itemCode=NO_PRIZE` 时，输出未中奖 reason，不产生候选发奖。
5. `pluginContext` 必须挂在 `state.plugin.LOTTERY.*` 路径下。
6. 审计必须包含权重总和、抽取位置或随机输入摘要，且不泄露 secret。

### 5. LOTTERY Edge And Reject Paths

1. 抽奖窗口开始前参与，输出未开始 reason，不产生候选奖项。
2. 抽奖窗口结束后参与，输出已结束 reason，不产生候选奖项。
3. `items` 为空、`itemCode` 重复、`weight < 0`、总权重为 0 时，配置校验失败。
4. `perUserDrawLimit <= 0` 时，配置校验失败。
5. `rewardCode` 引用不存在或未启用的活动奖励时，配置校验失败。
6. 权重为 0 的奖项在多次固定输入样本中不得被抽中。
7. 相同稳定随机输入必须得到相同抽奖结果。
8. 不同事件幂等键或不同 `drawIndex` 应允许得到不同审计输入和可解释结果。
9. 达到 `perUserDrawLimit` 后再次抽奖，输出 `DRAW_LIMIT_REACHED` 或等价 reason，不产生新的候选奖励。

### 6. Insufficient Quota / Budget

1. RED_ENVELOPE 插件命中候选奖励，但主引擎 `STOCK` quota 不足时，最终参与结果应被主引擎拒绝或不发奖。
2. LOTTERY 插件命中候选奖励，但主引擎库存或预算不足时，最终参与结果应被主引擎拒绝或不发奖。
3. quota 不足时插件 audit 仍应记录命中候选，主引擎 audit 应记录额度不足原因。
4. quota 不足不能回滚成插件空包或未中奖；插件结果和主引擎拒绝原因应可区分。
5. 插件不得自行扣减、恢复或覆盖 quota。

### 7. Idempotency And Duplicate Requests

1. 同一标准事件幂等键重复进入 RED_ENVELOPE，必须返回首次结果或被 Event Inbox 幂等拦截，不得重复消费抢包次数。
2. 同一标准事件幂等键重复进入 LOTTERY，必须返回首次结果或被 Event Inbox 幂等拦截，不得重复消费抽奖次数。
3. 同一用户使用不同事件幂等键重复参与时，应受 `perUserLimit` 或 `perUserDrawLimit` 限制。
4. 重试场景下 participation snapshot/audit 不应出现互相矛盾的插件结果。
5. grant/outbox 侧不得因为重复请求产生重复发奖命令。

### 8. Concurrency

1. RED_ENVELOPE：同一用户、同一场次、同一活动版本并发提交多次抢包，只能有一次有效抢包状态变更。
2. RED_ENVELOPE：多用户并发抢同一场次时，插件候选与主引擎 quota 应保持一致；库存不足的请求由主引擎拒绝。
3. LOTTERY：同一用户并发抽奖，最终有效抽奖次数不得超过 `perUserDrawLimit`。
4. LOTTERY：并发重试同一幂等键，抽奖结果必须一致。
5. 并发失败不应影响同一事件下其他普通活动或其他插件活动。
6. 并发测试应启用 Go race detector；如项目存在外部依赖，可至少对插件和主引擎相关包执行定向 race 测试。

### 9. Simulation

1. RED_ENVELOPE simulate 返回场次、命中/空包原因、候选奖励和 quota 风险提示。
2. LOTTERY simulate 返回抽中奖项、权重解释、随机输入摘要和候选奖励。
3. simulate 不写真实 `event_inbox`、`participation`、`grant_order`、`outbox`、`quota` 或真实插件状态。
4. simulate 输出应包含 `pluginContext`，且路径符合 `state.plugin.<pluginCode>.*`。
5. simulate 对非法配置应返回清晰校验错误。

### 10. Plugin Failure Isolation

1. RED_ENVELOPE 插件执行错误时，仅绑定该插件的活动失败或跳过，不影响同一事件下普通活动。
2. LOTTERY 插件执行错误时，仅绑定该插件的活动失败或跳过，不影响同一事件下其他插件活动。
3. 插件错误必须进入 audit，包含 `pluginCode`、`lifecycleStep`、`status` 和 reason。
4. 插件错误不得产生 grant/outbox。

### 11. PostgreSQL Smoke

1. 使用 PostgreSQL 测试库启动最小真实链路。
2. 创建或加载一个 RED_ENVELOPE 活动和一个 LOTTERY 活动，均绑定合法 reward 与 quota。
3. 投递一条标准事件触发 RED_ENVELOPE 正常命中，断言 participation 或 audit 中存在插件 snapshot。
4. 投递一条标准事件触发 LOTTERY 正常命中或未中奖，断言 participation 或 audit 中存在权重/随机审计摘要。
5. 构造 quota 不足场景，断言插件候选存在但主引擎不创建成功 grant/outbox。
6. 重复投递相同幂等事件，断言 PostgreSQL 中 participation/grant/outbox 不重复。
7. simulate 场景执行前后比对 PostgreSQL 关键表行数，断言无真实副作用。

## Required Commands

后续 Test Agent 应按项目实际包名调整定向命令；以下命令清单仅供执行阶段使用，本轮不得执行。

~~~bash
go test ./...
go test -race ./...
go test ./... -run 'Test.*RedEnvelope|Test.*Lottery|Test.*ActivityPlugin'
go test ./... -run 'Test.*Idempot|Test.*Duplicate|Test.*Concurrent'
go test ./... -run 'Test.*Quota|Test.*Insufficient|Test.*Budget'
go test ./... -run 'Test.*Simulation|Test.*Simulate'
go test ./... -run 'Test.*Postgres|Test.*PostgreSQL|Test.*Smoke'
~~~

如果项目区分单元测试和集成测试标签，PostgreSQL smoke 应使用项目既有标签或环境变量，例如：

~~~bash
EXMARKETING_TEST_POSTGRES_DSN='postgres://USER:PASSWORD@HOST:PORT/DB?sslmode=disable' go test ./... -run 'Test.*Postgres|Test.*PostgreSQL|Test.*Smoke' -count=1
~~~

## Acceptance Checklist

- 正常路径已覆盖 RED_ENVELOPE 和 LOTTERY。
- 额度不足已覆盖插件候选命中但主引擎 quota/budget 拒绝。
- 幂等重复已覆盖同一事件重试和同一用户多事件限制。
- 并发已覆盖同用户并发、同幂等键并发和多用户库存竞争。
- PostgreSQL smoke 已覆盖 snapshot/audit、grant/outbox/quota 边界和 simulate 无副作用。
- 插件职责边界已验证：不直接写 `participation`、`grant_order`、`outbox`、`quota`。

## Decision

~~~text
PASS
~~~
