# Analysis

## 需求

`2026-05-24-113400-2`：活动配置统一模型。

## 资料依据

已读取并作为本次分析依据：

- `tasks/2026-05-24-113400-2/task.md`
- `tasks/2026-05-24-113400-2/review.md`
- `README.md`
- `docs/architecture.md`
- `docs/api.md`
- `docs/data-model.md`
- `docs/roadmap.md`
- `docs/operations.md`
- `requirements/backlog.json`
- `/Users/tong/Documents/GitHub/OracleBetX-Projects/act-analysis/22-独立活动营销中台设计.md`

本轮按用户指令只做需求分析和需求细化，不读取代码、不写代码、不改代码、不提交。上一轮 Review 已给出当前执行边界：现有 Outbox worker 只对 `HTTP` / `WEBHOOK` 且 `targetRef` 为 `http(s)` URL 的奖励指令执行投递；`MQ` / `MANUAL_EXPORT` 或缺失合法 `targetRef` 会稳定进入投递失败或 DLQ。本分析将该边界作为验收约束。

## 背景目标

ExMarketing 是独立活动营销中台，目标是将普通活动从“每个活动一套 controller/model/job/topic”收敛为统一配置模型和统一执行链路。

设计文档的长期目标包括：

- 活动定义统一。
- 规则配置统一。
- 奖励发放统一。
- 参与记录、风控幂等、预算库存、审计对账统一。
- 活动类型插件化。
- 对外协议标准化。
- 不依赖具体业务系统，只接收标准事件，只输出标准奖励指令。

本任务是 Phase 1 配置模型收敛，不是把所有长期能力一次做完。当前最小目标是：

- 完善 `marketing_activity`、`marketing_rule`、`marketing_reward` 的创建和读回配置表达。
- 覆盖活动基础信息、时间、入口、参与人群元数据、互斥叠加、发奖方式和版本字段。
- 对 `config`、`ruleConfig`、`rewardConfig` 做基础 Schema 校验。
- 普通活动可通过 `POST /admin/v1/activities` 创建，并能沿当前事件处理链路生成 participation、grant_order、outbox。
- 创建阶段拒绝当前系统不能执行或会稳定失败的配置。

## 参与方

- 运营 / 活动配置人员：通过管理接口创建普通活动，配置活动基础信息、展示入口、人群元数据、规则、奖励、发奖方式。
- 外部业务系统：通过标准事件 API 上报事件；业务扩展字段放入 `attributes`。
- 活动引擎：按活动状态、时间、触发事件、规则和奖励配置执行匹配、资格判断、奖励计算和发奖指令生成。
- 外部发奖系统：接收 HTTP/Webhook Reward Command，并通过回调接口回传发奖结果。
- 运维 / 审计人员：通过活动配置、事件、发奖单、Outbox、DLQ 和审计记录排查问题。
- Dev / Test / Review Agent：按本分析确认的业务边界开发、测试和审核。

## 输入输出

### 管理端输入

`POST /admin/v1/activities` 创建活动请求包含：

- 活动基础信息：`tenantId`、`activityCode`、`activityName`、`activityType`
- 生命周期：`status`、`startTime`、`endTime`、`timezone`
- 触发入口：`triggerEvents`
- 匹配和互斥：`priority`、`exclusiveGroup`、`stackable`
- 活动扩展配置：`config`
- 规则列表：`rules`
- 奖励列表：`rewards`
- 配置版本：activity / rule / reward 的 `version`

### 标准事件输入

本任务不改变事件模型。标准事件字段保持：

- `eventId`
- `eventType`
- `sourceSystem`
- `tenantId`
- `subject`
- `biz`
- `occurredAt`
- `attributes`

业务字段必须进入 `attributes`。活动配置中的规则字段路径只能引用当前系统能读取的数据来源。

### 成功输出

创建成功返回可读回的活动配置，至少应包含：

- 活动核心字段。
- `config`。
- 每条规则的核心字段、`ruleConfig`、`version`。
- 每条奖励的核心字段、`rewardConfig`、`flowMultiple`、`grantMode`、`grantChannel`、`version`。

### 失败输出

