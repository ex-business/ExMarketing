# Analysis

## 任务信息

需求 ID：2026-05-24-113400-9

标题：活动插件体系

风险等级：中

需求来源：

- `tasks/2026-05-24-113400-9/task.md`
- `tasks/2026-05-24-113400-9/review.md`
- `README.md`
- `CLAUDE.md`
- `docs/architecture.md`
- `docs/api.md`
- `docs/data-model.md`
- `docs/roadmap.md`
- `docs/workflow.md`
- `/Users/tong/Documents/GitHub/OracleBetX-Projects/act-analysis/22-独立活动营销中台设计.md`

## Review 反馈处理

本轮为 Review 打回后的返工，已先读取 `tasks/2026-05-24-113400-9/review.md`，并逐条处理如下。

### P1-1：插件上下文规则可读命名空间未定稿

处理结论：已收敛为唯一命名空间 `state.plugin.<pluginCode>.*`。

本需求要求在现有规则字段白名单 `attributes.*`、`state.activity.*`、`state.subject.*`、`state.quota.*`、`state.exclusive.*`、`facts.*` 之外，新增且仅新增插件上下文字段路径：

```text
state.plugin.<pluginCode>.<fieldPath>
```

规则：

1. 插件上下文不得写入或覆盖 `attributes.*`、`facts.*`、`state.activity.*`、`state.subject.*`、`state.quota.*`、`state.exclusive.*`。
2. `<pluginCode>` 必须与活动配置绑定的 `pluginCode` 完全一致，大小写、字符集和注册表一致。
3. 同一活动本期只允许一个主插件，因此不存在多个插件写同一 `state.plugin.<pluginCode>` 的冲突。
4. 若插件字段与事件属性同名，例如 `amount`，规则中必须显式写成 `state.plugin.LOTTERY.amount` 或 `attributes.amount`，不得使用模糊短路径。
5. snapshot、simulate、reward calculator 可读取的插件上下文均使用同一结构，不能各自定义不同路径。

### P1-2：插件后置状态变更一致性模型未定义

处理结论：本期不实现真实插件状态持久化；`AfterQualified` 只允许输出无副作用的状态变更意图，不执行真实状态变更。

本需求将一致性边界明确为：

1. 当前公共内核的 activity-level transaction 仍只覆盖 participation、quota、grant_order、outbox，以及现有审计能力。
2. 本期 ActivityPlugin 不新增插件状态仓储，不新增插件状态表，不新增插件独立事务，不在插件内扣减抽奖次数、签到次数、任务进度、红包场次库存或任何玩法状态。
3. 插件的后置输出只能是 `stateChangeIntents`，用于 snapshot、simulate、审计解释和后续设计输入。
4. 因为本期不执行真实插件状态变更，quota 不足、规则拒绝、重复事件、主流程回滚时，不会留下插件侧半完成副作用。
5. 后续具体玩法若要求真实消费抽奖次数、记录签到、推进任务或占用玩法奖池，必须单独提出数据模型和事务需求；届时插件状态持久化必须与 participation、quota、grant_order、outbox 处于同一 PostgreSQL activity-level transaction，并使用同一 participation 幂等键或其确定性派生键。

### P2-1：DAILY_LIMIT、GIFT 混入本期验收

处理结论：已从本期必做验收中剔除。

`DAILY_LIMIT`、`DRAW_CHANCE`、`GIFT` 在现有文档中属于模型已提及或路线图目标，不作为本任务必实现能力。本任务可以在背景中提及其为未来扩展，但验收不得要求 Dev Agent 实现每日/周期 quota、抽奖次数奖励计算或实物奖品发放。

### P2-2：未知 pluginCode 处理口径模糊

处理结论：已收敛为创建/上线前校验优先拒绝，运行时只保留兜底错误。

要求：

