# Analysis

## 需求名称

activityplugin-context-state: 插件上下文与状态隔离

## Review 反馈处理

- `review.md` 当前为模板状态，未给出具体 P0/P1/P2 内容；但文件中保留了 `Open Questions` 和 `PASS / BLOCK` 判定模板。处理方式：本轮不把模板文本视为真实 Review 打回项，但按返工要求补齐所有原 analysis 空白，并显式给出可审核、可开发、可测试的默认最小口径。
- Review 可能关注的主要缺口是插件状态边界、幂等、隔离、并发、审计和验收对象不清。处理方式：本轮已分别在“业务规则”“输入输出”“异常场景”“边界条件”“验收标准”中细化，并关闭“字段名、错误码、配置来源、协议细节需 Owner 决策”的潜在 BLOCK。
- 若后续 Review 需要更严格错误码，本轮默认复用已有插件错误码体系，并补充状态访问类错误语义；该默认口径足以支撑测试，不阻塞研发。

## Analysis 自检修正

- 上一轮 `analysis.md` 为模板，`Score = 0`，`Decision = PASS / BLOCK` 未决。修正：本轮完整补齐背景目标、参与方、输入输出、业务规则、边界异常、非目标、风险、验收标准，并重新评分。
- 上一轮所有章节均为“待填写”。修正：本轮以 `task.md`、`docs/activityplugin-lifecycle.md`、`docs/api.md`、`docs/architecture.md`、`docs/data-model.md` 为依据形成需求口径；没有读取代码，也没有用代码实现替代需求澄清。
- 旧 BLOCK 原因可归纳为验收对象和状态协议不清。修正：本轮定义 `PluginContext` 可读内容、`PluginStateStore` 最小能力、状态隔离键、幂等键、并发语义、审计摘要和禁止替代主引擎对象，按最小可验收口径关闭旧 BLOCK。

## 背景和已知信息

ExMarketing 已有标准事件、活动配置、规则、奖励、quota、participation、grant_order、outbox 和 ActivityPlugin 生命周期边界。现有文档规定：

- ActivityPlugin 是受控扩展点，可构建玩法上下文、校验配置、返回候选结果和状态变更意图。
- 插件不得直接写 participation、grant_order、outbox、quota，也不得直接调用外部发奖系统。
- 插件上下文只能通过 `state.plugin.<pluginCode>.*` 暴露给规则和奖励。
- 标准事件外部幂等键固定为 `tenantId + ":" + sourceSystem + ":" + eventType + ":" + bizType + ":" + bizId`。
- participation 快照已有 `pluginCode`、`pluginContext`、`precheckResult`、`stateChangeIntents` 等字段。

本需求是在既有插件生命周期基础上，补齐插件可读上下文和可写状态的边界，防止玩法状态跨租户污染、重复消费、并发覆盖，以及绕过主引擎的发奖事实链路。

## 目标理解

本需求目标是建立一套最小但完整的插件上下文与状态隔离契约：

- 插件运行时只能读取标准化、快照化、可审计的 `PluginContext`。
- 插件可写状态只能通过 `PluginStateStore` 的受控操作完成。
- 所有插件状态必须按 `tenantId`、`activityId`、`activityVersion`、`pluginCode`、`subject`、`biz`、`stateKey` 隔离。
- 所有写入必须带幂等键，重复事件或重复命令不得重复消费状态。
- 并发更新同一状态时不得静默覆盖或丢失。
- 插件状态只能作为玩法过程状态或规则上下文，不能成为最终发奖、扣 quota、生成 grant order 或投递 outbox 的事实来源。

## 参与方

- 标准事件生产方：提供 `eventId`、`eventType`、`sourceSystem`、`tenantId`、`subject`、`biz`、`occurredAt`、`attributes`。
- 活动主引擎：负责活动匹配、规则评估、奖励计算、quota、participation、grant_order、outbox 和主链路幂等。
- ActivityPlugin：基于上下文构建玩法状态、返回预检结果和状态变更意图，但不直接发奖或扣 quota。
- PluginStateStore：保存插件私有过程状态，提供最小读写、幂等、并发和审计能力。
- 审计/观测模块：记录关键插件状态变更 before/after 摘要、traceId、事件和错误原因。
- Test/Review Agent：基于本文件确认需求是否可开发、可测试、可验收。

