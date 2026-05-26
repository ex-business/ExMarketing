# Analysis

## 需求说明

本需求细化 `MATCH` 活动匹配阶段：对一个已通过标准事件校验的事件，在租户隔离边界内筛选可进入后续 `QUALIFY -> REWARD -> RISK_LIMIT_BUDGET -> quota/grant/outbox` 链路的活动，并为可解释范围内未通过 MATCH 的活动输出稳定、机器可识别的原因。

默认最小可验收口径：

- MATCH 的活动查询边界先按事件 `tenantId` 限定，再按事件 `eventType` 命中活动 `triggerEvents` 限定；跨租户活动和不触发当前事件类型的活动不进入候选、未命中明细或计数。
- 在上述可解释全集内，活动需满足 `status=ACTIVE`、`startTime <= occurredAt < endTime`、入口配置允许、参与人群允许、MATCH 阶段规则通过，才可进入互斥裁剪前候选。
- 多个候选按 `priority DESC` 和稳定 tie-breaker 排序；同一 `exclusiveGroup` 内若存在 `stackable=false` 候选，则该组只保留一个最高优先级候选，其余同组候选输出互斥拒绝原因。
- 输出字段沿用现有模拟/API 口径：每个活动使用 `decision / decisionReasonCode / decisionReason`，`decision` 不新增 `MATCHED/UNMATCHED`，而使用现有规则结果语义 `PASSED / REJECTED / SKIPPED / ERROR`。

## Review 反馈处理

- P1-1 跨租户 unmatched 与租户隔离冲突：已修正。MATCH 查询范围必须先按事件 `tenantId` 限定；跨租户活动不得出现在 `matchedActivities`、`unmatchedActivities`、`candidateCount`、`matchedCount`、`unmatchedCount` 或模拟明细中。`TENANT_NOT_MATCHED` 不作为本任务 MATCH 对外输出原因码；仅可作为内部防御日志、配置校验或单活动诊断时的不可达保护，不进入标准模拟响应。
- P1-2 `decision` 枚举与现有模型/API 不一致：已修正。需求不允许引入 `MATCHED / UNMATCHED` 新枚举，不做兼容性破坏。MATCH 输出沿用现有 `RuleDecision` 语义：通过为 `PASSED`，未通过为 `REJECTED`，被明确跳过的规则级检查为 `SKIPPED`，不可解释配置或执行错误为 `ERROR`。
- P2-1 未命中解释范围过大：已修正。`unmatchedActivities` 的可解释全集定义为同租户且 `triggerEvents` 包含当前 `eventType` 的活动集合。`EVENT_TYPE_NOT_MATCHED` 不在常规 MATCH 列表中出现；如果未来新增“解释某个指定活动为何不匹配某事件”的诊断接口，可在该接口中使用，但不属于本轮最小验收。
- P2-2 `stackable` 互斥规则歧义：已修正为组级算法。空 `exclusiveGroup` 不参与互斥；非空组内若全部候选 `stackable=true`，全部保留；只要组内存在至少一个 `stackable=false` 候选，该组进入单选模式，只保留排序第一的候选，组内其他候选全部 `REJECTED/EXCLUSIVE_GROUP_LOST`。
- Open Question `TENANT_NOT_MATCHED` 是否保留：本轮默认从 MATCH 对外输出原因码中移除，保留为内部防御性诊断，不影响模拟 API。
- Open Question 是否允许改变 `SimulationActivityResult.decision`：本轮不允许改变，必须沿用 `PASSED / REJECTED / SKIPPED / ERROR`。

## Analysis 自检修正

