# Analysis

## 需求 ID

2026-05-24-113929-7

## 标题

模拟运行解释器

## Review 反馈处理

本轮已读取 `tasks/2026-05-24-113929-7/review.md`，并逐条处理 Review Agent 的打回意见。本轮仅做需求分析返工，未读取实现代码，未写代码。

### P0

- 无。

### P1

1. Review 指出上一版对 Fact 依赖的口径会破坏“模拟运行解释器”的核心一致性目标：一方面要求 `POST /admin/v1/simulate` 复用正式链路语义，另一方面又规定规则引用 `facts.*` 时不主动调用外部 Fact Provider，导致依赖 Fact Query 的活动在模拟运行中被系统性误判为缺失或拒绝。

处理：

- 已修正为：模拟运行默认复用正式事件链路的只读 Fact Query 语义。
- 当候选活动实际引用 `facts.<factCode>.*` 且正式链路会触发 Fact Query 时，模拟运行也应按同一 registry、同一标准事件上下文、同一 mapping、同一缓存和同一安全边界查询 fact。
- 该查询必须是只读解释行为：不得写 `marketing_event_inbox`、`marketing_participation`、`marketing_grant_order`、`marketing_outbox`、`marketing_quota`，不得投递发奖指令，不得消费插件状态。
- Provider 失败、超时、mapping 失败或映射后字段缺失时，不应把活动静默通过；依赖该 fact 的规则输出可解释原因，原因码最小口径为 `FACT_QUERY_FAILED`、`FACT_MAPPING_FAILED`、`FACT_FIELD_MISSING`。
- “完全离线、不查询外部 Fact Provider”的能力不作为本轮默认模拟语义；如后续需要，应作为显式降级模式单独定义，并明确该模式不保证与依赖 `facts.*` 的正式链路一致。

### P2

1. Review 指出审计日志口径不清晰，容易让 Dev/Test Agent 对 `marketing_audit` 是否允许写入产生分歧。

处理：

- 已明确：模拟运行允许写一条只读操作审计 `SIMULATION_RUN` 到 `marketing_audit`，该审计用于后台操作追踪，不属于 participation、grant_order、outbox、quota 等业务执行副作用。
- 审计内容必须脱敏，不得记录 secret、token、完整 Authorization、Fact Provider 明文密钥、外部 provider auth 配置、完整敏感 attributes 或完整 provider 响应。
- 无副作用验收中的“不得写业务执行表”明确指 `marketing_event_inbox`、`marketing_participation`、`marketing_grant_order`、`marketing_outbox`、`marketing_quota` 及插件玩法状态表；`marketing_audit` 的脱敏 `SIMULATION_RUN` 记录允许存在。

### Open Questions

- Review 提问“是否允许模拟运行在规则引用 `facts.*` 时执行正式链路同等的只读 Fact Query”。本轮默认答案为允许，并作为最小可验收口径，以关闭旧 BLOCK。推导依据是任务目标要求回答“如果按当前配置处理，这条事件会命中哪些活动”，且现有 API / 数据模型文档已经定义 Fact Query 是规则引用 `facts.*` 时才触发的正式链路可选能力。

## Analysis 自检修正

本轮已读取上一版 `tasks/2026-05-24-113929-7/analysis.md`。上一版自评 `Score 94 / Decision PASS`，但 Review 判定 `Score 78 / Decision BLOCK`，需要修正以下错误假设和扣分项：

- 错误假设一：把“无副作用”扩大解释为“不主动调用外部 Fact Provider”。本轮修正为：无副作用约束的核心是不写业务执行表、不投递发奖、不消费状态；只读 Fact Query 可参与模拟解释。
- 错误假设二：认为缺失 fact 时一律按 `FACT_FIELD_MISSING` 拒绝即可。本轮修正为：若正式链路会查询 provider，模拟运行必须先尝试只读 Fact Query；只有 provider 查询失败、mapping 失败或映射后字段仍缺失时，才输出相应原因。
- 扣分项：Fact Query 口径导致模拟结果与真实执行链路在依赖 facts 的活动上系统性不一致。本轮改为复用正式只读 Fact Query 语义，关闭该扣分项。
- 待确认问题：“是否需要模拟运行主动调用 Fact Query Provider？”上一版列为非阻塞待确认且默认不支持。本轮改为默认支持正式链路等价的只读 Fact Query，不再作为阻塞待确认问题。
- 审计口径不清：上一版仅说“默认不写任何业务执行表”。本轮明确允许脱敏 `SIMULATION_RUN` 审计，并说明它不属于业务执行副作用。