## 细化需求

### PluginContext 可读范围

`PluginContext` 必须由主引擎构建并传入插件，插件不得自行从外部系统读取主链路事实。最小内容包括：

- 标准事件：`eventId`、`idempotencyKey`、`eventType`、`sourceSystem`、`tenantId`、`subject`、`biz`、`occurredAt`、`attributes`。
- 活动配置快照：`activityId`、`activityCode`、`activityType`、`activityVersion`、`pluginCode`、插件相关配置、规则和奖励摘要。
- 规则结果：当前生命周期步骤前已产生的 MATCH / QUALIFY / REWARD / RISK_LIMIT_BUDGET 摘要。
- 已解析 facts：只读 fact query 的成功结果和映射后的字段，不包含未授权原始响应。
- 当前时间：由主引擎提供的 `now`，用于确定性执行和测试；插件不得自行使用系统时间改变决策。
- `traceId`：用于串联事件、插件状态、participation 和审计。

默认最小口径：`PluginContext` 是单次事件处理的只读快照；插件不能修改其中内容，不能把 context 内对象引用作为可变共享状态。

### PluginStateStore 最小接口语义

`PluginStateStore` 至少支持以下能力，名称可按项目既有风格调整，但语义必须一致：

- `Get`：按完整状态作用域读取单个 `stateKey` 当前值和版本。跨 tenant 或作用域不完整时拒绝或返回无结果。
- `PutIfAbsent`：仅在目标状态不存在时创建；若同一幂等键重复提交，返回既有结果且不重复写审计事件；若状态已存在但幂等键不同，返回已存在/冲突语义，不能覆盖。
- `CompareAndSwap`：以 `expectedVersion` 或等价版本条件更新；版本匹配时写入并递增版本，版本不匹配时返回并发冲突，不能静默覆盖。
- `AppendEvent`：向插件状态事件流追加一条不可变事件；同一幂等键重复追加必须 no-op，不产生重复事件。
- `ListByRound`：按同一 tenant、activity、activityVersion、pluginCode、biz/round 范围列出状态或事件，用于场次类玩法查询；不得跨 tenant 或跨 pluginCode 汇总。

### 状态作用域

每条插件状态必须具备以下完整作用域：

- `tenantId`：必填，来自标准事件和活动配置一致性校验。
- `activityId`：必填，标识活动实例。
- `activityVersion`：必填，标识活动配置快照版本；不同版本状态默认隔离。
- `pluginCode`：必填，必须是活动绑定且注册可用的插件编码。
- `subject`：必填，默认由 `subject.type + ":" + subject.id` 构成；无用户主体的场次级状态可使用显式系统主体，如 `system:<bizId>`，但仍必须填充。
- `biz`：必填，默认由 `biz.bizType + ":" + biz.bizId` 构成；场次/订单/任务等业务对象必须落入该维度。
- `stateKey`：必填，由插件定义并受 `stateSchema` 约束；同一作用域下唯一表示一个状态对象。

默认最小口径：状态唯一键为 `tenantId + activityId + activityVersion + pluginCode + subject + biz + stateKey`。任何缺少上述维度的写入均拒绝；读取不得自动放宽维度。

### 幂等规则

- 写操作必须携带 `idempotencyKey`，优先使用标准事件幂等键；如插件内部派生命令需要独立幂等，则必须以标准事件幂等键为前缀追加稳定后缀。
- 状态幂等唯一键为 `idempotencyKey + pluginCode + stateKey`，并且必须落在完整状态作用域内。
- 同一作用域、同一幂等键、同一 `pluginCode`、同一 `stateKey` 的重复写入必须返回首次结果，不得重复消费抽奖次数、红包池余额、翻卡次数或事件流。
- 同一幂等键尝试写入不同 payload 时，默认按幂等冲突处理：不覆盖既有状态，并返回可审计错误。
- `eventId` 用于审计和追踪；当 `eventId` 与 `idempotencyKey` 同时存在时，业务幂等以 `idempotencyKey` 为准。

### 状态审计

关键状态变更必须记录审计摘要：