- 上一轮文档错误假设一：把跨租户活动纳入 `unmatchedActivities` 并要求输出 `TENANT_NOT_MATCHED`。本轮按租户隔离优先原则修正为查询前置边界，关闭该错误假设。
- 上一轮文档错误假设二：新增 `MATCHED / UNMATCHED` decision。已改回现有 `RuleDecision` 语义，避免扩大 API、模型和测试范围。
- 上一轮文档扣分项一：未命中解释全集不清晰，导致 `candidateCount/unmatchedCount` 可能被不同实现理解为“同租户所有活动”或“预筛候选”。本轮明确计数只基于同租户且触发当前事件类型的可解释全集。
- 上一轮文档扣分项二：互斥叠加算法不够确定。已补充组级单选/全叠加算法和 tie-breaker。
- 早期 Analysis 模板问题：曾出现 `Score=0`、`Decision` 占位、现状/目标/方案未填写。本轮已补齐需求说明、背景目标、输入输出、业务规则、边界条件、异常场景、非目标、风险和验收标准。

## 背景目标

ExMarketing 是独立活动营销中台。外部业务系统只上报标准事件，系统内部按 `Event Intake -> Campaign Match -> Rule Evaluation -> Quota / Budget / Inventory -> Grant Order -> Outbox` 处理。MATCH 是后续规则、奖励和额度流程的入口，应解决“哪些活动可被当前事件触发”和“为什么其他活动未进入后续处理”。

目标：

- 对标准事件筛选同租户、同事件类型下的活动候选。
- 对可解释范围内未命中的活动输出稳定拒绝原因，支撑 `/admin/v1/simulate`、运营排查和上线前验证。
- 对多活动候选按优先级和互斥组收敛，避免同组活动重复进入后续发奖链路。
- 保持与现有 API、数据模型、规则阶段和租户隔离边界一致。

## 参与方

- 外部业务系统：通过 `/openapi/v1/events` 或 webhook 上报标准事件。
- Event Intake：完成事件模型校验、鉴权、租户授权和幂等写入；不由 MATCH 重新定义。
- 运营/管理员：配置活动状态、时间、入口、展示、规则、奖励、优先级和互斥组。
- MATCH 阶段：读取标准事件和活动配置，输出匹配/未匹配解释。
- 后续阶段：消费 MATCH 通过的活动，继续执行 QUALIFY、REWARD、RISK/LIMIT、quota、grant_order、outbox。
- 模拟运行：展示 MATCH、规则、奖励和额度预览，但不得产生持久化副作用。

## 输入

标准事件输入：

- `eventId`：外部事件追踪 ID。
- `eventType`：事件类型；MATCH 查询边界之一，必须命中活动 `triggerEvents`。
- `sourceSystem`：来源系统；用于入口来源过滤和幂等键。
- `tenantId`：租户隔离字段；MATCH 查询边界之一。
- `subject.type` / `subject.id`：参与主体。
- `biz.bizType` / `biz.bizId`：业务对象，用于幂等键。
- `occurredAt`：事件发生时间，作为活动有效期判断基准。
- `attributes`：业务扩展字段，MATCH 可读取 `channel`、`segment`、`segments`、`riskLevel` 等入口和人群字段。

活动配置输入：

- `tenantId`、`activityCode`、`activityType`、`version`。
- `status`：活动状态，最小可验收口径仅 `ACTIVE` 可通过。
- `triggerEvents`：可触发活动的事件类型列表；常规 MATCH 查询必须已按当前 `eventType` 命中过滤。
- `startTime`、`endTime`、`timezone`：活动有效期与时区。
- `priority`：候选排序优先级，数值越大越优先。
- `exclusiveGroup`、`stackable`：互斥与叠加配置。
- `config.entry`：入口开关、渠道、来源系统、允许租户、人群过滤。
- `config.display`：展示配置；`display.visible=false` 只影响展示入口可见性，不使内部事件处理或模拟运行失效。
- `marketing_rule.rule_phase=MATCH` 且 `enabled=true` 的规则。

## 输出

MATCH 输出必须可被模拟运行和后续引擎消费：