旧 BLOCK 已关闭。本轮不等待 Owner 决策；在现有材料没有额外产品开关定义的情况下，采用“模拟运行与正式链路解释一致优先，副作用隔离通过禁止业务写入和外部发奖保证”的默认最小可验收口径。

## 依据材料

本轮重新读取并参考了以下需求和文档材料：

- `tasks/2026-05-24-113929-7/task.md`
- `tasks/2026-05-24-113929-7/review.md`
- `tasks/2026-05-24-113929-7/analysis.md`
- `docs/api.md`
- `docs/architecture.md`
- `docs/data-model.md`
- `tasks/2026-05-24-fact-query-2/analysis.md`
- `tasks/2026-05-24-match/analysis.md`
- `tasks/2026-05-24-quota/analysis.md`

## 背景目标

ExMarketing 是独立活动营销中台。正式事件链路接收标准事件后，会执行活动匹配、规则判断、可选 Fact Query、奖励计算、Quota 预算库存判断、participation 记录、grant order 生成和 outbox 发奖指令投递。

模拟运行解释器用于运营、测试和研发排障：输入一条事件样例，解释“如果按当前配置处理，这条事件会命中哪些活动、为什么未命中、每条规则为何通过或未通过、预计奖励是多少、预算/库存/次数会如何变化、最终发奖指令会是什么”。

核心目标：

- 模拟解释结果尽量与正式事件链路在同一配置、同一事件、同一 Quota 快照和同一 Fact Query 能力下保持一致。
- 输出结构化、可审核、可测试的规则、奖励、Quota、participation、grant order、outbox 预览。
- 保证 preview-only：不写业务执行表、不锁定 Quota、不发放奖励、不投递 outbox、不消费插件状态。
- 允许后台写脱敏只读操作审计，便于追踪谁在何时执行了模拟。

默认最小可验收口径：

- 使用现有管理端接口 `POST /admin/v1/simulate`。
- 输入是一条标准事件 JSON。
- 候选、规则、奖励、互斥、Quota 和 Fact Query 语义复用正式链路。
- 只读读取活动配置、规则、奖励、Quota 快照、Fact Provider registry、Fact Query 成功缓存和必要营销内部状态。
- 不写 `marketing_event_inbox`、`marketing_participation`、`marketing_grant_order`、`marketing_outbox`、`marketing_quota`。
- 不触发 Outbox worker，不调用外部发奖 HTTP/Webhook/MQ，不修改用户余额。
- 不消费插件玩法状态，只输出状态变更意图。
- 允许写脱敏 `marketing_audit` / `SIMULATION_RUN` 只读操作审计。

## 参与方

- 运营 / 活动配置人员：提交事件样例，检查活动配置是否按预期命中、未命中和预览发奖。
- 测试人员：构造金额、时间、规则、互斥、Fact Query、Quota、奖励为 0、插件状态等边界事件。
- 研发 / 排障人员：定位规则未通过、Fact Query 失败、奖励不可计算、Quota 超限、插件前置校验失败等原因。
- ExMarketing 管理端：提供本地后台和模拟运行 API。
- ExMarketing 引擎：提供活动匹配、规则解释、奖励预览、Quota 预览、Fact Query 只读解释、发奖命令预览。
- Fact Provider：仅在活动规则实际引用 `facts.*` 且正式链路需要该 fact 时，作为只读事实查询来源参与模拟。
- 外部发奖系统：不参与模拟运行；模拟结果只展示将要对接的指令形态，不真实投递。

## 输入

模拟运行输入是一条标准事件 JSON，与真实事件接入模型一致，至少包含：

- `eventId`
- `eventType`
- `sourceSystem`
- `tenantId`
- `subject.type`
- `subject.id`
- `biz.bizType`
- `biz.bizId`
- `occurredAt`
- `attributes`

字段约束：

- `occurredAt` 必须是带时区信息的 RFC3339 时间。
- 业务字段必须放在 `attributes`；不得把 `orderAmount`、`deposit.amount`、`user.vipLevel` 等业务字段作为非标准顶层字段。
- `attributes` 必须是 JSON object，可包含金额、币种、渠道、标签、风险等级、首存标记、设备、IP 等规则或 Fact Provider 输入需要的字段。
- 幂等键语义仍按 `tenantId + ":" + sourceSystem + ":" + eventType + ":" + bizType + ":" + bizId` 推导，仅用于解释 participation / grant order / outbox 的预览幂等字段。
- 默认不支持调用方指定 `activityId` 做单活动模拟；模拟范围是同 tenant、同 trigger event 的当前候选活动。
- 默认不支持请求体携带 inline Fact Provider 配置、外部 endpoint、auth、mapping、timeout 或 TTL。Fact Query 只能使用已注册且 enabled 的 provider registry。
- 请求可携带 trace 或操作者上下文用于审计和排障，但不得影响规则判断、奖励计算、Fact Query 缓存 key 或业务结果。