非法配置返回 `400 Bad Request`。错误信息必须能定位到配置范围，例如：

- activity 基础字段。
- activity `config`。
- `rules[n].ruleConfig`。
- `rewards[n].rewardConfig`。
- `rewards[n].grantChannel`。
- `version`。

## 统一配置模型

### Activity 核心字段

以下字段由活动核心字段承载，不应在 `config` 中重复表达执行语义：

- `tenantId`
- `activityCode`
- `activityName`
- `activityType`
- `status`
- `startTime`
- `endTime`
- `timezone`
- `triggerEvents`
- `priority`
- `exclusiveGroup`
- `stackable`
- `version`

活动状态允许值：

```text
DRAFT
SCHEDULED
ACTIVE
PAUSED
ENDED
```

创建规则：

- `tenantId`、`activityCode`、`activityName`、`activityType` 必填且非空。
- `status` 如传入，必须属于允许值；如省略，应沿用当前默认语义。
- `startTime`、`endTime` 必填，且 `startTime < endTime`。
- `timezone` 必填，必须是合法时区。
- `triggerEvents` 必须至少包含一个非空事件类型，不能重复。
- `priority` 如传入必须是整数。
- `exclusiveGroup` 可为空字符串；不为空时用于同组互斥语义。
- `stackable` 必须是布尔值；省略时沿用当前默认语义。
- `version` 创建时只允许省略、`0` 或 `1`；创建成功后的初始版本语义为 `1`。
- 不支持创建历史版本或指定大于 `1` 的版本。

### Activity config

`config` 只承载运营展示、入口和人群元数据，不直接参与资格判断、奖励计算、预算扣减或发奖路由。

建议 Schema：

```json
{
  "entry": {
    "channels": ["APP", "WEB"],
    "entranceCodes": ["deposit_page"],
    "landingUrl": "https://example.com/promo/first-deposit"
  },
  "display": {
    "title": "First Deposit Bonus",
    "subtitle": "Deposit and get bonus",
    "bannerUrl": "https://example.com/banner.png",
    "termsUrl": "https://example.com/terms",
    "visibleFrom": "2026-05-01T00:00:00Z",
    "visibleTo": "2026-06-01T00:00:00Z",
    "sort": 100
  },
  "audience": {
    "segmentCodes": ["new_user"],
    "tags": ["first_deposit"],
    "countries": ["CN"]
  },
  "metadata": {
    "owner": "growth"
  }
}
```

业务规则：

- `config` 可省略或为 `null`。
- 如出现，`config` 必须是 object。
- 允许的一层字段仅为 `entry`、`display`、`audience`、`metadata`。
- `entry.channels`、`entry.entranceCodes`、`audience.segmentCodes`、`audience.tags`、`audience.countries` 必须是字符串数组；数组内元素不可为空。
- URL 字段如出现，必须是合法 `http` 或 `https` URL。
- `display.visibleFrom`、`display.visibleTo` 如出现，必须是合法时间；两者同时出现时 `visibleFrom <= visibleTo`。
- `display.sort` 如出现，必须是整数。
- `metadata` 只允许保存非执行语义的附加信息，不得用于绕过规则、奖励、quota 或发奖链路。
- `audience` 只表示展示、人群圈选或运营标签元数据，不等同于参与资格；真正资格条件必须写入 `rules`。

## Rule 配置

当前阶段只允许创建系统已能执行的普通条件规则。

规则核心字段：

- `ruleCode`
- `rulePhase`
- `ruleType`
- `ruleConfig`
- `sort`
- `enabled`
- `version`

规则阶段允许值：

```text
MATCH
QUALIFY
REWARD
RISK
LIMIT
```

当前允许的规则类型：

```text
CONDITION
```

`CONDITION` 配置：

```json
{
  "logic": "AND",
  "conditions": [
    {"field": "attributes.amount", "op": ">=", "value": "100.00"}
  ]
}
```

业务规则：