1. 创建活动、更新活动或上线前校验时，如果配置了未知 `pluginCode`，必须拒绝配置，返回机器可识别错误。
2. 已存在历史脏配置或注册表异常导致运行时遇到未知 `pluginCode` 时，活动不得静默跳过，不得默认按无插件执行，必须返回明确配置错误并阻止该活动产生发奖。
3. 测试验收以“配置阶段拒绝”为主，运行时错误只作为防御性兜底。

### Open Questions 处理

1. 插件上下文统一暴露为 `state.plugin.<pluginCode>.*`；不允许覆盖已有 `state.activity`、`attributes` 或其他白名单路径。
2. 本需求不实现插件状态持久化。
3. 本期 `AfterQualified` 只返回状态变更意图并进入 snapshot/simulate/audit，不执行真实状态变更。

## 背景目标

ExMarketing 是独立活动营销中台，目标是通过标准事件、可选 Fact Query、标准 Reward Command 和 Callback 与外部业务系统解耦。系统自身负责活动配置、活动匹配、规则判断、奖励计算、预算库存、参与记录、幂等控制、发奖编排和审计对账，不连接外部业务库，不引用外部业务代码包，不直接修改用户余额。

当前公共内核已经覆盖：

```text
Event Intake -> Idempotency -> Campaign Match -> Rule Evaluation
-> Quota / Budget / Inventory -> Grant Order -> Outbox -> External Reward System
```

本需求要细化 ActivityPlugin 扩展模型。插件不是普通活动的专用实现入口，也不是绕过公共内核的发奖入口；插件只用于补充配置模型无法表达的玩法上下文和玩法解释。

目标：

1. 定义 ActivityPlugin 的业务边界、输入输出、规则命名空间和无副作用后置意图模型。
2. 保证普通满减、首存、续存等低风险活动继续只通过数据配置完成。
3. 保证全新玩法接入时，插件只补充上下文，最终仍走 rule evaluator、reward calculator、quota、participation、grant_order、outbox。
4. 保证模拟运行、snapshot、审计材料可以解释插件上下文对命中、拒绝、奖励预览和 quota 预览的影响。
5. 明确本期不实现插件状态持久化，避免半完成副作用和事务边界不清。

## 细化后的需求说明

ActivityPlugin 是活动类型/玩法扩展点，用于在标准事件上下文之外补充营销系统自身可控的玩法上下文。

适用场景：

```text
红包雨：场次、玩法窗口、活动系统自身奖池状态、用户是否处于可参与窗口
抽奖：活动系统自身抽奖次数快照、奖品池快照、空奖解释、玩法可参与状态
签到：连续签到天数快照、周期状态、补签状态
任务：任务进度快照、任务节点状态、完成状态
VIP：活动系统可用的红利领取周期、权益领取状态
邀请：邀请奖励进度、被邀请人达标状态、邀请奖励领取状态
```

不适用场景：

```text
普通满减
首存固定赠送
首存比例赠送
首存阶梯赠送
续存固定 / 比例 / 阶梯赠送
只依赖 attributes 或 facts 的签到、任务、VIP、邀请活动
只需要 FIXED / RATIO / LADDER / RANDOM / PROBABILITY / POOL_SPLIT 的普通奖励计算活动
```

插件准入标准：

1. 能通过 `triggerEvents`、`rules`、`rewards`、`quotas`、`attributes.*`、`facts.*` 表达的活动，不新增插件。
2. 只是在奖励金额、比例、阶梯、概率、库存、用户次数上有差异的活动，不新增插件。
3. 需要营销系统自身维护或读取玩法状态快照时，才允许考虑插件。
4. 需要将玩法上下文暴露给规则、奖励计算、模拟运行或审计时，才允许考虑插件。
5. 本期插件不得执行真实玩法状态变更，只能输出状态变更意图。

## 参与方

运营 / 活动配置人员：

配置活动基础信息、触发事件、规则、奖励、quota 和发奖通道。普通活动不应感知插件开发。