示例输入：

```json
{
  "eventId": "evt_sim_001",
  "eventType": "DEPOSIT_SUCCESS",
  "sourceSystem": "payment",
  "tenantId": "site_001",
  "subject": {"type": "USER", "id": "10001"},
  "biz": {"bizType": "DEPOSIT", "bizId": "pay_sim_001"},
  "occurredAt": "2026-05-24T12:00:00Z",
  "attributes": {
    "amount": "299.00",
    "currency": "CNY",
    "isFirstDeposit": true,
    "channel": "APP"
  }
}
```

## 输出

模拟运行响应必须是结构化 JSON，至少包含：

- `simulatedAt`：服务端模拟时间。
- `summary.candidateCount`：候选活动数。
- `summary.matchedCount`：最终命中活动数。
- `summary.unmatchedCount`：未命中活动数。
- `summary.hasRewardableActivity`：是否存在可生成发奖预览的活动。
- `summary.hasQuotaRisk`：是否存在预算、库存或次数限制风险。
- `matchedActivities`：最终命中活动列表。
- `unmatchedActivities`：未命中、跳过、互斥压制、Quota 拒绝或不可解释活动列表。

活动级结果至少包含：

- 活动身份：`activityId`、`tenantId`、`activityCode`、`activityName`、`activityType`、`version`。
- 活动级决策：`decision`、`decisionReasonCode`、`decisionReason`。
- `ruleResult`：按阶段和排序输出规则解释。
- `factQueryResults`：活动依赖的 Fact Query 查询、缓存、成功或失败解释。
- `rewardPreviews`：每个 enabled reward 的预计结果、金额或权益 payload、计算明细。
- `quotaImpactPreviews`：预算、库存、用户次数、每日次数等会如何预览占用或是否超限。
- `participationPreview`：如果真实执行将产生的参与快照关键字段。
- `grantOrderPreviews`：如果真实执行将产生的发奖单预览。
- `outboxCommandPreviews`：如果真实执行将投递的发奖指令预览。
- `plugin`：如活动绑定插件，展示 `pluginCode`、插件上下文、前置检查结果和状态变更意图。

活动级 `decision` 机器枚举必须沿用现有 `RuleDecision`：

```text
PASSED
REJECTED
SKIPPED
ERROR
```

含义：

- `PASSED`：活动最终命中，且当前预览可解释。
- `REJECTED`：活动未命中、规则未通过、互斥压制、Quota 超限或奖励无法形成可执行预览。
- `SKIPPED`：活动或规则被显式禁用、跳过，且不参与最终命中判断。
- `ERROR`：活动配置、规则、奖励、Fact Query 或内部读取在运行时不可解释。

禁止新增以下活动级机器枚举：

```text
MATCHED
UNMATCHED
FAILED
```

规则级解释字段最小口径：

- `ruleCode`
- `rulePhase`
- `ruleType`
- `sort`
- `status`
- `passed`
- `reasonCode`
- `reason`
- `field`
- `expected`
- `actual`

规则级 `status` 机器枚举同样必须沿用现有 `RuleDecision`：

```text
PASSED
REJECTED
SKIPPED
ERROR
```

含义：

- `PASSED`：规则通过。
- `REJECTED`：规则已执行但条件未通过，例如字段缺失、值不匹配、金额低于阈值、Fact 字段不可用。
- `SKIPPED`：规则 `enabled=false` 或被阶段逻辑明确跳过，不影响活动决策。
- `ERROR`：规则配置非法、操作符非法、Decimal 解析失败、类型不兼容、evaluator 不可用或 Fact Query 运行错误导致无法正常判断。

业务描述可以写“规则失败”“未通过”，但 JSON 机器值不得输出 `FAILED`。

Fact Query 输出最小口径：

- `factCode`
- `requiredBy`：引用该 fact 的活动、规则或奖励。
- `queried`：是否实际触发查询。
- `cached`：是否命中成功 fact 缓存。
- `status`：`PASSED / REJECTED / ERROR`。
- `reasonCode`：`FACT_QUERY_FAILED / FACT_MAPPING_FAILED / FACT_FIELD_MISSING` 等。
- `reason`：脱敏的人可读原因。
- `fields`：被规则引用的 `facts.<factCode>.*` 字段路径及是否可用。

