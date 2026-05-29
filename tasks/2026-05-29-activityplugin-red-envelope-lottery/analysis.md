# Analysis

## 需求 ID

2026-05-29-activityplugin-red-envelope-lottery

## 需求标题

activityplugin-red-envelope-lottery: 红包雨与抽奖插件样板

## 资料来源

- `tasks/2026-05-29-activityplugin-red-envelope-lottery/task.md`
- `tasks/2026-05-29-activityplugin-red-envelope-lottery/review.md`
- `README.md`
- `docs/activityplugin-lifecycle.md`
- `docs/api.md`
- `docs/data-model.md`
- `requirements/backlog.json`
- `tasks/2026-05-24-activityplugin/analysis.md`
- `tasks/2026-05-29-activityplugin-lifecycle/analysis.md`
- `tasks/2026-05-29-activityplugin-context-state/analysis.md`
- `tasks/2026-05-29-activityplugin-scheduler-events/analysis.md`

本轮只做需求分析和需求细化。不读取代码，不写代码，不改代码，不提交，不设计具体代码改法。

## Review 反馈处理

`review.md` 当前为模板内容，包含 `P0:`、`P1:`、`P2:`、`Open Questions 待填写` 和 `PASS / BLOCK` 判定规则，但没有实际填写的 Review finding、开放问题或打回原因。

处理结果：

- P0：无实际条目；无需针对具体 P0 修正。
- P1：无实际条目；无需针对具体 P1 修正。
- P2：无实际条目；无需针对具体 P2 修正。
- Open Questions：为模板占位，不是 Review Agent 提出的具体问题；本轮基于任务说明、ActivityPlugin 生命周期、状态隔离、调度事件、API 和数据模型补齐默认最小可验收口径。
- Decision：模板中的 `PASS / BLOCK` 是规则说明，不是实际 BLOCK 结论；不作为 Review 打回阻塞。

## Analysis 自检修正

上一轮 `analysis.md` 是初始模板，存在以下 BLOCK 原因和扣分项：

- Score 为 `0`，低于 80。
- Decision 为 `PASS / BLOCK` 占位，没有有效结论。
- 背景、目标、细化需求、业务规则、输入输出、异常场景、边界条件、非目标、风险、验收方式和待确认问题均为“待填写”。
- 未明确 RED_ENVELOPE 与 LOTTERY 的玩法职责、配置 Schema、结果输出、随机审计、库存/预算边界、模拟运行输出和验收对象。

修正结果：

- 已补齐背景目标、参与方、输入输出、业务规则、边界条件、异常场景、非目标范围、风险和验收标准。
- 已给出默认最小可验收口径：RED_ENVELOPE 和 LOTTERY 是样板插件，只生产玩法上下文、候选奖励、命中/未命中原因和可审计随机解释；不直接发奖、不扣 quota、不写 participation/grant_order/outbox、不调用外部发奖系统。
- 已明确缺口默认约束：库存、预算、用户次数和真实发奖仍由主引擎 quota/reward/grant/outbox 链路承担；插件中的 pool 或 item 只用于候选与解释，不作为最终库存事实。
- 已关闭旧 BLOCK：本轮 Score 为 91，Decision 为 PASS。

## 背景和已知信息

ExMarketing 的核心链路统一负责标准事件接入、幂等、活动匹配、规则、奖励、quota、participation、grant_order、outbox、callback、审计和对账。ActivityPlugin 是受控扩展点，只能构建玩法上下文、校验配置、返回候选结果、提出状态变更意图或处理插件调度事件。

已有材料明确：

- 插件上下文只允许通过 `state.plugin.<pluginCode>.*` 暴露给规则和奖励。
- 插件不得直接写 participation、grant_order、outbox、quota，也不得直接调用外部奖励系统。
- 插件执行结果需要进入 participation snapshot 或 audit，包含 `pluginCode`、`pluginVersion`、`lifecycleStep`、`status`、`reasonCode`、`pluginContext`、`precheckResult`、`stateChangeIntents`。
- 模拟运行不写真实 event_inbox、participation、grant_order、outbox、quota 或真实插件状态。
- 插件调度事件必须转成稳定标准事件进入 Event Inbox，不得绕过标准幂等链路。