产品 / 玩法设计方：

定义新玩法是否确实需要特殊上下文；给出玩法状态字段、参与限制、展示解释、模拟运行和上线校验要求。

营销中台核心链路：

负责事件接入、幂等、活动匹配、规则执行、奖励计算、quota、participation、grant_order、outbox、callback、audit。

ActivityPlugin：

负责构建 `state.plugin.<pluginCode>.*` 上下文，输出前置可参与判断和后置状态变更意图，不负责真实发奖、真实 quota 扣减或真实玩法状态消费。

外部业务系统：

继续只通过标准事件、可选 Fact Query、Reward Command 和 Callback 接入；不得因为插件而新增业务库直连或业务代码依赖。

测试 / QA：

验证普通活动纯配置可用，验证插件上下文路径唯一，验证插件无副作用，验证异常场景下不重复发奖、不产生不一致状态。

## 输入

基础输入：

```text
标准事件：eventId、eventType、sourceSystem、tenantId、subject、biz、occurredAt、attributes
活动配置：activityCode、activityType、status、triggerEvents、timezone、priority、exclusiveGroup、stackable、config
规则配置：rules
奖励配置：rewards
Quota 配置：当前可执行 BUDGET、STOCK、USER_LIMIT
活动系统自身只读状态快照：参与记录、玩法状态快照、次数快照、场次快照、奖池快照、任务进度快照
可选 facts：通过受控 Fact Provider 获取的外部事实
```

插件配置输入：

```text
pluginCode
activityType 绑定关系
插件 config_json 片段
插件上下文字段 Schema
插件 stateChangeIntents Schema
插件上线前校验规则
```

输入约束：

1. 外部业务字段只能从 `attributes`、`facts`、`mapping`、`adapter` 进入。
2. 插件不得要求连接外部业务数据库。
3. 插件不得要求调用外部业务代码包。
4. 插件不得改变标准事件模型和幂等键格式。
5. 金额、预算、库存、概率等数值必须遵守 Decimal 和配置校验要求。
6. `DAILY_LIMIT`、`DRAW_CHANCE`、`GIFT` 不属于本期必做输入能力。

## 输出

插件允许输出：

```text
pluginContext：统一挂载到 state.plugin.<pluginCode>.*
precheckResult：可参与 / 不可参与 / 配置错误 / 状态不可用及 reasonCode
rewardContext：奖励计算可读取的同一 pluginContext
explain：用于 snapshot、simulate、audit 的插件解释
stateChangeIntents：资格通过后拟发生的玩法状态变更意图，仅记录，不执行
```

插件不得输出或直接生成：

```text
真实发奖
用户余额变更
grant_order 落库结果
outbox 投递结果
quota 扣减 / 冻结 / 确认结果
真实抽奖次数消费
真实签到记录
真实任务进度推进
真实红包场次库存占用
绕过规则引擎的最终通过结论
绕过幂等的 participation
```

插件输出进入统一链路：

```text
ActivityPlugin pluginContext / precheckResult / stateChangeIntents
-> rule evaluator
-> reward calculator
-> quota preview or reservation
-> participation
-> grant_order
-> outbox
-> callback / reconciliation
```

## 业务规则

### 命名空间规则

1. 插件上下文唯一规则可读路径为 `state.plugin.<pluginCode>.*`。
2. `state.plugin.<pluginCode>.*` 是规则字段白名单的新增路径；除此之外不新增 `state.<activityType>.*`、`plugin.*` 或其他别名。
3. 插件不得写入或覆盖 `attributes.*`、`facts.*`、`state.activity.*`、`state.subject.*`、`state.quota.*`、`state.exclusive.*`。
4. 插件字段缺失、类型不匹配、非法操作符时，规则不得默认通过。
5. reward calculator 若引用插件上下文，也必须使用同一 `state.plugin.<pluginCode>.*` 路径。
6. snapshot 和 simulate 中必须原样展示 `pluginCode`、`pluginContext`、`precheckResult`、`stateChangeIntents` 和字段路径，便于测试和审计断言。