- `matchedActivities`：可解释全集内通过基础检查、MATCH 规则和互斥裁剪后保留的活动。
- `unmatchedActivities`：可解释全集内被状态、时间、入口、人群、MATCH 规则、配置错误或互斥裁剪拒绝的活动。
- 每个活动包含 `activityId`、`activityCode`、`activityType`、`priority`、`exclusiveGroup`、`stackable`、`version`。
- 每个活动包含 `decision`：
  - `PASSED`：活动通过 MATCH 并保留在 `matchedActivities`。
  - `REJECTED`：活动未通过检查或被互斥裁剪，进入 `unmatchedActivities`。
  - `ERROR`：活动配置不可解释或 MATCH 规则执行错误，进入 `unmatchedActivities`。
  - `SKIPPED`：用于规则级解释中表示禁用规则跳过；活动级结果通常不使用 `SKIPPED` 表示命中或未命中。
- 每个活动包含 `decisionReasonCode` 和 `decisionReason`。
- 可选解释字段 `matchChecks` / `ruleResult` 应记录已执行检查摘要，但不要求为跨租户或不触发当前事件类型的活动生成明细。
- `summary.candidateCount`：可解释全集数量，即同租户且 `triggerEvents` 包含当前 `eventType` 的活动数量。
- `summary.matchedCount`：`matchedActivities` 数量。
- `summary.unmatchedCount`：`unmatchedActivities` 数量。

常规 MATCH 对外拒绝原因码：

- `ACTIVITY_NOT_ACTIVE`：活动状态不是 `ACTIVE`。
- `ACTIVITY_NOT_STARTED`：`occurredAt` 早于 `startTime`。
- `ACTIVITY_ENDED`：`occurredAt` 不早于 `endTime`。
- `ENTRY_DISABLED`：`config.entry.enabled=false`。
- `ENTRY_CHANNEL_NOT_ALLOWED`：`entry.channels` 非空且不包含 `attributes.channel`，或事件缺少必需 channel。
- `ENTRY_SOURCE_SYSTEM_NOT_ALLOWED`：`entry.sourceSystems` 非空且不包含 `sourceSystem`。
- `ENTRY_TENANT_NOT_ALLOWED`：`entry.tenantIds` 非空且不包含当前事件 `tenantId`；该配置不能扩大租户边界。
- `AUDIENCE_SUBJECT_TYPE_NOT_ALLOWED`：`entry.audience.subjectTypes` 非空且不包含 `subject.type`。
- `AUDIENCE_SEGMENT_NOT_ALLOWED`：`entry.audience.segments` 非空且事件 `attributes.segment/segments` 无交集。
- `AUDIENCE_RISK_LEVEL_NOT_ALLOWED`：`entry.audience.riskLevels` 非空且不包含 `attributes.riskLevel`。
- `MATCH_RULE_REJECTED`：启用的 MATCH 阶段规则执行未通过。
- `EXCLUSIVE_GROUP_LOST`：通过前置检查的候选在互斥组裁剪中落选。
- `INVALID_MATCH_CONFIG`：活动配置缺失、类型错误、时间非法、入口配置不可解释或规则执行错误，无法安全判断为通过。

不作为常规 MATCH 对外输出的原因码：

- `TENANT_NOT_MATCHED`：跨租户活动不允许被扫描或返回；该原因仅可用于内部防御日志或单活动诊断。
- `EVENT_TYPE_NOT_MATCHED`：不触发当前事件类型的活动不在可解释全集内；该原因仅可用于配置校验或单活动诊断。

## 业务规则

