# Analysis

## 需求 ID

2026-05-24-113929-7

## 标题

模拟运行解释器

## Review 反馈处理

本轮读取了 `tasks/2026-05-24-113929-7/review.md`。该文件仍为 Review Agent 模板，占位内容包含 `P0:`、`P1:`、`P2:`、`Open Questions 待填写`、`PASS / BLOCK`，但未给出具体打回原因、问题描述、文件行号或可执行修改意见。

处理结论：

- P0：无实质反馈需要处理。
- P1：无实质反馈需要处理。
- P2：无实质反馈需要处理。
- Open Questions：无实质待答问题；本轮不因模板占位保持 BLOCK。
- Decision：Review 未形成明确 BLOCK 理由；本轮按需求材料和上一轮 Analysis 自检问题重新细化。

## Analysis 自检修正

上一轮 `analysis.md` 是模板占位，存在以下问题：

- `现状 / 目标理解 / 方案 / 改动范围 / 风险点 / 验收方式` 均为“待填写”，无法进入 Review。
- `Score = 0`，低于 80。
- `Decision = PASS / BLOCK` 未决。
- 未回应任务背景中“模拟运行输入事件样例，输出命中的活动、未命中原因、每条规则通过/失败原因、预计奖励、预算/库存影响、最终发奖指令预览”的验收点。
- 未明确模拟运行不得写入 `participation`、`grant_order`、`outbox` 的副作用边界。

本轮修正：

- 补齐背景目标、参与方、输入输出、业务规则、边界条件、异常场景、非目标范围、风险和验收标准。
- 基于 `task.md`、`README.md`、`CLAUDE.md`、`docs/architecture.md`、`docs/api.md` 和上游设计文档 `22-独立活动营销中台设计.md` 形成默认最小可验收口径。
- 对未唯一明确的展示字段、错误码和排序规则给出最小默认约束，避免只等待 Owner 决策。
- 关闭上一轮占位导致的 BLOCK，重新评分为 90，Decision PASS。

## 背景目标

ExMarketing 是独立活动营销中台，核心链路是接收标准事件，完成活动匹配、规则判断、奖励计算、预算库存判断、参与记录和发奖指令编排。模拟运行能力面向运营、配置人员、测试人员和研发排障场景，用于在不产生真实业务副作用的前提下解释“一个事件样例会如何被当前活动配置处理”。

本需求要细化“模拟运行解释器”：输入一条标准事件样例，输出活动命中与未命中解释、规则级通过或失败原因、预计奖励、预算和库存影响、最终发奖指令预览，并明确该过程不得写入真实 `marketing_participation`、`marketing_grant_order`、`marketing_outbox` 等执行结果表。

默认最小可验收口径：

- 模拟运行使用现有管理端接口 `POST /admin/v1/simulate`。
- 模拟运行复用正式事件处理链路的活动匹配、规则执行、奖励计算、Quota 预览、插件上下文预览语义，但只生成预览结果。
- 模拟运行只读当前活动配置、规则、奖励、Quota 快照和必要的营销系统内部状态。
- 模拟运行不能作为真实发奖、Quota 锁定、参与记录创建或幂等消费。

## 参与方

- 运营 / 活动配置人员：提交事件样例，检查活动配置是否按预期命中和发奖。
- 测试人员：构造边界事件，验证规则、奖励和 Quota 风险解释。
- 研发 / 排障人员：定位活动未命中、奖励为 0、Quota 超限、插件前置校验失败等原因。
- ExMarketing 管理端：提供本地后台入口和模拟运行 API。
- ExMarketing 引擎：提供活动匹配、规则执行、奖励计算、Quota 预览、发奖命令预览的统一解释能力。
- 外部发奖系统：不参与模拟运行，模拟结果只展示将要发出的指令形态。

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
- 业务字段必须放在 `attributes`，不能新增订单、存款、会员等业务系统顶层模型。
- 幂等键语义仍按 `tenantId + ":" + sourceSystem + ":" + eventType + ":" + bizType + ":" + bizId` 推导，用于预览 participation / grant_order / outbox 的幂等字段，但模拟运行不得写入真实 inbox 或消费真实幂等记录。
- `attributes` 允许包含金额、币种、渠道、标签、风险等级、首存标记、设备、IP 等规则需要的字段。
- 默认不要求调用方额外传入活动 ID；模拟范围是同 tenant、同 trigger event 的当前候选活动。若未来新增按活动 ID 定向模拟，不属于本需求最小范围。

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

