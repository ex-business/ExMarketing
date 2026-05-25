# Test Plan

## Agent 指令

你是 Test Plan Agent。Review 已为 PASS。本文件只定义后续 Test Agent 应执行的测试用例和命令清单。

本轮未执行测试、构建、格式化或 git 命令。

## Test Strategy

测试目标是验证 ActivityPlugin 只作为玩法上下文和无副作用意图扩展点，不能绕过公共活动链路。后续 Test Agent 应优先覆盖以下风险：

1. 普通满减、首存、续存活动仍可仅通过数据配置完成，不需要插件。
2. 插件上下文只能通过 `state.plugin.<pluginCode>.*` 进入规则、奖励预览、snapshot、simulate 和 audit。
3. 插件不得覆盖 `attributes.*`、`facts.*`、`state.activity.*`、`state.subject.*`、`state.quota.*`、`state.exclusive.*`。
4. 插件不得直接发奖、扣 quota、写 outbox、消费玩法状态或产生真实插件状态持久化。
5. 额度不足、重复事件、并发事件、运行时异常都不得造成重复 participation、重复 grant_order、重复 outbox 或半完成副作用。
6. PostgreSQL 仓储必须做 smoke 验证，不能只用内存仓储代表最终验收。

## Test Cases

### 1. 正常路径：无插件普通活动

前置：

- 配置一个无 `pluginCode` 的普通满减活动。
- 配置一个无 `pluginCode` 的首存活动。
- 配置一个无 `pluginCode` 的续存活动。
- 规则仅依赖 `attributes.*`、`facts.*`、普通 reward 和普通 quota。

步骤：

1. 分别发送满足条件的标准事件。
2. 断言活动匹配、规则通过、reward calculator 计算正确。
3. 断言 quota 正常扣减或预留。
4. 断言每个命中活动产生一条 participation。
5. 断言每个奖励配置产生一张 grant_order。
6. 断言 outbox 生成标准 Reward Command。
7. 断言 snapshot 或 audit 中没有插件上下文。

预期：

- 普通活动全链路成功。
- 不需要新增插件即可完成满减、首存、续存。
- 主流程幂等键格式不变。

### 2. 正常路径：插件上下文参与规则和奖励预览

前置：

- 注册一个测试用插件，例如 `LOTTERY_TEST`。
- 配置一个兼容 activityType 的活动，绑定 `pluginCode=LOTTERY_TEST`。
- 插件返回 `pluginContext`，例如 `state.plugin.LOTTERY_TEST.drawChance=1`、`state.plugin.LOTTERY_TEST.poolStatus=OPEN`。
- 规则引用 `state.plugin.LOTTERY_TEST.drawChance` 和 `state.plugin.LOTTERY_TEST.poolStatus`。

步骤：

1. 发送满足标准事件和插件上下文的事件。
2. 断言 rule evaluator 能读取 `state.plugin.LOTTERY_TEST.*`。
3. 断言 reward calculator 使用同一插件上下文路径完成奖励预览或计算。
4. 断言正式发奖仍经过 quota、participation、grant_order、outbox。
5. 断言 snapshot、simulate 或 audit 展示 `pluginCode`、`pluginContext`、`precheckResult`、规则结果、奖励预览、quota 影响和 `stateChangeIntents`。

预期：

- 插件只补充上下文和解释。
- 最终通过仍由规则、奖励和 quota 决定。
- `stateChangeIntents` 可被记录或展示，但不执行真实玩法状态变更。

### 3. 命名空间和覆盖保护

前置：

- 插件返回与事件属性同名字段，例如插件上下文里有 `amount`，事件 `attributes.amount` 也存在。
- 插件尝试返回或污染 `attributes.amount`、`facts.vipLevel`、`state.activity.status`、`state.quota.remaining` 等非插件命名空间字段。

步骤：