### 无副作用状态意图规则

1. 本期 `AfterQualified` 不执行真实状态变更。
2. 本期不新增插件状态持久化模型，不新增插件状态仓储，不新增插件状态事务。
3. `stateChangeIntents` 只能表达“如果后续实现真实状态持久化，应发生什么”，例如 `consumeDrawChance=1`、`recordSignIn=true`、`advanceTaskNode=task_1`。
4. `stateChangeIntents` 必须关联 tenant、activity、subject、event、biz、pluginCode、participationId 或 participation 幂等键。
5. 模拟运行必须展示意图但不写任何数据库状态。
6. 正式运行可以把意图写入 participation snapshot 或审计解释，但不得改变真实玩法状态。
7. 若后续任务要求真实状态消费，必须先补充事务和幂等需求：插件状态变更、participation、quota、grant_order、outbox 必须处于同一 PostgreSQL activity-level transaction，并使用同一 participation 幂等键或确定性派生键。

### 插件职责规则

1. 插件只负责特殊上下文、玩法状态快照和玩法解释。
2. 插件可以基于标准事件、活动配置、营销系统自身状态快照和可选 facts 构建上下文。
3. 插件可以输出前置不可参与原因，例如抽奖次数快照为 0、红包场次未开始、玩法状态未初始化。
4. 插件不得把活动最终命中结果写死为通过；最终决策仍由主流程综合规则、奖励和 quota 结果产生。
5. 插件不得承担通用风控、幂等、预算、库存、次数限制、发奖单、Outbox、Callback 职责。

### 普通活动配置优先规则

1. 首存、续存、满减、充值赠送等存款类活动，若规则只依赖事件金额、币种、是否首存、支付方式、用户标签、次数限制、预算库存，必须走数据配置。
2. 签到、任务、VIP、邀请活动，若所需条件可由事件 `attributes` 或可选 `facts` 提供，必须走数据配置。
3. 抽奖、红包类活动，若只需要 `RANDOM`、`PROBABILITY`、`POOL_SPLIT` 奖励计算和普通 `BUDGET`、`STOCK`、`USER_LIMIT`，不应新增插件。
4. 不允许为了单个运营活动新增专用 controller、专用 model、专用 job、专用 Kafka topic 或专用发奖链路。

### 配置和注册规则

1. 一个活动本期最多绑定一个主 ActivityPlugin。
2. `pluginCode` 必须来自注册表，并与活动的 `activityType` 兼容。
3. 未配置 `pluginCode` 的活动必须走普通配置链路。
4. 配置了未知 `pluginCode` 的活动，在创建、更新或上线前校验阶段必须拒绝。
5. 运行时遇到未知 `pluginCode` 只作为兜底错误处理：该活动不得静默跳过，不得按无插件执行，不得发奖。
6. 插件配置包含未知字段、非法版本、缺少必填字段、概率/奖池/次数/周期配置不合法时，应在创建或上线前校验阶段拒绝。

### 发奖和额度规则

1. 插件不得直接发奖。
2. 插件不得创建或绕过 `marketing_grant_order`。
3. 插件不得直接写 `marketing_outbox` 并投递奖励。
4. 插件不得直接修改用户余额、积分、优惠券或会员权益。
5. 插件不得自行扣减预算、库存、用户次数或玩法次数。
6. 正式奖励必须由统一 reward calculator 计算，并经 quota、participation、grant_order、outbox 链路输出。
7. Quota 不足时，主流程回滚 participation、grant_order、outbox；由于本期插件无真实副作用，插件侧不得留下已消费状态。

### 幂等和一致性规则