模拟运行响应必须以结构化 JSON 返回，至少包含：

- `simulatedAt`：模拟运行服务端时间。
- `summary.candidateCount`：候选活动数。
- `summary.matchedCount`：最终可奖励或命中的活动数。
- `summary.unmatchedCount`：未命中活动数。
- `summary.hasRewardableActivity`：是否存在可生成发奖预览的活动。
- `summary.hasQuotaRisk`：是否存在预算、库存或次数限制风险。
- `matchedActivities`：命中活动列表。
- `unmatchedActivities`：未命中活动列表。

每个活动结果至少包含：

- 活动身份：`activityId`、`tenantId`、`activityCode`、`activityName`、`activityType`、`version`。
- 匹配决策：`decision`、`decisionReasonCode`、`decisionReason`。
- 规则解释：按阶段和 sort 输出每条规则的结果。
- 奖励预览：每个 enabled reward 的预计结果、金额或权益 payload、计算明细。
- Quota 影响预览：预算、库存、用户次数、每日次数等会如何占用或是否超限。
- `participationPreview`：若进入参与预览，展示参与记录将保存的关键快照字段。
- `grantOrderPreviews`：若奖励金额或权益有效且 Quota 不超限，展示拟生成的发奖单预览。
- `outboxCommandPreviews`：若存在可执行发奖单，展示拟发送的标准发奖指令。
- 插件预览：如活动绑定插件，展示 `pluginCode`、插件上下文、前置检查结果和状态变更意图。

规则级解释字段最小口径：

- `ruleCode`
- `rulePhase`
- `ruleType`
- `sort`
- `status`：`PASSED` / `FAILED` / `SKIPPED` / `ERROR`
- `passed`
- `reasonCode`
- `reason`
- `field`
- `expected`
- `actual`

默认输出排序：

- 候选活动先按正式命中逻辑使用的优先级排序；同优先级按活动 ID 或创建顺序稳定排序。
- 规则按业务阶段 `MATCH -> QUALIFY -> REWARD -> RISK_LIMIT_BUDGET`，阶段内按 `sort` 升序。
- 奖励预览按活动 reward 配置顺序。
- Quota 预览按 `BUDGET -> STOCK -> USER_LIMIT -> DAILY_LIMIT`。

## 业务规则

### 活动候选

- 只评估与输入事件 `tenantId` 一致的活动。
- 只评估 `triggerEvents` 包含输入 `eventType` 的活动。
- 只评估当前配置可参与运行判断的活动状态；`DRAFT`、`PAUSED`、`ENDED` 等非可运行状态应进入未命中并给出原因，而不是静默丢弃。
- 活动时间判断必须使用活动配置的 `timezone` 和事件 `occurredAt`。
- `display.visible=false` 不影响模拟运行。
- `entry.enabled=false` 应在 MATCH 阶段拒绝候选并输出 `ENTRY_DISABLED`。
- `entry.channels`、`entry.sourceSystems`、`entry.tenantIds`、`entry.audience.*` 应按现有 API 文档的规则输出对应不允许原因。

### 规则执行

