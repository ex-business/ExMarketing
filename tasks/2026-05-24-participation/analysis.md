# Analysis

## 需求 ID

2026-05-24-participation

## Review 反馈处理

本轮读取了 `tasks/2026-05-24-participation/review.md`。

- `P0/P1/P2`：review 文件仍是模板，未列出具体问题；本轮按无明确 review finding 处理。
- `Open Questions`：review 文件仅写“待填写”，未形成可执行问题；本轮不等待 Owner，基于 `task.md`、`README.md`、`CLAUDE.md`、`docs/architecture.md`、`docs/data-model.md`、`docs/api.md` 和 `22-独立活动营销中台设计.md` 给出默认最小可验收口径。
- `Decision`：review 文件仍是 `PASS / BLOCK` 模板，未给出打回理由；本轮视为 analysis 初次补全/返工，不以 review 模板阻塞。

## Analysis 自检修正

上一轮 `analysis.md` 存在以下问题，本轮逐条修正：

- `Score = 0`：上一轮未完成分析，低于进入研发门槛。本轮按评分规则重新评估并补充完整需求，目标达到 `Score >= 80`。
- `Decision = PASS / BLOCK` 模板未决：本轮输出明确 Decision。
- `现状 / 目标理解 / 方案 / 改动范围 / 风险点 / 验收方式` 均为“待填写”：本轮补齐为可审核、可验收的需求说明。
- 旧文件要求读取“相关代码”，但本轮用户明确要求“不需要读取代码，不允许用代码实现细节替代需求澄清”；本轮遵循最新用户要求，只读取任务和文档，不读取代码。
- 旧文件没有处理 participation 的状态、快照、幂等、拒绝路径、零奖励、quota 不足和重复事件边界；本轮将这些作为业务规则和验收标准补齐。

## 背景目标

`ExMarketing` 是独立活动营销中台，只接收标准事件、判断活动配置、生成参与记录、发奖单和发奖指令，不直接修改外部业务系统数据或用户余额。

本需求来自“22-独立活动营销中台设计”：所有活动命中后必须先写 `marketing_participation`，保存当时活动配置、规则结果、事件 payload、奖励计算摘要和拒绝原因。核心目标是：

- 不能绕过 participation 直接发奖。
- `REJECTED`、`QUALIFIED`、`REWARDABLE` 等状态可追溯。
- 同一标准事件命中同一活动时 participation 幂等且唯一。
- participation 快照能支撑后续审计、对账、问题排查和重复事件判断。

## 参与方

- 外部业务系统：通过 `POST /openapi/v1/events` 或 Webhook Receiver 上报标准事件。
- ExMarketing Event Intake：校验事件、鉴权、幂等键并写入 `marketing_event_inbox`。
- ExMarketing Engine：匹配活动、执行规则、计算奖励、判断 quota。
- ExMarketing Ledger/Repository：持久化 `marketing_participation`、`marketing_grant_order`、`marketing_outbox` 和 quota 影响。
- 外部发奖系统：只接收由 grant order / outbox 产生的标准奖励指令。
- 运维/客服/审计人员：通过数据、日志、对账查询追踪某事件、用户、活动或发奖单的完整链路。

## 输入

### 标准事件输入

事件来源为标准事件处理链路。事件只允许以下顶层字段：

```text
eventId
eventType
sourceSystem
tenantId
subject
biz
occurredAt
attributes
```

`Idempotency-Key` 必须等于：

```text
tenantId:sourceSystem:eventType:bizType:bizId
```

### 活动配置输入

参与记录快照需要引用活动在命中时的配置，包括：

- `marketing_activity`：活动 ID、编码、类型、状态、时间范围、时区、优先级、互斥组、是否叠加、版本、扩展配置、插件编码。
- `marketing_rule`：规则编码、阶段、类型、配置、排序、启用状态、版本。
- `marketing_reward`：奖励编码、类型、计算方式、发奖模式、发奖通道、配置、启用状态、版本。
- 可选插件上下文：仅当活动配置 `pluginCode` 且插件注册可用时存在。