本需求用两个典型玩法验证 ActivityPlugin 契约是否足够：

- RED_ENVELOPE：偏场次、红包池、库存抢占、用户抢红包幂等、空包/命中原因。
- LOTTERY：偏抽奖次数、概率、候选奖项、随机种子或可审计随机来源、结果审计。

## 目标理解

本需求目标是实现两个“最小样板插件”来证明插件边界可用，而不是交付完整营销前台玩法：

- 提供 `RED_ENVELOPE` 插件样板，支持场次窗口、红包池、用户一次抢包语义、命中/未命中原因和模拟运行解释。
- 提供 `LOTTERY` 插件样板，支持抽奖次数、奖项候选、概率抽取、稳定随机解释和模拟运行解释。
- 提供两个插件的配置模板样例，用于后续管理后台配置和模拟运行演示。
- 插件输出玩法上下文、候选奖励、原因和状态变更意图；主引擎继续负责真实奖励配置、库存、预算、参与记录、发奖单和 outbox。
- 插件失败只影响绑定该插件的当前活动，不影响同一事件下其他普通活动或其他插件活动。

## 参与方

- 活动运营：配置红包雨或抽奖活动，查看模拟运行、命中原因和审计解释。
- 标准事件生产方：仍只上报标准事件，不直接调用插件专用接口。
- 活动主引擎：负责活动匹配、规则、奖励计算、quota、participation、grant_order、outbox、审计和模拟运行。
- RED_ENVELOPE 插件：根据场次窗口、红包池和用户抢包状态输出抢包上下文、候选奖励和命中/空包原因。
- LOTTERY 插件：根据抽奖次数、概率奖项和稳定随机输入输出抽奖候选结果和审计解释。
- PluginStateStore：承载玩法过程状态，例如用户已抢/已抽次数、场次内用户命中记录或插件事件流；不得承载最终发奖事实。
- Review/Test/Dev Agent：基于本文件审核、设计测试和落地实现。

## 细化需求

### 插件注册与活动绑定

- 必须注册两个插件编码：`RED_ENVELOPE` 和 `LOTTERY`。
- 两个插件均必须声明版本、能力、支持活动类型、配置 Schema 和状态/输出 Schema。
- 默认版本口径为 `1.0.0`；如活动配置版本范围，需按生命周期契约解析为具体版本。
- `RED_ENVELOPE` 默认支持活动类型 `RED_ENVELOPE`。
- `LOTTERY` 默认支持活动类型 `LOTTERY`。
- 活动配置 `pluginCode` 与 `activityType` 不兼容时，创建、更新、审批提交或上线前校验必须拒绝。
- 插件必须至少支持 `CONFIG_VALIDATE` 和 `CONTEXT_BUILD`。如果实现真实用户抢包/抽奖次数状态消费，则还必须通过 PluginStateStore 的幂等状态写入或等价状态变更意图表达，不得用进程全局变量替代。

### RED_ENVELOPE 配置 Schema 最小口径

活动扩展配置中应包含受控插件配置区，最小字段语义如下，具体 JSON 包装位置可沿用现有活动配置扩展风格，但不得新增非标准事件顶层字段：

- `rounds`：场次数组，必须非空。
- `roundCode`：场次编码，同一活动版本内唯一。
- `startAt` / `endAt`：场次窗口，必须为 RFC3339 带时区时间，且 `startAt < endAt`。
- `poolCode`：红包池编码，同一活动版本内可复用但必须可审计。
- `poolTotalCount`：本场候选红包数量，正整数。
- `emptyAllowed`：是否允许空包；默认 `true`。
- `perUserLimit`：每个用户在同一活动或同一场次的抢包次数上限，默认 `1`。
- `display`：展示文案摘要，例如标题、按钮文案、未开始/已结束/空包文案；仅用于 snapshot/simulate 展示，不参与发奖事实。
- `candidateRewards`：候选奖励引用列表，至少包含 `rewardCode` 或候选编码；真实奖励金额、库存和发奖配置仍以活动 `rewards` 与 quota 为准。