默认输出排序：

- 候选活动先按正式命中逻辑使用的优先级排序；同优先级按活动 ID 或创建顺序稳定排序。
- 规则按业务阶段 `MATCH -> QUALIFY -> REWARD -> RISK_LIMIT_BUDGET` 输出，阶段内按 `sort` 升序。
- Fact Query 按活动解释顺序和 `factCode` 稳定排序。
- 奖励预览按活动 reward 配置顺序。
- Quota 预览按 `BUDGET -> STOCK -> USER_LIMIT -> DAILY_LIMIT` 输出。

## 业务规则

### 活动候选

- 只评估与输入事件 `tenantId` 一致的活动。
- 只评估 `triggerEvents` 包含输入 `eventType` 的活动。
- 活动状态为 `ACTIVE` 且时间有效时才可最终命中。
- `DRAFT`、`SCHEDULED`、`PAUSED`、`ENDED` 等非当前可运行状态不得静默丢弃，应进入解释结果并给出 `REJECTED` 或 `SKIPPED` 及原因。
- 活动时间判断使用活动配置 `timezone` 和事件 `occurredAt`。
- `display.visible=false` 不影响模拟运行。
- `entry.enabled=false` 应在 MATCH 阶段拒绝候选，活动 `decision=REJECTED`，原因码建议为 `ENTRY_DISABLED`。
- `entry.channels`、`entry.sourceSystems`、`entry.tenantIds`、`entry.audience.*` 按现有 API 文档规则输出对应不允许原因。

### 规则执行

- 规则执行顺序固定为 `MATCH -> QUALIFY -> REWARD -> RISK_LIMIT_BUDGET`。
- `RISK_LIMIT_BUDGET` 是业务阶段名称；实际存储规则阶段仍使用 `MATCH`、`QUALIFY`、`REWARD`、`RISK`、`LIMIT`。
- `BUDGET` 是 Quota 类型，不是合法 `rulePhase`。
- `enabled=false` 的规则必须在解释结果中显示为 `SKIPPED`，且不影响活动决策。
- `CONDITION` 规则按 `logic=AND/OR` 和条件列表执行。
- 字段缺失、类型不匹配、条件值不满足应输出 `status=REJECTED`。
- 非法操作符、Decimal 解析失败、未注册 evaluator、规则配置结构非法应输出 `status=ERROR`；若该规则影响活动是否可解释，活动级 `decision=ERROR`。
- 字段路径仅允许标准事件字段、`attributes.*`、`facts.<factCode>.*`、`state.activity.*`、`state.subject.*`、`state.quota.*`、`state.exclusive.*`、绑定插件时的 `state.plugin.<pluginCode>.*`。
- 模拟运行不得把非标准顶层业务字段当作有效规则输入。

### Fact 依赖

- Fact Query 是可选事实补充能力，但只要当前候选活动实际引用 `facts.<factCode>.*` 且正式事件链路需要该 fact，模拟运行必须按正式链路同等只读语义触发查询。
- 未引用 `facts.*` 的活动不得为了模拟而额外查询 provider。
- 能通过事件 `attributes` 携带的数据，不应为了同一事实再实时查询 provider；是否查询以正式链路的 fact 依赖识别结果为准。
- Fact Query 只能按 `tenantId + factCode` 查找 enabled provider registry；禁止请求方通过 simulate body 覆盖 endpoint、auth、mapping、timeout、TTL 或 provider 配置。
- Provider 输入上下文使用规范化标准事件上下文，包括 `tenantId`、`factCode`、`subject`、`biz`、`attributes`，以及实际存在的 `eventId`、`eventType`、`sourceSystem`、`occurredAt`。
- `traceId`、请求时间、调用方 header、Authorization、随机数、运行实例 ID 不参与缓存 key，也不得影响规则判断。
- Fact Query 可命中正式成功缓存；缓存只缓存成功 mapping 后的 facts，不缓存 provider 失败、超时、安全拦截或 mapping 失败。
- Provider 成功并 mapping 成功时，规则读取 `facts.<factCode>.<mappedPath>`。
- Provider 未配置、禁用、HTTP 失败、网络错误、超时、安全拦截、响应不可解析等查询失败时，依赖该 fact 的规则输出 `status=REJECTED` 或 `ERROR`，原因码归类为 `FACT_QUERY_FAILED`，并保留脱敏明细。
- Mapping required 字段缺失、类型不匹配、mapping key/source/config 非法时，依赖该 fact 的规则输出 `FACT_MAPPING_FAILED`，并通过 reason 区分 `MAPPING_REQUIRED_MISSING`、`MAPPING_TYPE_MISMATCH`、`MAPPING_KEY_INVALID`、`MAPPING_SOURCE_INVALID`、`MAPPING_CONFIG_INVALID`。
- Query 和 mapping 成功但规则引用的具体 `facts.<factCode>.*` 字段不存在时，规则输出 `status=REJECTED`、`passed=false`、`reasonCode=FACT_FIELD_MISSING`。
- Fact Query 失败只影响依赖该 fact 的活动或规则；同一事件中不依赖该 fact 的其他活动不得被连带失败。
- 完全离线模拟不是本轮默认能力；如未来新增开关，必须在响应中明确标记“不保证与依赖 facts 的正式链路一致”。