## 输出

### participation 记录

每个进入活动判断链路且与活动形成明确决策的事件，应按活动维度输出 participation 记录。字段语义以 `docs/data-model.md` 为准：

- `tenant_id`
- `activity_id`
- `event_inbox_id`
- `subject_type`
- `subject_id`
- `biz_type`
- `biz_id`
- `status`
- `reject_reason`
- `snapshot_json`
- `idempotency_key`

participation 幂等键固定为：

```text
tenantId + ":" + activityId + ":" + eventInbox.idempotency_key
```

### snapshot_json 最小内容

`snapshot_json` 必须能还原本次活动判断的关键上下文，最小包含：

- `activity`：活动 ID、`activityCode`、`activityType`、`pluginCode`、`version`、互斥/叠加字段、活动扩展配置、规则配置摘要、奖励配置摘要。
- `event`：`inboxId`、`eventId`、`eventType`、`sourceSystem`、`tenantId`、`subject`、`biz`、`occurredAt`、`attributes`、事件幂等键。
- `ruleResult`：`MATCH`、`QUALIFY`、`REWARD`、`RISK_LIMIT_BUDGET` 四个业务阶段的执行摘要；规则级结果需包含通过/拒绝/跳过、原因码、原因、字段、期望值、实际值和排序信息。
- `rewards`：每个奖励的 `rewardCode`、`rewardType`、`calculateType`、`grantChannel`、金额/币种或非金额权益摘要、是否跳过、原因码和原因。
- `plugin`：存在插件时记录 `pluginCode`、插件上下文、预检结果、解释信息和状态变更意图；无插件时允许为空对象或空值。

## 业务规则

1. `marketing_event_inbox` 仍是事件幂等入口；重复事件不得重复创建 participation、grant order 或 outbox。
2. 事件通过基础校验并写入 inbox 后，按当前活动匹配和规则执行顺序处理：`MATCH -> QUALIFY -> REWARD -> RISK_LIMIT_BUDGET`。
3. 对活动形成明确拒绝结论时，必须写 `REJECTED` participation，记录 `reject_reason` 和完整 snapshot；不得生成 grant order/outbox，不得扣减 quota。
4. 规则通过但全部奖励无实际可发权益时，必须写 `QUALIFIED` participation；默认最小口径为 `reject_reason = "ZERO_REWARD_SKIPPED: no payable reward"`，不得写 `REWARDABLE`，不得生成 grant order/outbox，不得扣减 quota。
5. 规则通过且至少存在一个实际可发奖励时，必须先形成 `REWARDABLE` participation，再生成 grant order/outbox；不得存在没有 participation 的 grant order 或 outbox。
6. `REWARDABLE` participation、quota 扣减、grant order 创建和 outbox 创建必须在同一个活动级事务内保持一致；任何一步失败不得留下只发奖、只扣 quota 或只写 outbox 的不完整结果。
7. quota 不足时，本轮默认最小口径为：回滚该活动本次 `REWARDABLE` participation、grant order、outbox 和 quota 扣减，不保留 `REWARDABLE` 脏记录；通过审计记录 quota 拒绝原因。该口径来自 `docs/data-model.md` 和 `docs/api.md`，用于关闭“是否需要保留 quota 拒绝 participation”的歧义。
8. disabled 规则在规则结果中记录为 `SKIPPED`，不影响最终是否通过；非法规则配置应在活动创建或上线前被拒绝，不由 participation 需求兜底。
9. 非金额奖励只要存在正向实际权益数量，即使 `reward_amount=0.0000`，仍可进入 `REWARDABLE`、grant order 和 outbox；金额为 `0.0000` 且无其他权益时按零奖励处理。
10. 同一标准事件命中同一活动只能生成一条 participation；重放完全相同事件时返回 duplicate 语义，不新增 participation。
11. 相同 `Idempotency-Key` 但 payload 不一致时属于事件幂等错误，应拒绝事件处理，不新增 participation。
12. 模拟运行 `POST /admin/v1/simulate` 不写 `marketing_event_inbox`、`marketing_participation`、`marketing_grant_order`、`marketing_outbox` 或 quota，只能返回 preview。
13. ActivityPlugin 不得绕过 participation、grant order、outbox、callback 或 idempotency；插件只能提供规则/奖励可解释上下文。