默认约束：

- `poolTotalCount` 用于插件候选和模拟解释，不替代 `STOCK` quota。
- 如果需要严格库存，必须配置主引擎 `STOCK + ACTIVITY + rewardCode` quota；缺少 quota 行按主引擎现有语义为不限额，但插件模拟应标记 `quotaUnlimited=true` 或同等风险提示。
- 场次窗口之外的事件不命中红包，输出明确 reasonCode，不进入可发奖候选。

### LOTTERY 配置 Schema 最小口径

活动扩展配置中应包含受控插件配置区，最小字段语义如下：

- `lotteryCode`：抽奖池编码，同一活动版本内唯一。
- `drawWindow.startAt` / `drawWindow.endAt`：抽奖可参与窗口；可省略，省略时默认使用活动 start/end 窗口。
- `perUserDrawLimit`：每个用户在活动版本内可抽次数，正整数，默认 `1`。
- `seedSalt`：可选稳定盐；用于审计随机输入，不能包含 secret。
- `items`：奖项候选数组，必须非空。
- `items[].itemCode`：奖项编码，同一抽奖池内唯一。
- `items[].rewardCode`：映射到活动奖励配置的奖励编码；未中奖项可为空或使用固定 `NO_PRIZE`。
- `items[].weight`：非负 Decimal 或整数权重；至少一个奖项权重大于 0。
- `items[].displayName`：展示名称，用于模拟和审计解释。
- `items[].noPrize`：是否为未中奖项；默认 `false`。

默认约束：

- 权重总和必须大于 0。
- 权重只决定插件候选结果，不替代 `PROBABILITY` reward calculator 或主引擎 quota 校验。
- 命中奖项若映射 `rewardCode`，该 `rewardCode` 必须存在于活动 enabled rewards 中，否则上线前校验拒绝。
- 未中奖项不生成候选发奖奖励，输出 `NO_PRIZE` 原因。

### 运行时输出

两个插件都必须输出结构化插件摘要，用于 participation snapshot、audit 和模拟运行：

- `pluginCode`
- `pluginVersion`
- `lifecycleStep`
- `status`：默认枚举 `PASSED` / `REJECTED` / `SKIPPED` / `ERROR`
- `reasonCode`
- `reason`
- `pluginContext`
- `candidateRewardCode` 或候选奖项/红包编码
- `hit`：是否命中候选奖励
- `stateChangeIntents`：例如记录用户已抢、已抽次数、场次命中记录；仅是主引擎授权状态写入意图或 snapshot 解释，不是发奖事实
- `audit`：随机、场次、权重、输入 key、配置版本等可追踪摘要

`pluginContext` 必须暴露在 `state.plugin.<pluginCode>.*` 下，供规则或奖励引用。不得使用 `plugin.*`、`state.<activityType>.*` 或短路径别名。

## 输入输出

### 输入

- 标准事件：
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
- 活动配置快照：
  - `activityId`
  - `activityCode`
  - `activityType`
  - `activityVersion`
  - `pluginCode`
  - 活动 start/end/timezone/status/approval
  - rules、rewards、quota 快照摘要
  - RED_ENVELOPE 或 LOTTERY 插件配置
- 运行时上下文：
  - 主引擎提供的 `now`
  - traceId
  - 规则阶段上下文
  - 可读 PluginStateStore 状态
  - 标准事件幂等键
- 模拟运行输入：
  - 与正式事件相同的标准事件 payload
  - 当前活动配置和只读 quota 快照

### 输出

- RED_ENVELOPE 插件输出：
  - 当前场次识别结果：`roundCode`、`roundStatus`、`poolCode`
  - 用户是否已抢：`alreadyClaimed`
  - 是否命中红包：`hit`
  - 候选 `rewardCode`
  - 空包/命中/拒绝 reasonCode
  - 状态变更意图：记录本用户本场抢包结果或抢包尝试
  - 审计摘要：场次、活动版本、幂等键、候选红包编号或稳定抽取位置