- `rules` 可为空；空规则表示没有额外资格条件，活动仍需满足租户、状态、时间和触发事件匹配。
- 如传入规则，每条规则必须结构合法。
- `ruleCode` 必填且非空。
- `rulePhase` 必须属于允许值。
- `ruleType` 只能是 `CONDITION`。
- `FORMULA`、`LIMIT`、`SCRIPT` 等未开放规则类型必须在创建阶段返回 `400`。
- `ruleConfig` 必须是 object。
- `logic` 可省略，语义为 `AND`；如传入，只能是 `AND` 或 `OR`。
- `conditions` 可为空数组，表示该条规则恒通过。
- `conditions[n].field` 必须是非空字符串。
- `conditions[n].op` 必须属于当前条件操作符。
- `conditions[n].value` 的类型必须与操作符匹配。
- `sort` 如传入必须是整数。
- `enabled` 如传入必须是布尔值；禁用规则是否参与执行沿用当前系统语义，但创建时必须可保存并读回。
- `version` 创建时只允许省略、`0` 或 `1`；创建成功后为 `1`。

当前条件操作符：

```text
=
==
!=
<>
>
>=
<
<=
IN
NOT_IN
```

字段路径边界：

- 当前最小可验收范围必须支持标准事件顶层字段和 `attributes.*`。
- 金额或数值比较推荐使用 `attributes.*` 内的 Decimal 字符串，例如 `attributes.amount`。
- `facts.*` 依赖 Fact Provider 配置中心和 provider 注册校验，属于后续需求；本任务内若未配置 provider，不允许创建依赖 `facts.*` 的规则。
- `state.*`、周期累计、排行榜、签到连续天数等活动状态字段属于后续能力；本任务不得用这些字段假设普通活动可执行。

操作符值规则：

- `IN` / `NOT_IN` 的 `value` 必须是数组。
- `>`、`>=`、`<`、`<=` 用于金额或数值字段时，`value` 必须能按 Decimal 语义解析。
- Decimal 校验不允许以 float64 精度作为验收依据。

## Reward 配置

当前阶段只允许创建系统已能计算且能投递的奖励配置。

奖励核心字段：

- `rewardCode`
- `rewardType`
- `calculateType`
- `rewardConfig`
- `grantMode`
- `grantChannel`
- `flowMultiple`
- `enabled`
- `version`

### 当前最小可执行边界

本任务必须收紧为以下边界：

- `calculateType` 只允许 `FIXED`、`RATIO`。
- `grantMode` 只允许 `AUTO`。
- `grantChannel` 只允许 `HTTP`、`WEBHOOK`。
- `rewardConfig.targetRef` 对 `HTTP` / `WEBHOOK` 必填，且必须是合法 `http` 或 `https` URL。
- `MQ`、`MANUAL_EXPORT` 在对应 sender / 导出流程实现前，创建活动必须返回 `400`。
- `grantMode=MANUAL` 在人工处理状态、发奖单状态、是否生成 outbox、人工导出/关闭流程未定义前，创建活动必须返回 `400`。

该边界是本任务 PASS 的关键验收条件。不得允许创建会稳定进入 Outbox 投递失败或 DLQ 的活动。

### 奖励类型

设计目标中的奖励类型包括：

```text
BONUS
CASH
COUPON
POINT
GIFT
DRAW_CHANCE
MEMBERSHIP
```

当前普通活动最小可验收建议开放：

```text
BONUS
CASH
POINT
```

原因：

- 当前已实现计算方式是金额/数值型 `FIXED`、`RATIO`。
- `GIFT`、`DRAW_CHANCE`、`MEMBERSHIP` 通常需要额外 payload、库存、权益系统或玩法上下文，不能仅靠金额型 reward_amount 明确执行。
- `COUPON` 是否能仅通过 HTTP/Webhook command 执行，取决于外部发奖系统 payload 约定；如没有明确配置 Schema，本任务应先拒绝，避免创建后外部不可识别。

如 Dev 在现有系统中确认 `COUPON` 等类型只是透传 `rewardType + rewardAmount + payload` 且当前外部发奖接口可处理，可在实现说明中扩展允许值；但验收仍必须保证创建后不会因类型缺少 Schema 或投递目标而稳定失败。

### 通用业务规则