## 边界条件

- 未匹配任何活动：不生成 participation；事件 inbox 可按现有事件处理语义完成记录。
- 活动状态、时间窗、入口配置、租户、渠道、人群等 MATCH 阶段不满足：如该活动已作为候选并形成拒绝结论，应写 `REJECTED` participation；若系统定义为完全不进入候选，则不写 participation。默认最小验收口径：只要求“已进入候选并被规则/入口拒绝”的活动有 `REJECTED` 可追溯，不强制为所有非候选活动落库。
- 多个活动同时命中：按活动维度分别生成 participation；互斥/叠加规则导致拒绝的活动应能通过 `REJECTED` snapshot 解释原因。
- 一个活动多个奖励：一个 participation 可关联多个 grant order；每个 grant order 幂等键为 `participation.idempotency_key + ":" + rewardCode`。
- 部分奖励金额为 0、部分奖励可发：participation 仍可为 `REWARDABLE`，snapshot 中需分别说明奖励跳过或可发原因；只为可发奖励生成 grant order/outbox。
- 规则字段缺失、类型不匹配、操作符比较失败：规则结果应给出明确拒绝/失败原因并进入 `REJECTED`，不得默认通过。
- PostgreSQL 与内存仓储都需要遵守相同业务语义；涉及事务一致性的验收以 PostgreSQL 为准。
- snapshot 保存的是命中时刻配置和事件 payload 的审计快照；后续活动配置更新不得改变历史 participation 的 snapshot。

## 异常场景

- 非法 JSON、未知事件字段、缺少必填字段、非法 `occurredAt`、非法 Content-Type：事件在基础校验阶段失败，不生成 inbox 或 participation。
- 缺失或不匹配 `Idempotency-Key`：返回 `INVALID_IDEMPOTENCY_KEY`，不生成 participation。
- 完全重复事件：返回 duplicate，不重复创建 participation、grant order 或 outbox。
- 同 key 不同 payload：返回 `INVALID_IDEMPOTENCY_KEY`，不得覆盖或新增 participation。
- quota 不足：返回 `QUOTA_EXCEEDED`，不得保留 `REWARDABLE` participation，不得生成 grant order/outbox。
- grant order 或 outbox 写入失败：本活动事务失败，不能留下可发奖但无 outbox、或 outbox 无 grant order、或 grant order 无 participation 的不一致状态。
- snapshot 序列化失败：应使本活动处理失败，不允许跳过 snapshot 后继续发奖。
- 外部发奖系统不可用：不影响 participation 已经作为 `REWARDABLE` 与 grant order/outbox 一起落库；后续由 outbox retry/DLQ 处理。

## 非目标范围

- 不新增活动类型、规则类型、奖励类型或插件能力。
- 不设计或实现新的 API、后台页面、报表页面。
- 不改变标准事件模型、事件鉴权模型或 idempotency key 格式。
- 不实现 MQ 事件接入或 MQ outbox sender。
- 不直接调用钱包、积分、优惠券等外部系统发奖。
- 不引入外部业务数据库查询或外部业务代码依赖。
- 不处理历史 participation 数据迁移或旧数据补快照。
- 不改变模拟运行无副作用的边界。
- 不以代码实现假设替代业务口径；本需求只定义 participation 必须记录什么、何时记录、何时不得记录。

## 风险