- LOTTERY 插件输出：
  - `lotteryCode`
  - 当前用户已抽次数和本次是否允许抽
  - 抽中的 `itemCode`
  - 候选 `rewardCode` 或 `NO_PRIZE`
  - 权重、总权重、稳定随机输入摘要、抽取位置
  - 状态变更意图：记录本用户本次抽奖结果和次数消费
- 主引擎后续输出：
  - 规则解释、奖励预览、quota impact、participationPreview、grantOrderPreviews、outboxCommandPreviews 仍由主引擎生成。
  - 插件只影响候选上下文和原因，不直接输出真实发奖单。

## 业务规则

### 通用规则

1. 插件只负责玩法上下文、候选结果、命中/未命中原因和审计解释。
2. 插件不得直接创建或修改 participation、grant_order、outbox、quota。
3. 插件不得直接发奖、调用钱包、调用外部 reward API、确认 callback 或写 MQ。
4. 插件不得绕过 Event Inbox 幂等；正式事件仍按标准幂等键处理。
5. 插件输出必须进入 participation snapshot 或 audit，使每次命中、空包、未中奖、配置错误和执行错误可追踪。
6. 插件失败只影响当前绑定该插件的活动，不影响同一事件下其他活动。
7. 模拟运行必须输出插件上下文和命中/未命中原因，但不得消费真实插件状态或扣 quota。

### RED_ENVELOPE 业务规则

- `occurredAt` 或主引擎定义的活动时间落在某个场次 `[startAt, endAt)` 内时，才可参与该场次。
- 同一活动版本中场次时间允许不重叠；默认最小口径为禁止同一活动内场次窗口重叠，避免一个事件同时命中多个红包场次。
- 用户同一场次抢包幂等以标准事件幂等键和状态作用域共同约束；重复同一事件返回首次结果，不重复消费候选红包。
- `perUserLimit` 达到后输出 `USER_LIMIT_REACHED` 或等价 reason，不输出新的候选奖励。
- 红包池为空或候选位置未命中奖励时输出 `EMPTY_ENVELOPE` 或 `POOL_EMPTY`，不生成候选发奖。
- 插件候选红包命中后，真实库存仍由主引擎 `STOCK` quota 判断；quota 不足时主引擎拒绝，插件不得自行覆盖。
- 若 `emptyAllowed=false`，配置必须保证候选奖励能力与 `poolTotalCount` 有一致解释；无法保证时上线前校验应拒绝或要求配置主引擎库存。

### LOTTERY 业务规则

- 抽奖窗口外输出 `LOTTERY_NOT_STARTED` 或 `LOTTERY_ENDED`，不产生候选奖项。
- 用户抽奖次数达到 `perUserDrawLimit` 后输出 `DRAW_LIMIT_REACHED`，不产生新的候选奖项。
- 每次有效抽奖必须基于稳定随机输入产生可复现抽取结果。
- 默认随机输入组成：`tenantId + activityId + activityVersion + pluginCode + lotteryCode + subject.type + subject.id + event idempotency key + drawIndex + seedSalt`。
- `seedSalt` 不得是 secret；审计可记录 seed 输入摘要或 hash，不应记录敏感信息。
- 权重为 0 的奖项不会被抽中；所有权重为 0 必须配置校验失败。
- 命中 `noPrize=true` 或 `itemCode=NO_PRIZE` 时输出未中奖原因，不产生候选发奖。
- 命中有 `rewardCode` 的奖项后，真实奖励、库存、预算、次数和发奖仍由主引擎校验和执行。

## 配置校验规则

- 未注册插件、插件禁用、版本不兼容、activityType 不支持、能力缺失时，活动创建、更新、审批提交或上线前校验必须拒绝。
- `RED_ENVELOPE.rounds` 为空、场次时间非法、场次编码重复、窗口重叠、`poolTotalCount <= 0`、`perUserLimit <= 0` 时拒绝。
- `RED_ENVELOPE.candidateRewards` 引用不存在的 enabled `rewardCode` 时拒绝。
- `LOTTERY.items` 为空、`itemCode` 重复、`weight < 0`、总权重为 0、`perUserDrawLimit <= 0` 时拒绝。
- `LOTTERY.items[].rewardCode` 引用不存在的 enabled `rewardCode` 时拒绝；`noPrize=true` 项可不配置 `rewardCode`。
- 所有 Decimal、时间和布尔字段必须可解析；未知高风险字段不能静默忽略。
- 规则引用插件上下文时，只允许绑定插件自己的路径，如 `state.plugin.RED_ENVELOPE.hit` 或 `state.plugin.LOTTERY.itemCode`。