- 规则阶段固定为 `MATCH -> QUALIFY -> REWARD -> RISK_LIMIT_BUDGET`。
- 实际存储规则阶段仍使用 `MATCH`、`QUALIFY`、`REWARD`、`RISK`、`LIMIT`；`BUDGET` 是 Quota 类型，不是合法 `rulePhase`。
- `enabled=false` 的规则必须在解释结果中显示为 `SKIPPED`，且不影响活动决策。
- `CONDITION` 规则按 `logic=AND/OR` 和条件列表执行；字段缺失、类型不匹配、非法操作符、Decimal 解析失败应显示为失败或错误原因，不能默认通过。
- 字段路径仅允许标准事件字段、`attributes.*`、`facts.<factCode>.*`、`state.activity.*`、`state.subject.*`、`state.quota.*`、`state.exclusive.*`、绑定插件时的 `state.plugin.<pluginCode>.*`。
- 模拟运行不得把非标准顶层业务字段当作有效规则输入。

### 活动互斥与叠加

- 同一 `exclusiveGroup` 且 `stackable=false` 的活动，按正式活动选择规则只保留应命中的活动。
- 被互斥排除的活动应进入未命中或非最终命中解释，原因码应能表达互斥关系，例如 `EXCLUSIVE_GROUP_SUPPRESSED`。
- 若活动 `stackable=true` 或不在同一互斥组，允许多个活动同时命中并分别展示奖励和 Quota 预览。

### 奖励计算

- 奖励计算必须使用 Decimal 语义，金额输出 4 位小数。
- `RATIO` 支持按 `baseField` 取数、`rate` 计算和 `maxAmount` 封顶。
- `FIXED`、`LADDER`、`RANDOM`、`PROBABILITY`、`POOL_SPLIT` 等按现有奖励配置能力预览；随机和概率类必须使用稳定 seed，使相同 tenant、activity、rewardCode、幂等键、配置版本和 seedSalt 的模拟结果可复现。
- 金额结果为 `0.0000` 时，不展示可执行 grant order / outbox command 预览；可以保留 reward 计算解释，说明 `ZERO_REWARD`。
- 非金额类奖励若合法，应在 `rewardPayload` 中预览 points、quantity、chanceCount、durationDays 等权益字段，金额为 `0.0000`。

### Quota / 预算 / 库存预览

- 模拟运行只读取当前 Quota 快照，输出 `wouldReserve`、`reserveAmount`、`remainingBefore`、`remainingAfterPreview`、`wouldExceed`、`unlimited` 等预览语义。
- 支持的 Quota 类型：`BUDGET`、`STOCK`、`USER_LIMIT`、`DAILY_LIMIT`。
- 缺少对应 Quota 行时按无限制处理，并输出 `unlimited=true`。
- `STOCK` 默认每个 enabled reward 扣 1，`scopeKey` 为 `rewardCode`。
- `DAILY_LIMIT` 的 scopeKey 按 `subject.id:活动时区自然日` 预览。
- 若任一必要 Quota 预览 `wouldExceed=true`，该活动应给出 `QUOTA_EXCEEDED` 风险原因，且不展示可执行 grant order / outbox command 预览。
- 模拟运行不得冻结、扣减、确认或释放真实 Quota。

### Participation / Grant / Outbox 预览

- `participationPreview` 仅展示如果真实执行将产生的参与快照，不写入 `marketing_participation`。
- `grantOrderPreviews` 仅展示如果真实执行将产生的发奖单，不写入 `marketing_grant_order`。
- `outboxCommandPreviews` 仅展示如果真实执行将投递的标准发奖指令，不写入 `marketing_outbox`。
- 预览中的 command id / participation id / grant order id 如无法确定真实 ID，应使用带 `sim_` 前缀或明确标记 `previewOnly=true`，不能伪装成已持久化 ID。
- Outbox 预览不得触发 HTTP、Webhook、MQ 或任何外部系统调用。

### 插件预览

- 绑定 `pluginCode` 的活动，模拟运行可展示插件构建的上下文、前置检查结果和状态变更意图。
- 插件模拟不得写入真实玩法状态表，不得消费抽奖次数、红包库存或其他插件状态。
- 插件状态变更只能作为 `stateChangeIntents` 预览输出。

## 边界条件