1. 配置规则分别引用 `attributes.amount` 和 `state.plugin.<pluginCode>.amount`。
2. 验证两者取值互不覆盖。
3. 配置使用非法别名路径，例如 `plugin.<pluginCode>.amount`、`state.<activityType>.amount` 或短路径 `amount`。
4. 验证非法路径不能通过规则校验或不能默认通过。
5. 验证插件输出的非授权命名空间字段被拒绝、忽略并记录明确错误，且不会覆盖主流程上下文。

预期：

- 唯一允许的插件规则路径是 `state.plugin.<pluginCode>.*`。
- 插件不能覆盖 attributes、facts 或已有 state 白名单。
- 字段缺失、类型不匹配、非法操作符不能默认通过。

### 4. 配置校验：未知和非法 pluginCode

前置：

- 准备未知 `pluginCode`。
- 准备已注册但 activityType 不兼容的 `pluginCode`。
- 准备缺少必填字段、非法版本、未知字段、非法概率或非法周期的插件配置。

步骤：

1. 创建活动时绑定未知 `pluginCode`。
2. 更新活动时绑定未知 `pluginCode`。
3. 上线前校验活动时绑定未知 `pluginCode`。
4. 使用 activityType 不匹配的插件配置。
5. 使用非法 config_json。

预期：

- 创建、更新或上线前校验阶段必须拒绝。
- 返回机器可识别错误码或 reasonCode。
- 不得保存可上线的脏配置。

### 5. 运行时兜底：未知 pluginCode 不发奖

前置：

- 通过测试夹具或数据库准备一条历史脏配置：活动为 ACTIVE，但绑定未知 `pluginCode`。

步骤：

1. 发送可命中该活动的标准事件。
2. 断言运行时返回明确配置错误。
3. 查询 participation、grant_order、outbox、quota 相关记录。

预期：

- 活动不得静默跳过。
- 活动不得按无插件执行。
- 不产生发奖、不扣 quota、不写 outbox。

### 6. 上下文构建失败和玩法状态不足

前置：

- 插件依赖必需事件字段、facts 或营销系统自身状态快照。
- 准备以下失败输入：缺少必需 attributes、Fact Provider 不可用、玩法状态未初始化、抽奖次数快照为 0、红包场次未开始或已结束。

步骤：

1. 对每类失败输入发送标准事件。
2. 断言插件返回明确 `precheckResult` 和 reasonCode。
3. 断言活动不会默认通过。
4. 断言不产生 grant_order 和 outbox。
5. 断言 snapshot、simulate 或 audit 可解释失败原因。

预期：

- 可预期业务异常返回可识别 reasonCode。
- 状态不足不发奖。
- 不产生半完成副作用。

### 7. 额度不足

前置：

- 配置绑定插件的活动，插件前置通过，规则通过，奖励计算有结果。
- 将活动预算、库存或用户次数 quota 设置为不足。

步骤：

1. 发送满足插件和规则条件的标准事件。
2. 断言 quota 阶段拒绝。
3. 查询 participation、grant_order、outbox。
4. 查询或断言不存在真实插件状态消费记录。
5. 验证 `stateChangeIntents` 未被执行；如系统设计记录意图，应只存在于拒绝解释或预览材料中，不代表真实状态变化。

预期：

- quota 不足时不得发奖。
- 不留下已发奖、已扣 quota、已消费玩法状态但主流程失败的不一致结果。

### 8. 幂等重复事件

前置：

- 配置绑定插件的活动，插件上下文构建确定性成功。
- 准备相同 `tenantId:sourceSystem:eventType:bizType:bizId` 的重复标准事件。

步骤：

1. 连续发送同一事件两次或多次。
2. 断言同一活动只产生一条 participation。
3. 断言同一 participation 的同一奖励配置只产生一张 grant_order。
4. 断言 outbox 不重复生成。
5. 断言 `stateChangeIntents` 的幂等标识稳定，使用 participation 幂等键或确定性派生键。
6. 断言没有真实插件玩法状态重复消费。

预期：

- 重复事件返回幂等结果。
- 插件不会破坏主流程幂等语义。

### 9. 并发

前置：