1. 租户隔离优先：MATCH 查询必须按事件 `tenantId` 限定活动集合。任何跨租户活动不得进入候选、未命中明细、计数或模拟响应。
2. 事件类型是查询边界：常规 MATCH 只解释 `triggerEvents` 包含当前 `eventType` 的活动。`triggerEvents` 为空、缺失或不包含当前事件类型的活动不进入常规输出。
3. 活动在线判断：可解释全集内仅 `ACTIVE` 活动可通过；`DRAFT`、`SCHEDULED`、`PAUSED`、`ENDED` 均输出 `REJECTED/ACTIVITY_NOT_ACTIVE`。
4. 时间有效性：使用事件 `occurredAt` 判断活动有效期，不使用系统处理时间。有效区间为 `startTime <= occurredAt < endTime`。
5. 时区语义：`timezone` 用于运营配置解释和展示；已存储为 RFC3339 绝对时间的 `startTime/endTime/occurredAt` 应按绝对时间比较。
6. 入口配置：
   - 无 `config.entry` 时按入口不限制处理，保持旧活动兼容。
   - `entry.enabled=false` 必须拒绝。
   - `entry.channels` 非空时读取 `attributes.channel`，缺失或不命中均拒绝。
   - `entry.sourceSystems` 非空时读取 `sourceSystem`，缺失或不命中均拒绝。
   - `entry.tenantIds` 只能收窄当前活动租户内的入口；非空且不包含事件 `tenantId` 时拒绝，不得用于查询或返回其他租户活动。
7. 参与人群：
   - `subjectTypes` 非空时必须包含 `subject.type`。
   - `segments` 支持事件 `attributes.segment` 单值或 `attributes.segments` 数组，至少一个交集才通过。
   - `riskLevels` 非空时必须包含 `attributes.riskLevel`。
8. 展示配置：`display.visible=false` 不拒绝内部事件处理和模拟运行。活动中心展示列表属于独立展示查询需求，不混入本轮 MATCH。
9. MATCH 阶段规则：只执行 `rule_phase=MATCH` 且 `enabled=true` 的规则；启用规则未通过时当前活动 `REJECTED/MATCH_RULE_REJECTED`。禁用规则可在规则级解释中标为 `SKIPPED`，不影响活动通过。
10. 检查顺序：状态、时间、入口、人群、MATCH 规则、互斥。多个拒绝条件同时存在时，活动级 `decisionReasonCode` 使用最早失败的主原因；解释明细可保留已执行检查。
11. 候选排序：通过前置检查和 MATCH 规则的候选先按 `priority DESC` 排序；同优先级使用稳定 tie-breaker，默认 `createdAt ASC`，若不可用则 `activityId ASC`。
12. 互斥组算法：
    - `exclusiveGroup` 为空：不参与互斥，候选保留。
    - 同一非空 `exclusiveGroup` 内全部候选 `stackable=true`：全部保留。
    - 同一非空 `exclusiveGroup` 内存在任意 `stackable=false` 候选：该组进入单选模式，仅保留组内排序第一的候选；组内其他候选全部进入 `unmatchedActivities`，输出 `REJECTED/EXCLUSIVE_GROUP_LOST`，并在解释中标明胜出活动。
    - 组内排序使用 `priority DESC, createdAt ASC, activityId ASC`，不得随机。
13. MATCH 不写入 `participation`、`grant_order`、`outbox`，不扣 quota；事件 inbox 写入和幂等属于 Event Intake，不属于 MATCH 需求本身。

## 边界条件

- `occurredAt` 等于 `startTime`：时间检查通过。
- `occurredAt` 等于 `endTime`：时间检查不通过，输出 `REJECTED/ACTIVITY_ENDED`。
- `startTime` 或 `endTime` 缺失、不可解析或 `startTime >= endTime`：输出 `ERROR/INVALID_MATCH_CONFIG`。
- 事件缺失 `tenantId` 或 `eventType`：应由标准事件校验拒绝；若进入 MATCH，结果应为空集合且不得跨租户兜底查询。
- 活动 `triggerEvents` 为空或缺失：不属于常规可解释全集，不进入 MATCH 输出；上线前校验应拒绝或提示。
- `entry.channels` 非空但事件无 `attributes.channel`：输出 `REJECTED/ENTRY_CHANNEL_NOT_ALLOWED`。
- `entry.audience.segments` 非空但事件无 segment 信息：输出 `REJECTED/AUDIENCE_SEGMENT_NOT_ALLOWED`。
- `entry.tenantIds` 非空且不包含事件 `tenantId`：输出 `REJECTED/ENTRY_TENANT_NOT_ALLOWED`。
- 活动无 `config.display` 或 `display.visible=false`：不得因此拒绝 MATCH。
- 同优先级同互斥组：按稳定 tie-breaker 选择胜出者。
- 无可解释活动：`matchedActivities=[]`、`unmatchedActivities=[]`、`candidateCount=0`、`matchedCount=0`、`unmatchedCount=0`。
- 重复事件：MATCH 判断结果应基于相同事件与配置保持业务一致；重复写入和幂等响应由 Event Intake 负责。