1. 外部事件幂等键仍为 `tenantId:sourceSystem:eventType:bizType:bizId`。
2. 同一标准事件命中同一活动只能产生一条 participation。
3. 同一 participation 的同一奖励配置只能产生一张 grant_order。
4. 插件上下文构建必须是确定性的：同一事件、同一活动配置、同一状态快照下，输出应可重放解释。
5. `stateChangeIntents` 幂等标识必须使用 participation 幂等键或其确定性派生键，确保重复事件展示同一意图，而不是多个不同意图。
6. 本期不允许通过插件真实消费抽奖次数、签到次数、任务进度或玩法状态，因此重复事件不会产生插件状态重复消费。

### 审计和模拟运行规则

1. 插件上下文必须进入 participation snapshot 或模拟运行结果。
2. 模拟运行必须展示插件上下文对命中、拒绝、奖励预览和 quota 预览的影响。
3. 插件异常、配置缺失、状态不足、玩法不可参与等原因必须输出机器可识别 `reasonCode`。
4. `stateChangeIntents` 必须在模拟运行中展示为预览，不写入真实状态。
5. 正式运行若产生 participation snapshot，应包含插件上下文和意图，支持重放、审计和排查。

## 边界条件

1. 活动没有绑定插件时，必须走普通配置链路。
2. 活动绑定未知插件时，配置阶段优先拒绝；运行时兜底报配置错误且不发奖。
3. 插件绑定活动类型与活动配置不一致时，应拒绝配置。
4. 插件上下文缺少必需字段时，活动不得默认通过。
5. 插件依赖的玩法状态快照不存在时，应按业务语义返回不可参与、配置错误或状态未初始化，不得发奖。
6. 插件产生的字段与事件 `attributes` 字段同名时，必须通过命名空间区分，不存在优先级覆盖。
7. 多活动同时匹配时，插件不得破坏 priority、exclusiveGroup、stackable 的统一规则。
8. 一个活动本期只允许一个主插件；多插件组合是非目标。
9. DRAFT、SCHEDULED、ACTIVE、PAUSED、ENDED 状态下插件是否参与模拟或正式运行，应遵循活动主流程状态规则。
10. 时区相关玩法，例如签到周期、红包雨场次、VIP 周期，必须使用活动显式 timezone，不得依赖服务器默认时区。
11. 插件上下文构建超时或系统异常时，该活动应返回可解释失败，不得默认通过。
12. 模拟运行不得写 inbox、participation、grant_order、outbox、quota 或插件状态。

## 异常场景

插件配置异常：

```text
pluginCode 未注册
pluginCode 与 activityType 不匹配
config_json 缺少插件必需字段
插件配置包含未知字段或非法版本
概率、奖池、次数、周期等配置不合法
插件上下文字段 Schema 与规则引用不一致
```

上下文构建异常：

```text
标准事件缺少插件必需 attributes
营销系统自身玩法状态快照未初始化
抽奖次数快照为 0
红包场次未开始 / 已结束
签到周期不可参与
任务状态不满足推进条件
Fact Provider 缺失或不可用
插件字段类型不符合规则或奖励计算要求
```

主流程异常：

```text
插件前置通过但规则拒绝
插件上下文成功但 reward calculator 返回 0 奖励
quota 不足导致主流程回滚
重复事件命中同一活动
grant_order 或 outbox 创建失败
发奖失败进入 retry / DLQ
callback 失败或状态冲突
```

安全异常：

```text
插件试图连接外部业务库
插件试图直接修改用户余额
插件试图绕过标准事件和幂等
插件试图输出专用业务系统模型
插件试图记录 secret、token、完整 Authorization header
```

异常处理要求：

1. 业务可预期异常应返回机器可识别 `reasonCode`。
2. 不可参与类异常应形成拒绝或不可评估结果，不应返回通用内部错误。
3. 系统异常不得产生半完成发奖副作用。
4. 所有影响正式参与或发奖的插件异常应可在 snapshot、simulate 或 audit 中排查。

## 非目标范围

本需求不包含：