### 活动互斥与叠加

- 同一 `exclusiveGroup` 且 `stackable=false` 的活动，按正式活动选择规则只保留最终应命中的活动。
- 被互斥排除的活动应进入解释结果，活动级 `decision=REJECTED`，原因码建议为 `EXCLUSIVE_GROUP_SUPPRESSED`。
- 若活动 `stackable=true` 或不在同一互斥组，允许多个活动同时 `PASSED`，并分别展示奖励和 Quota 预览。

### 奖励计算

- 奖励计算必须使用 Decimal 语义，金额输出 4 位小数。
- `RATIO` 支持按 `baseField` 取数、`rate` 计算和 `maxAmount` 封顶。
- `FIXED`、`LADDER`、`RANDOM`、`PROBABILITY`、`POOL_SPLIT` 按现有奖励配置能力预览。
- 随机和概率类必须使用稳定 seed，使相同 tenant、activity、rewardCode、幂等键、配置版本和 seedSalt 的模拟结果可复现。
- 金额奖励计算结果为 `0.0000` 且无其他实际权益时，不展示可执行 grant order / outbox command 预览；保留 reward 计算解释，原因码建议为 `ZERO_REWARD`。
- 非金额类奖励若合法，应在 `rewardPayload` 中预览 points、quantity、chanceCount、durationDays 等权益字段，金额为 `0.0000` 不代表无奖励。
- 奖励配置 `targetRef` 缺失或非法时，不应伪造可执行 outbox；应在 reward / outbox 预览中明确不可执行原因。

### Quota / 预算 / 库存预览

- 模拟运行只读取当前 Quota 快照。
- Quota 预览应输出 `wouldReserve`、`reserveAmount`、`remainingBefore`、`remainingAfterPreview`、`wouldExceed`、`unlimited` 等语义。
- 支持的 Quota 类型：`BUDGET`、`STOCK`、`USER_LIMIT`、`DAILY_LIMIT`。
- 缺少对应 Quota 行时按无限制处理，并输出 `unlimited=true`。
- `BUDGET` 预览金额按拟发奖金额计算。
- `STOCK` 默认每个 enabled reward 扣 1，`scopeKey` 为 `rewardCode`。
- `USER_LIMIT` 默认每个 rewardable participation 预览扣 1。
- `DAILY_LIMIT` 默认每个 rewardable participation 预览扣 1，`scopeKey` 按 `subject.id:活动时区自然日`。
- 若任一必要 Quota 预览 `wouldExceed=true`，该活动 `decision=REJECTED`，原因码建议为 `QUOTA_EXCEEDED`，且不展示可执行 grant order / outbox command 预览。
- 模拟运行不得冻结、扣减、确认或释放真实 Quota。

### Participation / Grant / Outbox 预览

- `participationPreview` 仅展示如果真实执行将产生的参与快照，不写入 `marketing_participation`。
- 规则未通过时，可预览真实链路会形成的拒绝原因，但不得写 `REJECTED` participation。
- `grantOrderPreviews` 仅展示如果真实执行将产生的发奖单，不写入 `marketing_grant_order`。
- `outboxCommandPreviews` 仅展示如果真实执行将投递的标准发奖指令，不写入 `marketing_outbox`。
- 预览中的 command id、participation id、grant order id 如无法确定真实 ID，应使用 `sim_` 前缀或明确标记 `previewOnly=true`，不能伪装成已持久化 ID。
- Outbox 预览不得触发 HTTP、Webhook、MQ 或任何外部发奖系统调用。

### 插件预览

- 绑定 `pluginCode` 的活动，模拟运行可展示插件构建的上下文、前置检查结果和状态变更意图。
- 插件模拟不得写入真实玩法状态表，不得消费抽奖次数、红包库存或其他插件状态。
- 插件状态变更只能作为 `stateChangeIntents` 预览输出。

### 审计