## 异常场景

- 活动配置 JSON 类型错误、`entry` 或 `audience` 字段不可解释：该活动输出 `ERROR/INVALID_MATCH_CONFIG`，不得默认通过。
- MATCH 规则字段缺失、类型不匹配、非法操作符或 evaluator 未注册：按规则结果输出 `REJECTED/MATCH_RULE_REJECTED` 或 `ERROR/INVALID_MATCH_CONFIG`，不得默认通过。
- 可解释全集中活动加载规则或奖励配置部分失败：MATCH 对该活动不得静默通过；若影响 MATCH 判断，输出 `ERROR/INVALID_MATCH_CONFIG`。
- 互斥组配置矛盾或同优先级重复：按稳定排序裁剪，保证结果可复现。
- 模拟运行中的 quota、reward 或后续阶段异常不得反向改变 MATCH 的租户、事件类型和互斥口径；后续阶段风险应在各自预览字段表达。

## 非目标范围

- 不实现 QUALIFY、REWARD、RISK、LIMIT、BUDGET 规则的业务语义。
- 不设计新的奖励计算、发奖单、Outbox、Callback 或 quota 扣减逻辑。
- 不新增面向生产的管理接口或鉴权策略。
- 不改变标准事件模型、幂等键格式或跨系统接入协议。
- 不新增 `MATCHED / UNMATCHED` decision 枚举。
- 不要求常规 MATCH 扫描同租户所有活动来解释 `EVENT_TYPE_NOT_MATCHED`。
- 不扫描、返回或计数跨租户活动。
- 不把 `display.visible=false` 解释为内部事件处理不可匹配。
- 不读取或依赖外部业务数据库。

## 风险

- 租户隔离风险：若为了解释 `TENANT_NOT_MATCHED` 扫描跨租户活动，会破坏数据边界。本轮通过查询前置租户边界规避。
- API 兼容风险：新增 `MATCHED/UNMATCHED` 会扩大模型改动。本轮沿用 `PASSED/REJECTED/SKIPPED/ERROR`。
- 未命中解释范围风险：若解释全集不固定，测试和实现会对计数产生分歧。本轮固定为同租户且触发当前事件类型。
- 展示入口与事件入口易混淆：本轮明确 `display.visible` 不影响内部 MATCH，`entry.*` 才影响事件入口匹配。
- 互斥叠加运营语义可能有争议：本轮采用组级安全默认；存在非叠加候选时整组单选。
- 延迟事件时间判断风险：本轮使用 `occurredAt`，避免处理时间导致补单事件错失活动。
- 历史脏配置风险：不可解释配置按 `ERROR/INVALID_MATCH_CONFIG`，不得默认通过。

## 验收标准

功能验收：