## 异常场景

- 插件配置非法：上线前 BLOCK，错误码使用 `PLUGIN_CONFIG_INVALID` 或等价结构化错误。
- 插件未注册、禁用、版本不兼容或能力缺失：活动不能上线；正式运行遇到时当前活动拒绝，不写 grant_order/outbox。
- RED_ENVELOPE 无可用场次：返回 `ROUND_NOT_FOUND` 或 `ROUND_NOT_ACTIVE`。
- RED_ENVELOPE 用户重复抢同一场次：返回首次结果或 `ALREADY_CLAIMED`，不得重复消费。
- RED_ENVELOPE 候选命中但主引擎库存不足：主引擎返回 quota 风险或拒绝，插件结果保留为候选解释，不发奖。
- LOTTERY 用户重复投递同一事件：返回首次抽奖结果，不重复消费次数。
- LOTTERY 权重配置导致无法抽取：上线前拒绝；运行时如发现，返回 `PLUGIN_CONFIG_INVALID` 并拒绝当前活动。
- 随机输入缺少必要字段：返回 `PLUGIN_EXECUTION_FAILED` 或结构化配置错误，不默认放行。
- PluginStateStore 幂等冲突：当前活动拒绝，记录冲突原因，不生成发奖副作用。
- 模拟运行遇到 quota wouldExceed：展示插件命中和 quota 风险，但不展示可执行 grant/outbox 预览。
- 审计写入失败：不得静默丢失关键插件执行失败或状态消费审计；按高风险错误处理。

## 边界条件

- 多租户：插件状态、候选结果、审计和模拟必须带 `tenantId`，不得跨租户读取。
- 活动版本：同一活动不同 `activityVersion` 的插件配置和玩法状态默认隔离，不做自动迁移。
- 时间语义：配置时间必须带时区；窗口判断按统一时间比较，展示可按活动 timezone。
- 随机确定性：同一活动版本、同一事件幂等键、同一 drawIndex、同一配置下，LOTTERY 结果必须可复现。
- 并发：同一用户同一场次抢最后一个候选红包，或同一用户并发抽最后一次机会时，只能有一个状态消费成功；失败方返回冲突、已抢或次数已达上限。
- 缺少 quota 行：主引擎当前语义为不限额；插件不得自行假设有库存限制，模拟输出应能解释该风险。
- 奖励金额：插件不计算最终金额；金额类奖励仍使用主引擎 Decimal reward calculator，输出 4 位小数。
- 非金额权益：插件候选 `rewardCode` 可指向 COUPON/POINT/GIFT/DRAW_CHANCE/MEMBERSHIP 等主引擎支持的奖励类型，但发奖 payload 仍由主引擎生成。

## 非目标范围

- 不做真实前端红包雨动画、抽奖转盘或交互页面。
- 不做 WebSocket 推送。
- 不做大规模排行榜。
- 不新增玩法专用发奖接口。
- 不绕过主引擎 reward、quota、participation、grant_order、outbox。
- 不实现复杂概率风控、黑名单、动态奖池补货、实时库存看板或运营手动派奖。
- 不承诺插件配置可覆盖所有商业抽奖玩法；本期只做验证 ActivityPlugin 契约的最小样板。
- 不把插件 pool 余额当作财务预算、库存或最终发奖事实。

## 风险点

- 风险等级高：红包雨和抽奖容易被误实现为直接扣库存或直接发奖，必须用验收约束插件边界。
- 随机审计风险：LOTTERY 如果使用当前时间或不可复现随机源，会导致重复事件、审计和模拟不一致。
- 并发风险：抢红包和抽奖次数消费若未遵守 PluginStateStore 幂等与 CAS 语义，会重复消费玩法状态。
- 库存边界风险：插件红包池与主引擎 STOCK quota 容易混淆，需要明确插件只输出候选，真实库存由 quota 决定。
- 模拟误导风险：模拟运行展示命中不等于 quota 已锁定或奖励已发放，输出必须说明预览性质。
- 配置风险：权重、场次和 rewardCode 引用若不在上线前校验，会在活动运行时变成高风险故障。