1. 不实现具体红包雨、抽奖、签到、任务、VIP、邀请插件的完整玩法。
2. 不实现真实插件状态持久化。
3. 不实现抽奖次数、签到次数、任务进度、红包场次库存的真实消费。
4. 不新增插件状态表、插件状态仓储或插件独立事务。
5. 不新增普通首存、续存、满减活动的业务专用代码。
6. 不新增外部业务数据库连接。
7. 不新增外部业务代码包依赖。
8. 不实现 Admin 可视化插件配置页面的完整交互。
9. 不实现 WebSocket 推送网关。
10. 不实现 MQ Event In 或 MQ Command Out。
11. 不实现人工发奖、导出发奖或完整审批流。
12. 不实现 `DAILY_LIMIT` 业务流程扩展。
13. 不实现 `DRAW_CHANCE` 或 `GIFT` 奖励计算。
14. 不改变标准事件模型、幂等键模型、grant_order、outbox、callback 的基本语义。
15. 不用插件替代 rule evaluator、reward calculator 或 quota 的通用扩展模型。

## 风险

业务边界风险：

如果插件被理解成“活动专用代码入口”，会重新回到每个活动一套 controller、job、发奖逻辑的旧模式，破坏营销中台公共内核。控制方式是普通活动配置优先，插件只输出 `state.plugin.<pluginCode>.*` 上下文和无副作用意图。

命名空间风险：

如果插件上下文路径不唯一，规则、奖励计算、模拟运行和审计会出现不同口径。控制方式是本期唯一允许 `state.plugin.<pluginCode>.*`。

账务一致性风险：

插件若直接发奖、扣库存或消耗玩法次数，可能造成重复发奖、预算漂移、玩法次数和 grant_order 不一致。控制方式是本期禁止真实插件状态变更，只允许意图输出。

幂等风险：

重复事件下，插件状态变更若没有与事件幂等绑定，可能重复消耗抽奖次数、重复签到或重复推进任务。控制方式是本期不消费真实状态，意图必须使用 participation 幂等键或确定性派生键。

配置复杂度风险：

普通活动如果过早插件化，会增加运营配置、测试和上线成本。控制方式是明确首存、续存、满减等活动只走数据配置。

审计风险：

插件上下文和意图如果不进入 snapshot、simulate 或 audit，后续无法解释用户为何命中、为何拒绝、为何获得某个奖品。控制方式是验收要求输出统一解释材料。

未来扩展风险：

真实红包雨、抽奖、签到等玩法最终仍需要状态持久化。控制方式是将其列为后续独立需求，必须单独定义数据模型、事务边界、幂等键和 PostgreSQL 验收。

## 验收标准

### 需求验收

1. 文档明确 ActivityPlugin 只负责特殊上下文、玩法状态快照、前置解释和后置意图，不负责通用风控、幂等、预算和发奖。
2. 文档明确普通满减、首存、续存类活动只走数据配置，不新增插件。
3. 文档明确插件上下文唯一规则路径为 `state.plugin.<pluginCode>.*`。
4. 文档明确插件不得覆盖 `attributes.*`、`facts.*`、`state.activity.*`、`state.subject.*`、`state.quota.*`、`state.exclusive.*`。
5. 文档明确本期不实现插件状态持久化，`AfterQualified` 只返回无副作用 `stateChangeIntents`。
6. 文档明确未知 `pluginCode` 创建/上线前校验必须拒绝，运行时只作为兜底错误。
7. 文档明确 `DAILY_LIMIT`、`DRAW_CHANCE`、`GIFT` 不属于本期必做验收。

### 行为验收