- `rewards` 必须至少包含一条 enabled 或可执行奖励；普通活动不能无奖励创建。
- 每条 reward 必须有非空 `rewardCode`。
- `rewardType` 必须属于本任务开放的可执行奖励类型。
- `calculateType` 只能是 `FIXED` 或 `RATIO`。
- `LADDER`、`RANDOM`、`PROBABILITY`、`POOL_SPLIT`、`DRAW_CHANCE`、`GIFT` 等待实现计算方式必须在创建阶段返回 `400`。
- `rewardConfig` 必须是 object。
- `rewardConfig.targetRef` 必填，且必须是合法 `http` 或 `https` URL。
- `grantMode` 必须是 `AUTO`。
- `grantChannel` 必须是 `HTTP` 或 `WEBHOOK`。
- `flowMultiple` 和 `rewardConfig.flowMultiple` 如出现，必须按 Decimal 字符串或可精确解析的 Decimal 语义校验。
- 顶层 `flowMultiple` 与 `rewardConfig.flowMultiple` 不允许表达冲突值；冲突时返回 `400`。
- `enabled` 如传入必须是布尔值；禁用奖励是否参与执行沿用当前系统语义，但创建时不得导致普通活动无可执行奖励。
- `version` 创建时只允许省略、`0` 或 `1`；创建成功后为 `1`。

### FIXED rewardConfig

```json
{
  "amount": "20.00",
  "currency": "CNY",
  "flowMultiple": "3",
  "targetRef": "https://wallet.example.com/reward"
}
```

规则：

- `amount` 必填。
- `amount` 必须是大于 `0` 的 Decimal 字符串。
- `currency` 如出现必须是非空字符串。
- `targetRef` 必填且必须是 `http` 或 `https` URL。

### RATIO rewardConfig

```json
{
  "baseField": "attributes.amount",
  "rate": "0.20",
  "maxAmount": "500.00",
  "currency": "CNY",
  "flowMultiple": "3",
  "targetRef": "https://wallet.example.com/reward"
}
```

规则：

- `baseField` 必填。
- 当前普通活动最小可验收范围为 `attributes.*` 数值字段。
- `rate` 必填，必须是大于 `0` 的 Decimal 字符串。
- `maxAmount` 如出现，必须是大于 `0` 的 Decimal 字符串。
- `currency` 如出现必须是非空字符串。
- `targetRef` 必填且必须是 `http` 或 `https` URL。

## 发奖模式与链路语义

### AUTO

`grantMode=AUTO` 表示事件命中且规则通过后，系统按当前统一链路处理：

1. 生成 `REWARDABLE` participation。
2. 扣减 BUDGET / STOCK / USER_LIMIT 等 quota。
3. 生成 `marketing_grant_order`。
4. 生成 `marketing_outbox`。
5. Outbox worker 对 `HTTP` / `WEBHOOK` 的 `targetRef` 投递 Reward Command。
6. 外部发奖系统通过 callback 更新发奖结果。

本任务只允许这个可执行路径。

### MANUAL

`grantMode=MANUAL` 当前需求未定义以下关键语义：

- 是否生成 `grant_order`。
- `grant_order` 初始状态是什么。
- 是否生成 `outbox`。
- 人工处理入口在哪里。
- 人工成功、失败、关闭如何回写。
- 是否需要释放或确认 quota。

因此本任务内 `MANUAL` 不是可创建配置，必须返回 `400`。人工发奖应另开需求定义完整状态机、审计和验收。

### MQ / MANUAL_EXPORT

`MQ`、`MANUAL_EXPORT` 属于长期设计目标或后续生产化能力。当前任务内：

- `grantChannel=MQ` 必须返回 `400`，直到 MQ sender 实现并有端到端验收。
- `grantChannel=MANUAL_EXPORT` 必须返回 `400`，直到人工导出、状态回写、审计和补偿流程定义完成。

## 边界条件