- 无候选活动：返回 200，`candidateCount=0`，命中和未命中列表为空。
- 全部未命中：返回 200，逐活动说明未命中原因，`hasRewardableActivity=false`。
- 多活动命中：按互斥和叠加规则输出最终命中列表。
- 活动配置不完整或非法：如果活动已保存但运行时发现不可执行配置，应返回该活动的 `ERROR` 决策解释；只有请求本身非法时才返回 4xx。
- 规则字段缺失：对应规则失败，并输出缺失字段路径。
- 金额字段不可解析 Decimal：对应规则或奖励失败，不得使用 float 或静默转 0。
- 奖励配置 targetRef 缺失：若该 reward 无法生成可执行发奖指令，应在 reward / outbox 预览中明确原因。
- Quota 不存在：按无限制，而不是失败。
- Quota 已满：活动可展示规则通过和奖励计算，但最终发奖预览应被抑制并标记 Quota 风险。
- 幂等键与已有真实事件重复：模拟运行不应返回 duplicate 执行结果，不应写 inbox；可在预览中展示推导出的幂等键。
- Fact 依赖：若规则依赖 `facts.*`，模拟运行最小口径不主动发起外部事实查询；若当前链路已有配置化 fact 获取能力，必须保持只读和可解释。无法获得 fact 时按字段缺失解释。
- 时间边界：`occurredAt == startTime` 视为在活动期内，`occurredAt >= endTime` 视为不在活动期内，依据 `startTime < endTime` 的既有创建约束。

## 异常场景

- 请求 JSON 非法：返回 `400 Bad Request`，错误说明 JSON 解析失败。
- 缺少标准事件必填字段：返回 `400 Bad Request`，列出缺失字段。
- `occurredAt` 非 RFC3339 或无时区：返回 `400 Bad Request`。
- `subject` 或 `biz` 为空：返回 `400 Bad Request`。
- `attributes` 不是 JSON object：返回 `400 Bad Request`。
- 请求体超过服务限制：按现有 HTTP 边界返回 4xx。
- 内部读取活动、Quota 或配置失败：返回 5xx 或结构化错误；不得产生部分写入，因为模拟运行整体无写副作用。
- 单个活动解释失败但整体请求可继续时，默认最小口径是该活动标记 `decision=ERROR` 并继续解释其他活动，避免一个坏配置阻断全部模拟。

## 非目标范围

- 不新增真实事件接入方式，不处理 MQ/Webhook Receiver 的新增能力。
- 不设计新的活动配置模型、规则类型或奖励类型。
- 不实现真实发奖、真实 Quota 扣减、真实 participation / grant_order / outbox 写入。
- 不直接连接外部业务数据库，不引用外部业务代码，不修改用户余额。
- 不在模拟运行中执行真实 HTTP、Webhook、MQ 发奖投递。
- 不承诺模拟结果锁定预算、库存或奖励资格；模拟结果只代表当前配置和当前 Quota 快照。
- 不实现生产可公开暴露的无鉴权管理接口；当前管理端模拟运行仍按本地后台 / 管理 API 边界理解。
- 不要求支持批量模拟、回放历史事件、A/B 配置对比或可视化流程图；这些可作为后续需求。

## 风险

- 无副作用风险：若模拟运行复用正式链路不当，可能误写 inbox、participation、grant_order、outbox 或 Quota。验收必须验证相关表无新增或变更。
- 解释一致性风险：模拟结果必须与正式链路的匹配、规则、奖励和 Quota 判断保持一致，不能出现运营模拟通过但真实事件失败的配置解释偏差。
- Quota 快照风险：模拟只读快照，不锁定额度；并发真实事件可能导致模拟后实际执行结果变化，响应必须明确 preview-only 语义。
- 金额精度风险：奖励和预算预览涉及金额，必须使用 Decimal 语义并输出 4 位小数。
- 插件状态风险：插件预览不得消费玩法状态，只能输出状态变更意图。
- Fact 依赖风险：模拟时外部事实不可用可能影响规则解释；最小口径按缺失字段解释，不把外部系统作为强依赖。
- 安全边界风险：管理接口不应被误认为可公网开放；模拟输入可能包含敏感属性，日志中不应记录 secret、token、完整 Authorization。