1. 新增普通首存活动，只通过标准活动配置、规则、奖励和 quota 配置即可表达，不需要新增插件。
2. 新增普通续存活动，只通过标准活动配置、规则、奖励和 quota 配置即可表达，不需要新增插件。
3. 新增普通满减活动，只通过标准活动配置、规则、奖励和 quota 配置即可表达，不需要新增插件。
4. 新增具备特殊玩法状态快照的红包雨或抽奖类活动时，可以通过 ActivityPlugin 补充 `state.plugin.<pluginCode>.*` 上下文，但发奖仍走统一 reward、quota、grant_order、outbox 链路。
5. 插件构建上下文失败时，活动不会默认通过，并能输出明确失败原因。
6. 插件绑定未知 `pluginCode` 或非法配置时，创建/上线前校验拒绝。
7. 运行时遇到未知 `pluginCode` 时，该活动不发奖，输出明确配置错误。
8. 重复事件不会因插件导致重复 participation、重复 grant_order、重复 outbox 或真实玩法状态消费。
9. Quota 不足时，不会留下已发奖或已消费玩法状态但主流程失败的不一致结果。
10. 模拟运行能展示 `pluginCode`、`pluginContext`、规则结果、奖励预览、quota 影响、发奖指令预览和 `stateChangeIntents`。
11. 插件相关关键上下文能进入 snapshot、simulate 或 audit，支持排查和对账。

### 架构验收

1. 不新增业务专用 controller、业务专用发奖链路、业务专用外部数据库依赖。
2. 不改变标准事件接入字段和幂等键格式。
3. 不改变 participation、grant_order、outbox、callback 的统一职责。
4. 插件与 rule evaluator、reward calculator、quota、outbox 的职责边界清楚。
5. 活动主流程不因新增普通活动而修改。
6. 不新增真实插件状态持久化表或插件状态仓储。

### 测试验收

后续实现阶段至少应覆盖以下测试场景：

1. 无插件普通活动命中并发奖。
2. 无插件首存 / 续存 / 满减活动通过数据配置完成。
3. 规则可读取 `state.plugin.<pluginCode>.*`，且不能通过其他插件别名路径读取。
4. 插件上下文不能覆盖 `attributes.*`、`facts.*` 或已有 `state.*` 白名单路径。
5. 未知插件配置在创建/上线前校验被拒绝。
6. 运行时未知插件兜底错误不发奖。
7. 插件上下文构建失败时活动拒绝且不发奖。
8. 插件上下文构建成功后仍由规则、奖励和 quota 决定最终结果。
9. 插件玩法状态不足快照，例如抽奖次数为 0，活动不发奖且原因明确。
10. 重复事件不会重复消费插件玩法状态；本期应验证没有真实插件状态写入。
11. Quota 不足时 plugin `stateChangeIntents` 不会被执行，participation、grant_order、outbox 不产生不一致。
12. 模拟运行无副作用，并展示插件上下文和状态变更意图。
13. PostgreSQL 仓储下验证主流程事务和幂等语义，不能只用内存仓储代表最终验收。

## 待确认问题

以下问题不阻塞本需求进入研发，但必须在具体玩法落地前单独确认：

1. 后续真实插件状态持久化采用独立玩法状态表、扩展 quota reservation，还是其他统一 state 模型。
2. 一个活动未来是否允许组合多个插件；本期固定一个活动一个主插件。
3. 红包雨 WebSocket 推送属于插件解释、外部网关订阅还是后续单独任务。
4. 抽奖次数、签到次数、任务进度未来是否统一抽象为 quota/state，还是各玩法维护独立状态。
5. 具体插件 Admin 配置页面和审批流由哪个后续任务承接。

## Score

```text
需求清晰度        19 / 20
范围可控          19 / 20
风险可控          18 / 20
验收标准明确      19 / 20
架构一致性        19 / 20
Total             94 / 100
```

## Decision

```text
PASS
```

需求细节已明确插件上下文路径、冲突规则、snapshot/simulate 输出口径、未知插件处理口径，以及本期无真实插件状态持久化的事务边界。剩余问题属于具体玩法插件落地前的独立设计，不阻塞本任务进入 Review。

需求：2026-05-24-113400-9