- 顶层未知字段应拒绝，避免 API 使用方误以为配置生效。
- `config`、`ruleConfig`、`rewardConfig` 的执行语义字段必须受 Schema 校验；未知执行字段应拒绝。
- `metadata` 可作为非执行元数据扩展容器，但不能影响活动命中、资格判断、奖励计算或发奖路由。
- 空字符串、空数组、重复 `triggerEvents`、非法枚举、非法时间、非法时区都应在创建阶段失败。
- 规则列表为空是合法语义，表示无额外资格条件。
- 奖励列表为空或没有可执行 enabled 奖励不可创建。
- 活动互斥叠加完整运行时策略不在本任务内；本任务只保证字段可配置、可存储、可读回、基础校验明确。
- `audience` 不参与资格判定。
- 配置版本只表达初始版本字段补齐，不表达审批流、版本历史、回滚或 diff。
- Quota 配置不属于本任务的创建活动请求；缺少 quota 行仍按当前文档表示不限制。
- Fact Provider 配置中心不属于本任务；不得让 `facts.*` 规则在 provider 未配置时创建成功。

## 异常场景

- 非法 JSON：返回 `400`。
- 顶层未知字段：返回 `400`。
- `tenantId` / `activityCode` / `activityName` / `activityType` 缺失或为空：返回 `400`。
- `triggerEvents` 缺失、为空、含空字符串或重复：返回 `400`。
- `startTime >= endTime`：返回 `400`。
- `timezone` 缺失或非法：返回 `400`。
- `status` 非法：返回 `400`。
- `config` 不是 object：返回 `400`。
- `config` 包含未知一层字段：返回 `400`。
- `entry` / `display` / `audience` 字段类型错误：返回 `400`。
- URL 字段格式非法：返回 `400`。
- `visibleFrom > visibleTo`：返回 `400`。
- `ruleType` 不是 `CONDITION`：返回 `400`。
- `ruleConfig` 不是 object：返回 `400`。
- `ruleConfig.logic` 非法：返回 `400`。
- `conditions[n].field` 为空：返回 `400`。
- `conditions[n].op` 非法：返回 `400`。
- `IN` / `NOT_IN` 的 `value` 不是数组：返回 `400`。
- 数值比较值不能按 Decimal 解析：返回 `400`。
- `facts.*` 字段没有已配置 provider：返回 `400`。
- `rewards` 为空或无可执行奖励：返回 `400`。
- `rewardType` 不在本任务开放范围：返回 `400`。
- `calculateType` 不是 `FIXED` / `RATIO`：返回 `400`。
- `FIXED` 缺少 `amount`、金额非 Decimal、金额小于等于 `0`：返回 `400`。
- `RATIO` 缺少 `baseField` / `rate`，`rate` 非 Decimal 或小于等于 `0`：返回 `400`。
- `RATIO.maxAmount` 非 Decimal 或小于等于 `0`：返回 `400`。
- `grantMode=MANUAL`：返回 `400`。
- `grantChannel=MQ` 或 `MANUAL_EXPORT`：返回 `400`。
- `grantChannel=HTTP` / `WEBHOOK` 但 `rewardConfig.targetRef` 缺失、为空或不是 `http(s)` URL：返回 `400`。
- 顶层 `flowMultiple` 与 `rewardConfig.flowMultiple` 冲突：返回 `400`。
- activity / rule / reward 的 `version` 大于 `1` 或非法：返回 `400`。

## 非目标范围

- 不新增普通活动专属 controller、model、job、Kafka topic 或业务表。
- 不实现 `LADDER`、`RANDOM`、`PROBABILITY`、`POOL_SPLIT` 等计算方式。
- 不实现 `FORMULA`、`LIMIT`、`SCRIPT` 规则执行能力。
- 不实现 MQ Event In。
- 不实现 MQ Command Out。
- 不实现人工导出发奖。
- 不实现 `grantMode=MANUAL` 的状态机、后台处理、审计和补偿。
- 不实现审批流、发布流、编辑、版本历史、回滚、配置 diff。
- 不实现完整后台配置页面。
- 不改变标准事件接入幂等规则。
- 不改变 participation、quota、grant_order、outbox 的原子链路。
- 不改变 Fact Query 安全边界。
- 不接入外部业务库，不引用外部业务代码，不直接修改用户余额。
- 不将会员、订单、存款、游戏、风控等外部领域建成系统内置强模型。

## 数据和 API 影响

### 数据模型