## 验收标准

### 功能验收

1. 输入一条合法标准事件，接口返回 200，包含 `simulatedAt`、`summary`、`matchedActivities`、`unmatchedActivities`。
2. 至少一个活动命中时，命中活动结果包含活动身份、最终决策、规则解释、奖励预览、Quota 影响预览、participation 预览、grant order 预览和 outbox command 预览。
3. 活动未命中时，未命中活动必须给出机器可读 `decisionReasonCode` 和人可读 `decisionReason`。
4. 每条规则必须输出阶段、规则编码、是否通过、状态、原因、字段、期望值、实际值和排序信息。
5. 奖励预览必须展示奖励编码、奖励类型、计算类型、金额或权益 payload、币种、计算明细；金额按 4 位小数输出。
6. Quota 预览必须展示预算、库存、用户次数或每日次数的影响；缺失 Quota 行显示 `unlimited=true`。
7. Quota 超限时，活动返回 Quota 风险原因，不展示可执行 grant order / outbox command 预览。
8. 互斥组内多个活动同时满足时，只按正式规则保留最终命中活动，其余活动输出互斥未命中原因。
9. `display.visible=false` 不影响模拟运行；`entry.enabled=false` 会导致 MATCH 阶段拒绝。
10. 金额为 0 的奖励不生成可执行发奖预览，并说明原因。

### 无副作用验收

1. 执行模拟运行前后，`marketing_event_inbox` 无新增模拟事件记录。
2. 执行模拟运行前后，`marketing_participation` 无新增记录。
3. 执行模拟运行前后，`marketing_grant_order` 无新增记录。
4. 执行模拟运行前后，`marketing_outbox` 无新增记录。
5. 执行模拟运行前后，`marketing_quota` 的 used / frozen / total 等聚合值无变化。
6. 模拟运行不会触发 Outbox worker、HTTP、Webhook、MQ 或外部发奖系统调用。

### 异常验收

1. 非法 JSON 返回 400。
2. 缺少必填标准事件字段返回 400。
3. 非法 `occurredAt` 返回 400。
4. 规则字段缺失时不返回 500，而是在规则解释中标记失败。
5. 单个活动配置运行时不可解释时，该活动可标记 ERROR，其他活动仍可继续输出解释。

### 文档验收

1. `docs/api.md` 中模拟运行字段语义与本需求一致。
2. 响应文档必须明确模拟结果不是 quota 锁定、奖励发放、插件状态消费或发奖指令投递。

## 待确认问题

以下问题不阻塞本轮 PASS，因本轮已给出默认最小可验收口径：

- 是否需要支持按 `activityId` 指定单活动模拟？默认不支持，只按 tenant + eventType 候选活动模拟。
- 是否需要模拟运行主动调用 Fact Query Provider？默认不主动调用外部事实源，避免模拟依赖外部系统；已有链路若支持也必须只读且可解释。
- 是否需要把模拟运行写审计日志？默认不写任何业务执行表；如未来需要审计，应单独定义只读操作审计，且不得影响本需求无副作用验收。
- 是否需要管理端页面展示完整解释树？本需求只要求 API 输出可验收结构，页面展示优化不纳入最小范围。

## 评分

```text
需求清晰度        18 / 20
范围可控          19 / 20
风险可控          18 / 20
验收标准明确      18 / 20
架构一致性        17 / 20
总分              90 / 100
```

扣分说明：

- Fact Query 在模拟运行中是否主动调用仍可能存在产品取舍，本轮采用“不主动外部依赖”的最小口径。
- 管理端页面展示层尚未定义详细交互，只要求 API 结构化输出。

## Decision

```text
PASS
```