- 作用域：tenant、activity、activityVersion、pluginCode、subject、biz、stateKey。
- 来源：eventId、idempotencyKey、traceId、生命周期步骤。
- 操作：Get 之外的写操作类型，如 PutIfAbsent、CompareAndSwap、AppendEvent。
- 结果：created、updated、appended、duplicate_noop、conflict、rejected。
- before/after 摘要：只记录必要摘要、版本、计数、状态哈希或安全裁剪后的 JSON，不记录敏感明文或大对象。

默认最小口径：Get 不强制写审计；所有成功写、幂等 no-op、并发冲突和隔离拒绝都应可观测。

## 业务规则

1. 插件状态属于玩法过程状态，例如场次进度、用户抽奖次数、红包池过程余额、翻卡进度、插件事件流。
2. 主引擎事实仍由 participation、grant_order、quota、outbox 承载；插件状态不得作为最终发奖事实来源。
3. 插件不得直接修改 quota，不得直接创建或修改 grant_order/outbox，不得绕过 participation。
4. 插件输出的 `stateChangeIntents` 只能作为主引擎授权后的状态写入请求或 participation 快照内容，不能自动代表发奖完成。
5. `state.plugin.<pluginCode>.*` 是规则和奖励读取插件上下文的唯一允许路径；不得使用 `plugin.*`、`state.<activityType>.*` 或短路径别名。
6. 活动未配置 `pluginCode` 时，不应产生插件状态写入；普通活动继续走现有配置链路。
7. 配置了 `pluginCode` 的活动必须满足插件注册、版本兼容、activityType 支持、能力满足和未禁用等生命周期规则。
8. Memory 和 PostgreSQL 或等价持久化实现必须遵守同一幂等、隔离和并发语义；测试不能只验证单一实现的宽松行为。
9. 状态值必须符合插件声明的 `stateSchema` 或等价校验边界；非法结构不得写入。
10. 大排行榜、大规模实时聚合、最终结算事实不属于本状态存储职责。

## 输入输出

### 输入

- 标准事件及其幂等键。
- 活动配置快照，包括 `activityId`、`activityVersion`、`pluginCode` 和插件配置。
- 主引擎已解析 facts 和规则结果摘要。
- 插件状态操作请求：操作类型、完整状态作用域、`stateKey`、payload、`idempotencyKey`、可选 `expectedVersion`、traceId。

### 输出

- 插件上下文：以 `state.plugin.<pluginCode>.*` 形式提供给规则/奖励的只读上下文。
- 状态读取结果：存在/不存在、当前 payload 摘要、版本、更新时间。
- 状态写入结果：created、updated、appended、duplicate_noop、conflict 或 rejected。
- 审计记录：关键写入和异常的 before/after 摘要与追踪字段。
- participation 快照中的插件摘要：`pluginCode`、`pluginContext`、`precheckResult`、`stateChangeIntents`、执行状态和原因。

## 异常场景

- 缺少 `tenantId`、`activityId`、`activityVersion`、`pluginCode`、`subject`、`biz` 或 `stateKey`：拒绝写入，不能降级为全局状态。
- 请求 tenant 与事件 tenant 或活动 tenant 不一致：拒绝访问或返回无结果，必须可审计。
- 跨 tenant、跨 activity、跨 activityVersion、跨 pluginCode 或跨 stateKey 查询：默认无结果；写入必须拒绝。
- 重复事件重复写入：返回首次写入结果或 duplicate no-op，不重复扣减/累加/追加。
- 同一幂等键不同 payload：返回幂等冲突，不覆盖既有状态。
- 并发 CAS 更新同一状态：只有一个成功；失败方返回冲突，需要调用方重新读取后再决定。
- PutIfAbsent 并发创建同一状态：只有一个创建成功；其他请求返回已存在或 duplicate no-op。
- AppendEvent 并发追加不同幂等键：均可追加但事件顺序必须可追踪；同一幂等键只能出现一次。
- 插件缺失、禁用、版本不兼容、能力缺失或执行失败：当前活动对该事件拒绝，不生成 grant_order/outbox。
- 插件状态 schema 校验失败：拒绝写入，不生成状态变更。
- 审计写入失败：默认视为高风险错误；关键状态写入不得在完全无审计能力时静默成功。

## 边界条件