- `marketing_activity.config_json` 承载 activity `config`。
- `marketing_activity.version` 表达活动配置初始版本。
- `marketing_rule.rule_config_json` 承载 `ruleConfig`。
- `marketing_rule.version` 表达规则配置初始版本。
- `marketing_reward.reward_config_json` 承载 `rewardConfig`。
- `marketing_reward.flow_multiple` 承载顶层流水倍数。
- `marketing_reward.grant_mode` 当前只允许 `AUTO`。
- `marketing_reward.grant_channel` 当前只允许 `HTTP` / `WEBHOOK`。
- `marketing_reward.version` 是任务要求覆盖的奖励配置版本字段；如果当前持久化模型尚未具备该字段，本任务需补齐语义，旧数据读回默认 `1`。

### API

`POST /admin/v1/activities`：

- 不新增路径。
- 合法普通活动返回 `201 Created`。
- 请求可包含 `config` 和 activity / rule / reward 的 `version`。
- 非法配置统一在创建阶段返回 `400 Bad Request`。
- 未实现或当前不可投递的发奖配置从“可保存但运行时失败”收紧为“创建失败”。

`GET /admin/v1/activities`：

- 应能读回活动 `config`。
- 应能读回 rule `version`。
- 应能读回 reward `version`。
- 应能读回 reward `targetRef`、`grantMode`、`grantChannel` 等创建时配置。

## 风险

- P1：如果继续允许 `MQ`、`MANUAL_EXPORT` 或缺失 `targetRef` 的 `HTTP` / `WEBHOOK`，活动会创建成功但 Outbox 稳定失败或进入 DLQ，违反“普通活动可通过配置创建和执行”。
- P1：如果允许 `grantMode=MANUAL` 但仍生成 outbox，会造成语义和状态机不一致；如果不生成 outbox 又无人工处理入口，也无法验收。
- P1：奖励版本字段如果只在 API 层出现、持久化读回缺失，会造成配置不可审核。
- P1：Decimal 校验如果用 float64 语义处理，会引入金额、比例、封顶和 quota 精度风险。
- P1：`audience` 容易被误解为资格规则，可能导致运营配置与实际参与资格不一致。
- P2：严格 Schema 会拒绝历史上能保存但运行时无效的配置；这是本任务有意将失败前移，需要文档明确。
- P2：开放过多 rewardType 可能导致外部发奖系统无法解释 payload；本任务应以可执行、可验收为准。
- P2：未来扩展字段与严格 Schema 存在冲突；后续新增执行字段必须通过需求更新 Schema 和测试。

## 验收标准

- 普通活动可通过 `POST /admin/v1/activities` 创建，不需要新增活动专属 controller、model、job 或 Kafka topic。
- 创建请求能覆盖活动基础信息、时间、入口、展示、人群元数据、互斥叠加、规则、奖励、发奖方式和版本字段。
- 合法 `CONDITION + FIXED + AUTO + HTTP targetRef=http(s)` 活动返回 `201`。
- 合法 `CONDITION + RATIO + AUTO + WEBHOOK targetRef=http(s)` 活动返回 `201`。
- 合法无规则活动可创建，语义为没有额外资格条件。
- 创建后 `GET /admin/v1/activities` 能读回 activity `config`、rule `version`、reward `version`、reward `targetRef`。
- 上报匹配事件后，合法活动仍能沿现有链路生成 participation、grant_order、outbox。
- Outbox 中 `target_channel` 只能是本任务允许的 `HTTP` / `WEBHOOK`，`target_ref` 必须是创建时配置的 `http(s)` URL。
- `grantChannel=MQ` 返回 `400`。
- `grantChannel=MANUAL_EXPORT` 返回 `400`。
- `grantMode=MANUAL` 返回 `400`。
- `grantChannel=HTTP` / `WEBHOOK` 但缺少 `rewardConfig.targetRef` 返回 `400`。
- `rewardConfig.targetRef` 不是 `http` 或 `https` URL 返回 `400`。
- 非法 activity `config` 返回 `400`，包括未知字段、字段类型错误、非法 URL、展示时间倒挂。
- 非法 `triggerEvents` 返回 `400`，包括空列表、空字符串、重复事件。
- 非法 `timezone`、`status` 或活动时间窗口返回 `400`。
- 非法 `ruleConfig` 返回 `400`，包括未知执行字段、非法 `logic`、非法 `op`、Decimal 比较值非法、`IN` / `NOT_IN` value 非数组。
- 未实现 `ruleType=FORMULA`、`LIMIT`、`SCRIPT` 返回 `400`。
- 非法 `rewardConfig` 返回 `400`，包括 `FIXED` 缺少 `amount`、金额非 Decimal、金额非正数、`RATIO` 缺少 `baseField` / `rate`、`rate` 非正数、`maxAmount` 非 Decimal 或非正数。
- 未实现 `calculateType=LADDER`、`RANDOM`、`PROBABILITY`、`POOL_SPLIT` 返回 `400`。
- activity / rule / reward 的 `version` 大于 `1` 返回 `400`；省略时读回为 `1`。
- `marketing_reward.version` 在持久化存储中可写入并读回；旧数据默认读回 `1`。
- 文档包含统一配置模型说明、发奖可执行边界、版本字段说明、`audience` 与规则资格的边界、非法配置 `400` 示例和可运行 curl 示例。