- PostgreSQL 仓储可用。
- 配置绑定插件的活动，quota 边界设置为有限值，例如预算或库存只允许成功 N 次。
- 准备同一幂等键并发请求和不同幂等键并发请求两组数据。

步骤：

1. 并发发送相同幂等键的事件，建议至少 20 个并发请求。
2. 断言最终只有一条 participation、一张 grant_order 和一次 outbox。
3. 并发发送不同幂等键但竞争同一 quota 的事件，建议并发数大于 quota 剩余额度。
4. 断言成功数不超过 quota。
5. 断言失败请求有明确 quota 或幂等 reasonCode。
6. 断言没有负库存、预算超扣、重复 outbox 或插件状态消费。

预期：

- 并发下幂等和 quota 仍保持一致。
- 插件上下文和意图不引入竞态副作用。

### 10. 模拟运行无副作用

前置：

- 配置绑定插件的活动。
- 插件可返回 `pluginContext`、`precheckResult`、`explain`、`stateChangeIntents`。

步骤：

1. 调用 simulate 或 snapshot 预览接口。
2. 断言响应包含 `pluginCode`、`pluginContext`、规则结果、奖励预览、quota 预览、发奖指令预览和 `stateChangeIntents`。
3. 调用后查询 inbox、participation、grant_order、outbox、quota 和插件状态相关持久化。

预期：

- simulate 只预览，不写主流程状态。
- `stateChangeIntents` 展示为意图，不执行真实状态变化。

### 11. PostgreSQL Smoke

前置：

- 使用真实 PostgreSQL 测试库。
- 迁移已完成。
- 测试库中没有影响本需求断言的脏数据，或测试使用独立 tenant/activityCode。

步骤：

1. 在 PostgreSQL 仓储下跑一条无插件普通活动成功链路。
2. 在 PostgreSQL 仓储下跑一条插件活动成功链路。
3. 在 PostgreSQL 仓储下跑一条 quota 不足链路。
4. 在 PostgreSQL 仓储下跑一条重复事件幂等链路。
5. 在 PostgreSQL 仓储下跑一组并发 quota 竞争链路。
6. 直接查询 PostgreSQL 中 participation、grant_order、outbox、quota/budget/inventory 相关表。

预期：

- 主流程事务和幂等语义在 PostgreSQL 中成立。
- quota 不足和异常路径不产生半完成记录。
- 本期不新增插件状态表、插件状态仓储或插件独立事务。

### 12. 架构边界回归

步骤：

1. 检查实现未新增普通活动专用 controller、专用 model、专用 job、专用 Kafka topic 或专用发奖链路。
2. 检查实现未新增外部业务数据库连接或外部业务代码包依赖。
3. 检查标准事件字段、幂等键、participation、grant_order、outbox、callback 基本语义未被插件改写。

预期：

- 插件是公共扩展点，不是单个活动的专用代码入口。

## Required Commands

后续 Test Agent 在 Dev 实现后应执行以下命令。命令清单仅供后续执行，本轮 Test Plan Agent 不执行。

~~~bash
go test ./...
go build ./...
~~~

如仓库已有针对 PostgreSQL 的集成测试标签、环境变量或 compose 脚本，后续 Test Agent 应在确认本地测试库配置后补充执行对应 PostgreSQL 集成测试命令，并记录实际命令、数据库连接目标和结果。

## Data / Smoke

后续 Test Agent 应准备或确认以下测试数据：

1. 普通满减、首存、续存三类无插件活动配置。
2. 一个注册表内存在且 activityType 兼容的测试插件配置。
3. 一个未知 `pluginCode` 配置。
4. 一个 activityType 不兼容插件配置。
5. 一个插件上下文字段与 `attributes` 同名的活动配置。
6. quota 充足、quota 不足、quota 边界并发竞争三组活动配置。
7. 同一幂等键重复事件和不同幂等键并发事件。
8. PostgreSQL 独立 tenant/activityCode 测试数据，便于直接查询 participation、grant_order、outbox 和 quota 结果。

## Decision

~~~text
PASS
~~~

需求：2026-05-24-113400-9