- 模拟运行允许写 `marketing_audit` 的 `SIMULATION_RUN` 只读操作审计。
- 该审计不属于业务执行副作用，不得影响活动命中、奖励、Quota、Fact Query、插件状态或发奖指令语义。
- 审计 payload 最小口径只记录 tenant、actor、action、eventId、eventType、biz 标识、候选/命中/未命中计数、是否发生 Fact Query、是否有 Quota 风险、脱敏错误摘要。
- 审计不得记录 secret、token、完整 Authorization、Fact Provider 明文密钥、provider auth_ref 对应秘密、完整敏感 attributes、完整 provider 请求/响应、外部发奖 payload 中的敏感字段。

## 边界条件

- 无候选活动：返回 200，`candidateCount=0`，`matchedActivities=[]`，`unmatchedActivities=[]`。
- 全部未命中：返回 200，逐活动说明未命中原因，`hasRewardableActivity=false`。
- 多活动命中：按互斥和叠加规则输出最终命中列表。
- 活动配置不完整或运行时不可解释：该活动 `decision=ERROR`；只有请求本身非法时才返回 4xx。
- 规则字段缺失：对应规则 `status=REJECTED`，输出缺失字段路径。
- 金额字段不可解析 Decimal：对应规则或奖励 `status=ERROR` 或 reward 预览错误，不得使用 float 或静默转 0。
- Fact Provider 不存在或 disabled：依赖该 fact 的规则不通过，输出 `FACT_QUERY_FAILED` 类原因；不依赖该 fact 的活动继续解释。
- Fact Query 超时或外部 HTTP 失败：依赖该 fact 的规则不通过或标记错误，输出 `FACT_QUERY_FAILED` 和脱敏明细；不得写业务执行表。
- Fact mapping 失败：依赖该 fact 的规则输出 `FACT_MAPPING_FAILED`；不得把 provider 原始响应透传到前端或审计。
- Fact Query 成功但字段缺失：依赖字段的规则输出 `FACT_FIELD_MISSING`。
- Quota 不存在：按无限制处理，不算失败。
- Quota 已满：活动可展示规则通过和奖励计算，但最终发奖预览被抑制，活动 `decision=REJECTED` 并标记 Quota 风险。
- 幂等键与已有真实事件重复：模拟运行不应返回 duplicate 执行结果，不应写 inbox；可在预览中展示推导出的幂等键。
- `occurredAt == startTime` 视为在活动期内；`occurredAt >= endTime` 视为不在活动期内。
- `display.visible=false` 不影响模拟运行；`entry.enabled=false` 会拒绝候选。
- 审计写入失败不应导致业务执行表出现部分写入；是否让接口整体失败可按现有后台审计策略处理，但必须输出可排查错误，不得静默伪造成功审计。

## 异常场景

- 请求 JSON 非法：返回 `400 Bad Request`，说明 JSON 解析失败。
- 缺少标准事件必填字段：返回 `400 Bad Request`，列出缺失字段。
- `occurredAt` 非 RFC3339 或无时区：返回 `400 Bad Request`。
- `subject` 或 `biz` 为空：返回 `400 Bad Request`。
- `attributes` 不是 JSON object：返回 `400 Bad Request`。
- 请求体包含 inline Fact Provider 配置或未知顶层业务字段：返回 `400 Bad Request` 或按现有 API 校验规则拒绝。
- 请求体超过服务限制：按现有 HTTP 边界返回 4xx。
- 内部读取活动、Quota、Fact Provider registry 或配置失败：返回 5xx 或结构化错误；不得产生业务执行表部分写入。
- 单个活动解释失败但整体请求可继续时，默认最小口径是该活动 `decision=ERROR`，并继续解释其他活动。
- Fact Query 失败但可定位到具体活动依赖时，不应导致整个模拟请求 5xx；应在活动或规则解释中表达失败原因。
- Fact Query 基础设施整体不可用且无法判定依赖活动结果时，可将受影响活动标记 `ERROR`，但不得影响无 fact 依赖活动的解释。

## 非目标范围