## 验收方式

最低验证：

```bash
go test ./...
go build ./...
```

涉及 PostgreSQL 兼容的验证：

```bash
docker compose up -d postgres
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./internal/postgres -run Test
```

建议测试矩阵：

- 创建合法 `CONDITION + FIXED + AUTO + HTTP + http(s) targetRef` 活动。
- 创建合法 `CONDITION + RATIO + AUTO + WEBHOOK + http(s) targetRef` 活动。
- 查询活动读回 `config`、rule version、reward version、grant config。
- 上报匹配事件后验证 participation、grant_order、outbox 链路未被破坏。
- 验证 outbox `target_channel` 和 `target_ref` 与奖励配置一致。
- activity config 未知字段、非法类型、非法 URL、展示时间倒挂返回 `400`。
- `triggerEvents` 为空、空字符串、重复返回 `400`。
- 非法 timezone 和非法 status 返回 `400`。
- CONDITION 非法 logic/op 返回 `400`。
- Decimal 比较值非法返回 `400`。
- IN/NOT_IN value 非数组返回 `400`。
- 未支持 ruleType 返回 `400`。
- FIXED 缺 amount、amount 非正数、amount 非 Decimal 返回 `400`。
- RATIO 缺 baseField/rate、rate 非正数、maxAmount 非正数返回 `400`。
- 未支持 calculateType 返回 `400`。
- `grantMode=MANUAL` 返回 `400`。
- `grantChannel=MQ` / `MANUAL_EXPORT` 返回 `400`。
- HTTP/WEBHOOK 缺 `targetRef` 或 `targetRef` 非 `http(s)` URL 返回 `400`。
- version 大于 `1` 返回 `400`。
- PostgreSQL 下旧奖励数据默认 version 为 `1`，新建奖励读回 version 为 `1`。

## 待确认问题

以下问题不阻断本任务，因为本分析已给出保守的最小可验收边界；未确认前一律不得扩大创建能力。

- `COUPON`、`GIFT`、`DRAW_CHANCE`、`MEMBERSHIP` 是否已有稳定外部发奖 payload 约定？未确认前不纳入本任务可创建范围。
- 顶层 `flowMultiple` 与 `rewardConfig.flowMultiple` 的长期推荐来源是哪一个？本任务要求冲突即拒绝。
- `ruleConfig.field` 是否要在本任务内支持 `occurredAt` 时间比较？当前最低验收只要求标准字段边界清楚，复杂时间窗口规则可后续定义。
- `display.visibleFrom/visibleTo` 是否必须落在活动 `startTime/endTime` 内？本任务只要求格式合法和前后顺序合法。
- `enabled=false` 的规则和奖励在当前系统的执行过滤语义是否完全一致？本任务只要求类型校验、保存、读回，以及普通活动至少存在可执行奖励。

## Score

```text
需求清晰度        19/20
范围可控          19/20
风险可控          18/20
验收标准明确      19/20
架构一致性        18/20
总分              93/100
```

## Decision

```text
PASS
```