- 给定同租户、`triggerEvents` 包含事件类型、`ACTIVE`、时间有效、入口允许、MATCH 规则通过的活动，输出在 `matchedActivities`，`decision=PASSED`。
- 非 `ACTIVE` 的可解释活动进入 `unmatchedActivities`，`decision=REJECTED`，`decisionReasonCode=ACTIVITY_NOT_ACTIVE`。
- `occurredAt < startTime` 输出 `REJECTED/ACTIVITY_NOT_STARTED`；`occurredAt >= endTime` 输出 `REJECTED/ACTIVITY_ENDED`。
- `entry.enabled=false` 输出 `REJECTED/ENTRY_DISABLED`。
- `entry.channels`、`entry.sourceSystems`、`entry.audience.subjectTypes/segments/riskLevels` 任一不命中时输出对应原因码。
- `display.visible=false` 的活动在内部事件处理和模拟运行中不因该字段被拒绝。
- 启用的 MATCH 阶段规则失败时输出 `REJECTED/MATCH_RULE_REJECTED`，并保留规则解释。
- 配置不可解释时输出 `ERROR/INVALID_MATCH_CONFIG`，不得默认通过。
- 多活动候选按 `priority DESC, createdAt ASC, activityId ASC` 稳定排序。
- 空 `exclusiveGroup` 的候选不互斥。
- 同一非空 `exclusiveGroup` 全部 `stackable=true` 时全部保留。
- 同一非空 `exclusiveGroup` 存在任意 `stackable=false` 候选时仅保留组内排序第一活动，其余输出 `REJECTED/EXCLUSIVE_GROUP_LOST`。
- 跨租户活动不得出现在 `matchedActivities`、`unmatchedActivities` 或 summary 计数中，不输出 `TENANT_NOT_MATCHED` 作为常规 MATCH 响应。
- 不触发当前 `eventType` 的活动不得出现在 `matchedActivities`、`unmatchedActivities` 或 summary 计数中，不输出 `EVENT_TYPE_NOT_MATCHED` 作为常规 MATCH 响应。
- 无可解释活动时，响应为空集合且 `candidateCount/matchedCount/unmatchedCount` 均为 0。
- 模拟运行不得写入 `marketing_event_inbox`、`marketing_participation`、`marketing_grant_order`、`marketing_outbox` 或 `marketing_quota`。

文档与一致性验收：

- `docs/api.md` 的模拟运行输出继续使用 `decision / decisionReasonCode / decisionReason`，不新增 `MATCHED/UNMATCHED` 枚举。
- `docs/api.md` 与本需求对 `display.visible=false`、`entry.enabled=false`、`entry.tenantIds` 的语义保持一致。
- `docs/data-model.md` 中 `marketing_activity.priority/exclusiveGroup/stackable/config_json` 语义与本需求一致。
- 不改变外部事件幂等键：`tenantId:sourceSystem:eventType:bizType:bizId`。

测试验收：

- 覆盖状态、时间边界、入口、人群、MATCH 规则失败、配置错误、优先级排序、互斥裁剪和空结果。
- 覆盖跨租户活动不返回、不计数、不输出 `TENANT_NOT_MATCHED`。
- 覆盖不触发当前事件类型的活动不返回、不计数、不输出 `EVENT_TYPE_NOT_MATCHED`。
- 覆盖 `decision` 只使用现有 `PASSED/REJECTED/SKIPPED/ERROR` 语义。
- 覆盖同组全部可叠加、同组存在非叠加、同优先级稳定 tie-breaker 三类互斥场景。
- 覆盖 `display.visible=false` 不阻断内部事件 MATCH。
- 涉及持久化行为的完整链路测试应同时验证内存仓储与 PostgreSQL 仓储；仅 MATCH 纯候选判断可用单元测试覆盖。

## 待确认问题

无阻塞问题。以下为后续可优化项，不影响本轮默认最小可验收口径：

- 是否需要新增“单活动诊断”能力，用于解释指定活动为何不匹配某事件，并在该诊断能力中使用 `TENANT_NOT_MATCHED` / `EVENT_TYPE_NOT_MATCHED`。
- 是否需要为“活动中心展示列表”单独设计只看 `display.visible`、`entry.entryCodes`、渠道和人群的查询接口。
- 是否需要运营侧显式配置同优先级互斥组 tie-breaker；本轮默认使用稳定字段排序。
- 是否需要将 `EXCLUSIVE_GROUP_LOST` 细分为“被非叠加活动压制”和“同优先级稳定裁剪”。

## 评分

```text
需求清晰度        19/20
范围可控          19/20
风险可控          18/20
验收标准明确      19/20
架构一致性        19/20
Score             94/100
```

## Decision

```text
PASS
```

需求：2026-05-24-match