- 不新增真实事件接入方式，不处理 MQ/Webhook Receiver 的新增能力。
- 不设计新的活动配置模型、规则类型、奖励类型、Quota 类型或 Fact Provider 配置模型。
- 不实现真实发奖、真实 Quota 扣减、真实 participation / grant order / outbox 写入。
- 不写 `marketing_event_inbox`，不消费真实幂等记录。
- 不直接连接外部业务数据库，不引用外部业务代码，不修改用户余额。
- 不在模拟运行中执行真实 HTTP、Webhook、MQ 发奖投递。
- 不允许请求体 inline provider、inline endpoint、inline auth、inline mapping 或绕过 registry 的临时 Fact Query 配置。
- 不实现完全离线模拟模式；如后续需要，必须作为显式降级能力另立需求。
- 不承诺模拟结果锁定预算、库存或奖励资格；模拟结果只代表当前配置、当前 Quota 快照、当前 Fact Query 结果或缓存状态。
- 不实现生产可公开暴露的无鉴权管理接口；当前管理端模拟运行仍按本地后台 / 管理 API 边界理解。
- 不要求支持批量模拟、回放历史事件、按 activityId 单活动模拟、A/B 配置对比或可视化流程图。

## 风险

- 无副作用风险：若模拟运行复用正式链路不当，可能误写 inbox、participation、grant_order、outbox、Quota 或插件状态。验收必须验证相关表无新增或变更。
- Fact Query 一致性风险：若模拟运行不执行正式只读 Fact Query，依赖 `facts.*` 的活动会与真实链路系统性不一致。本轮通过默认复用只读 Fact Query 关闭该风险。
- Fact Provider 外部依赖风险：模拟运行可能受 provider 超时、失败或缓存状态影响。响应必须输出 `FACT_QUERY_FAILED / FACT_MAPPING_FAILED / FACT_FIELD_MISSING` 等可解释原因。
- 安全风险：Fact Query 仍需遵守 allowlist、private/local IP 拒绝、禁止 redirect、禁用 proxy、禁止 inline provider 配置等安全边界。
- 枚举契约风险：若引入 `FAILED / MATCHED / UNMATCHED` 等新机器枚举，会破坏现有 JSON 语义、测试断言和文档一致性。
- 解释一致性风险：模拟结果必须与正式链路的匹配、规则、奖励、Fact Query 和 Quota 判断保持一致，不能出现运营模拟通过但真实事件失败的配置解释偏差。
- Quota 快照风险：模拟只读快照，不锁定额度；并发真实事件可能导致模拟后实际执行结果变化，响应必须明确 preview-only。
- 金额精度风险：奖励和预算预览涉及金额，必须使用 Decimal 语义并输出 4 位小数。
- 插件状态风险：插件预览不得消费玩法状态，只能输出状态变更意图。
- 审计敏感信息风险：模拟输入、Fact Provider、外部发奖预览可能包含敏感信息；审计和日志必须脱敏，不得记录 secret、token、完整 Authorization 或明文 provider auth。

## 验收标准

### 功能验收

1. 输入一条合法标准事件，接口返回 200，包含 `simulatedAt`、`summary`、`matchedActivities`、`unmatchedActivities`。
2. 至少一个活动命中时，命中活动结果包含活动身份、活动级 `decision`、规则解释、Fact Query 解释、奖励预览、Quota 影响预览、participation 预览、grant order 预览和 outbox command 预览。
3. 活动未命中时，未命中活动必须给出机器可读 `decisionReasonCode` 和人可读 `decisionReason`。
4. 活动级 `decision` 只能输出 `PASSED / REJECTED / SKIPPED / ERROR`，不得输出 `MATCHED / UNMATCHED / FAILED`。
5. 每条规则必须输出阶段、规则编码、是否通过、状态、原因、字段、期望值、实际值和排序信息。
6. 规则级 `status` 只能输出 `PASSED / REJECTED / SKIPPED / ERROR`；规则未通过必须使用 `REJECTED`，不得输出 `FAILED`。
7. 规则引用 `facts.*` 且正式链路需要该 fact 时，模拟运行必须执行同等只读 Fact Query，或命中同等成功缓存。
8. Fact Query provider 失败、超时、HTTP 失败或安全拦截时，依赖该 fact 的规则或活动输出 `FACT_QUERY_FAILED` 类原因，不依赖该 fact 的活动仍可继续解释。
9. Fact mapping 失败时，依赖该 fact 的规则或活动输出 `FACT_MAPPING_FAILED`，并通过 reason 稳定区分 mapping 失败类别。
10. Fact Query 成功但规则引用字段缺失时，规则输出 `FACT_FIELD_MISSING`。
11. 奖励预览必须展示奖励编码、奖励类型、计算类型、金额或权益 payload、币种、计算明细；金额按 4 位小数输出。
12. Quota 预览必须展示预算、库存、用户次数或每日次数的影响；缺失 Quota 行显示 `unlimited=true`。
13. Quota 超限时，活动返回 `decision=REJECTED` 和 Quota 风险原因，不展示可执行 grant order / outbox command 预览。
14. 互斥组内多个活动同时满足时，只按正式规则保留最终命中活动，其余活动输出互斥未命中原因。
15. `display.visible=false` 不影响模拟运行；`entry.enabled=false` 会导致 MATCH 阶段拒绝。
16. 金额为 0 且无其他实际权益的奖励不生成可执行发奖预览，并说明原因。