- 账务/发奖安全风险高：若 grant order/outbox 可绕过 participation，会造成不可审计发奖。
- 事务一致性风险高：`REWARDABLE`、quota、grant order、outbox 任一环节非原子会造成重复发奖、漏发、预算不一致或脏记录。
- 幂等风险高：同一事件重复上报必须不重复 participation 和发奖；同 key 不同 payload 必须拒绝。
- 审计风险中高：snapshot 内容不足会导致活动配置变更后无法解释历史拒绝或发奖。
- 数据量风险中：snapshot 包含配置、事件、规则和奖励摘要，需控制为必要审计字段，避免写入无界大对象或敏感 token。
- 兼容风险中：内存仓储和 PostgreSQL 仓储必须保持语义一致，但事务验收以 PostgreSQL 为准。

## 验收标准

1. 上报一个满足活动规则且有实际奖励的标准事件后，必须产生 `REWARDABLE` participation；其创建先于或原子伴随 grant order/outbox，且 grant order 必须关联该 participation。
2. 任何 grant order/outbox 都不能在缺少 participation 的情况下出现。
3. 上报一个进入候选但规则不通过的事件后，必须产生 `REJECTED` participation；`reject_reason` 和 `snapshot_json.ruleResult` 能解释拒绝原因；不得生成 grant order/outbox，不得扣 quota。
4. 规则通过但奖励金额为 `0.0000` 且无非金额权益时，必须产生 `QUALIFIED` participation，`reject_reason` 记录 `ZERO_REWARD_SKIPPED: no payable reward`；不得生成 grant order/outbox，不得扣 quota。
5. 非金额权益奖励可发时，即使 `reward_amount=0.0000`，仍生成 `REWARDABLE` participation、grant order/outbox，并在 snapshot/reward payload 中说明权益数量和引用。
6. `snapshot_json` 至少包含 activity、event、ruleResult、rewards，以及存在插件时的 plugin 信息；历史 snapshot 不受后续活动配置修改影响。
7. participation 幂等键必须为 `tenantId:activityId:eventInbox.idempotency_key`；同一事件同一活动重复上报不得新增第二条 participation。
8. 完全重复事件返回 duplicate 语义，不新增 participation、grant order 或 outbox。
9. 相同 `Idempotency-Key` 携带不同 payload 时返回 `INVALID_IDEMPOTENCY_KEY`，不新增 participation。
10. quota 不足时不得留下 `REWARDABLE` participation、grant order 或 outbox；应返回 `QUOTA_EXCEEDED` 并保留可审计的 quota 拒绝信息。
11. 模拟运行不写 participation，也不写 inbox、grant order、outbox 或 quota。
12. PostgreSQL 验证必须覆盖成功发奖、规则拒绝、零奖励、重复事件、同 key 不同 payload、quota 不足六类场景；最低仍需满足 `go test ./...` 和 `go build ./...`。

## 待确认问题

当前材料足以按默认最小可验收口径进入研发，无 Decision BLOCK。以下问题不阻塞本需求，但建议后续任务明确：

- 是否需要为“完全未进入候选”的活动也写 `REJECTED` participation？本轮默认不写，只要求已进入候选且形成拒绝结论的活动可追溯。
- 是否需要新增面向管理端的 participation 查询 API？本轮非目标，不新增 API；可通过现有对账/数据库验证。
- snapshot 是否需要脱敏策略字段白名单？本轮默认只保存标准事件 payload 和活动配置摘要，不保存 Authorization/token/secret；更细粒度脱敏可作为后续安全任务。

## Score

```text
88
```

评分拆分：

```text
需求清晰度        18 / 20
范围可控          18 / 20
风险可控          17 / 20
验收标准明确      18 / 20
架构一致性        17 / 20
```

## Decision

```text
PASS
```

理由：本轮已明确 participation 的输入输出、状态流转、快照内容、幂等、异常场景、非目标范围和 PostgreSQL 验收口径；剩余问题均有默认最小可验收口径，不阻塞进入 Review/Dev。