- `activityVersion` 变更后，新旧版本状态默认隔离；不做自动迁移。
- `ListByRound` 的 round 默认取 `biz.bizId`；如插件配置存在显式 roundId，则必须映射到 biz 或 stateKey 前缀，但仍不能跨 tenant/pluginCode。
- `subject` 为空的业务场景必须显式使用系统主体，不能省略 subject 维度。
- 插件状态 payload 应保持小对象；排行榜或大列表不纳入本需求。
- 当前时间只能来自主引擎传入的 `now`，用于测试可重复性。
- 模拟运行只可预览插件上下文和状态变更意图；默认不实际消费插件状态。
- PostgreSQL 持久化实现需要保证条件写入的原子性；Memory 实现需要在并发测试中表现出同等语义。
- 错误码命名可复用既有 `PLUGIN_*` 体系；状态访问类错误至少要能区分 invalid_scope、idempotency_conflict、version_conflict、schema_invalid、tenant_forbidden。

## 非目标范围

- 不实现具体红包池、抽奖池、翻卡、签到或排行榜算法。
- 不新增业务专用接口或专用页面模型。
- 不把插件状态作为最终发奖事实来源。
- 不改变 participation、grant_order、quota、outbox 的主引擎职责。
- 不做跨版本状态迁移、状态压缩、历史归档或大规模分析查询。
- 不要求插件直接调用外部奖励系统、钱包系统、MQ 或 HTTP 回调。
- 不扩大规则表达式字段路径；插件仍只能通过 `state.plugin.<pluginCode>.*` 暴露。

## 风险点

- 风险等级高：状态边界错误会导致重复发奖、跨租户污染、用户次数错误或 quota 与插件状态不一致。
- 幂等键设计不严格会使重复事件二次消费玩法状态。
- Memory 与 PostgreSQL 语义不一致会导致本地测试通过但生产并发丢失。
- 审计摘要过细可能泄露敏感信息，过粗又无法排查重复消费和并发冲突。
- 插件状态与主引擎事实边界若文档不清，后续玩法容易绕过 quota/grant_order/outbox。

## 验收标准

- 文档或接口说明明确列出插件状态与主引擎事实的边界：插件状态不替代 participation、grant_order、quota、outbox。
- `PluginContext` 至少包含标准事件、活动配置快照、规则结果、已解析 facts、当前时间和 traceId，并保持只读。
- 状态写入必须要求完整作用域；缺少 tenant 或跨 tenant 访问被拒绝或无结果。
- 同一 `idempotencyKey + pluginCode + stateKey` 重复写入不会重复消费状态，且返回可识别 no-op/首次结果。
- 同一状态并发 PutIfAbsent 只有一个创建成功。
- 同一状态并发 CompareAndSwap 不会覆盖或丢失更新，版本冲突可识别。
- AppendEvent 对同一幂等键只追加一次，对不同幂等键并发追加不丢失。
- 跨 tenant、跨 stateKey、跨 pluginCode、跨 activityVersion 的读取不能返回其他作用域状态。
- 关键写入保留 before/after 摘要、traceId、eventId/idempotencyKey、操作结果。
- 插件不能直接修改 quota/grant_order/outbox；对应测试或文档验收必须覆盖。
- Memory 与 PostgreSQL 或等价持久化实现的隔离、幂等、并发语义一致。
- `go test ./...` 通过；如项目要求构建验证，`go build ./...` 也应通过。

## 待确认问题

以下问题不阻塞研发，本轮给出默认最小可验收口径：

- 是否需要单独插件状态错误码命名？默认先使用可区分语义的内部错误分类，并可映射到既有 `PLUGIN_*` 审计体系。
- `ListByRound` 的 round 字段来源是否统一？默认取 `biz.bizId`，插件如需自定义 roundId 必须通过配置映射，但不得突破作用域。
- 模拟运行是否写插件状态？默认不写，只预览 context 和 stateChangeIntents。
- activityVersion 升级时是否迁移状态？默认不迁移，新旧版本隔离。

## Score

```text
需求清晰度        18/20
业务规则完整      18/20
边界异常清楚      19/20
验收标准明确      19/20
约束一致性        18/20
总分              92/100
```

## Decision

```text
PASS
```

需求：2026-05-29-activityplugin-context-state