### 无副作用验收

1. 执行模拟运行前后，`marketing_event_inbox` 无新增模拟事件记录。
2. 执行模拟运行前后，`marketing_participation` 无新增记录。
3. 执行模拟运行前后，`marketing_grant_order` 无新增记录。
4. 执行模拟运行前后，`marketing_outbox` 无新增记录。
5. 执行模拟运行前后，`marketing_quota` 的 used / frozen / total 等聚合值无变化。
6. 模拟运行不会触发 Outbox worker、HTTP、Webhook、MQ 或外部发奖系统调用。
7. 模拟运行允许执行只读 Fact Query；Fact Query 只能读取 provider registry、成功缓存和外部 provider 响应，不得写业务执行表或修改 provider 配置。
8. 插件模拟不会写入真实玩法状态表，也不会消费抽奖次数、红包库存或其他插件状态。
9. 允许写 `marketing_audit` 的脱敏 `SIMULATION_RUN` 只读操作审计；该审计不计入业务执行副作用。
10. 审计和日志不得记录 secret、token、完整 Authorization、Fact Provider 明文密钥、完整敏感 attributes 或完整 provider 响应。

### 异常验收

1. 非法 JSON 返回 400。
2. 缺少必填标准事件字段返回 400。
3. 非法 `occurredAt` 返回 400。
4. 规则字段缺失时不返回 500，而是在规则解释中标记 `REJECTED`。
5. 单个活动配置运行时不可解释时，该活动可标记 `ERROR`，其他活动仍可继续输出解释。
6. 依赖 Fact Query 的单个活动查询失败时，该活动输出 fact 失败原因；同一请求中无 fact 依赖的活动不被连带拒绝。
7. 请求体包含 inline Fact Provider 配置时返回 400 或按现有校验规则拒绝。

### 文档验收

1. `docs/api.md` 中模拟运行字段语义与本需求一致。
2. 响应文档必须明确模拟结果不是 quota 锁定、奖励发放、插件状态消费或发奖指令投递。
3. 文档必须明确活动级 `decision` 和规则级 `status` 的机器枚举均为 `PASSED / REJECTED / SKIPPED / ERROR`。
4. 文档必须明确模拟运行会在活动规则实际引用 `facts.*` 时复用正式链路同等只读 Fact Query。
5. 文档必须明确模拟运行允许写脱敏 `SIMULATION_RUN` 审计，且不得记录敏感属性、secret、token 或完整 Authorization。

## 待确认问题

以下问题不阻塞本轮 PASS，因本轮已给出默认最小可验收口径：

- 是否需要支持按 `activityId` 指定单活动模拟？默认不支持，只按 tenant + eventType 候选活动模拟。
- 是否需要完全离线模拟模式？默认不支持；默认模拟运行执行正式链路同等只读 Fact Query，以保证与依赖 facts 的正式链路一致。离线模式如后续需要，应单独定义显式开关和“不保证一致”的响应标记。
- 是否需要管理端页面展示完整解释树？本需求只要求 API 输出可验收结构，页面展示优化不纳入最小范围。
- 是否需要支持批量模拟或历史事件回放？默认不支持。
- 审计写入失败时是否应阻断模拟响应？默认按现有后台审计策略处理；无论阻断与否，都不得写业务执行表或伪造敏感审计内容。

## 评分

```text
需求清晰度        19 / 20
范围可控          18 / 20
风险可控          18 / 20
验收标准明确      19 / 20
架构一致性        19 / 20
总分              93 / 100
```

扣分说明：

- 单活动模拟、批量模拟、管理端展示层交互尚未定义，本轮明确排除在最小范围之外。
- 模拟运行复用只读 Fact Query 后会受外部 provider 可用性影响，本轮通过可解释失败原因和无业务写副作用验收控制风险，但仍需测试覆盖 provider 超时、mapping 失败和缓存命中。
- 审计写入失败是否阻断响应仍依赖现有后台审计策略，本轮仅锁定允许脱敏审计和禁止敏感内容。

## Decision

```text
PASS
```

理由：Review P1/P2 和 Open Question 已逐条回应；上一轮 Analysis 的 Fact Query 错误假设和审计口径不清已修正；Score 93，需求细节清楚、可审核、可验收，可交给 Review Agent 复审。