## 验收标准

1. `RED_ENVELOPE` 和 `LOTTERY` 两个插件可注册，并声明版本、能力、支持 activityType、配置 Schema 和输出 Schema。
2. 配置未知插件、禁用插件、版本不兼容、activityType 不兼容或能力缺失的活动会在创建、审批提交或上线前校验被拒绝。
3. RED_ENVELOPE 合法配置可通过校验；非法场次、重复场次编码、窗口重叠、非法 pool 数量、非法用户次数、未知 rewardCode 会被拒绝。
4. LOTTERY 合法配置可通过校验；空奖项、重复 itemCode、负权重、总权重为 0、非法次数、未知 rewardCode 会被拒绝。
5. RED_ENVELOPE 模拟运行能输出场次、是否命中、候选 rewardCode、空包/未开始/已结束/已抢/次数限制原因和插件上下文。
6. LOTTERY 模拟运行能输出抽奖次数、抽中 itemCode、候选 rewardCode 或未中奖、权重解释、稳定随机摘要和插件上下文。
7. 正式事件处理中，插件结果进入 participation snapshot 或 audit，可按活动、事件、用户、插件编码追溯。
8. LOTTERY 在同一活动版本、同一事件幂等键、同一 drawIndex 和同一配置下结果可复现；重复事件不重复消费次数。
9. RED_ENVELOPE 同一用户同一场次重复抢包不会重复消费状态；并发抢包不会让同一幂等事件生成多个结果。
10. 插件不直接写 grant_order、outbox、quota，不直接调用外部发奖系统。
11. 插件候选命中奖励后，预算、库存、用户次数和发奖单仍由主引擎处理；quota 不足时不得生成成功 grant_order/outbox。
12. 插件失败只拒绝当前绑定活动，不影响同一事件下其他活动。
13. 模拟运行不写 event_inbox、participation、grant_order、outbox、quota 或真实插件状态。
14. 插件输出字段只能通过 `state.plugin.RED_ENVELOPE.*` 和 `state.plugin.LOTTERY.*` 暴露；其他路径被配置校验拒绝。
15. 提供 RED_ENVELOPE 与 LOTTERY 活动配置模板样例、模拟运行样例和插件与主引擎职责边界说明。
16. `go test ./...` 通过。

## 待确认问题

以下问题不阻塞研发，本轮给出默认最小可验收口径：

- RED_ENVELOPE 是否必须做到严格红包池库存扣减？默认不做财务/库存事实扣减；插件只做候选和状态幂等，真实库存用主引擎 `STOCK` quota。
- LOTTERY 是否使用加密安全随机？默认使用可复现、可审计的稳定随机输入；本期不要求不可预测性，用于样板和审计复现。
- 插件配置 JSON 的具体包装字段名是否固定？默认沿用活动扩展配置的受控插件配置区；实现可按现有项目风格落地，但必须能被创建、上线前校验、snapshot 和模拟运行解释。
- 是否需要专门管理后台页面？默认不需要；本期交付配置模板和模拟样例即可支持后续后台演示。
- 是否需要插件调度自动触发场次开始/结束？默认样板可消费调度任务能力，但本期验收重点是插件可注册、校验和模拟/事件运行；完整调度 UI 不在范围内。

## Score

```text
需求清晰度        18/20
业务规则完整      18/20
边界异常清楚      19/20
验收标准明确      18/20
约束一致性        18/20
总分              91/100
```

扣分说明：具体 JSON 包装字段名和管理后台呈现形态允许 Dev 阶段沿用现有项目风格，未在 Analysis 中强行规定；但插件职责、输入输出、业务规则、边界异常和验收对象已经清楚，不阻塞 Review、Dev 和 Test。

## Decision

```text
PASS
```

需求：2026-05-29-activityplugin-red-envelope-lottery
