# Analysis

## Review 反馈处理

- P0：`review.md` 明确 P0 为 None，本轮无 P0 返工项。
- P1：Review 指出上一轮将插件标准事件 `occurredAt` 缺省为执行器 `now`，会导致同一 Event Inbox 幂等键在重试、重复执行、重放时 payload 不一致，从重复成功变成 `IDEMPOTENCY_CONFLICT`。本轮已修正为：调度插件事件的标准 payload 所有参与 Event Inbox 完整 payload 比较的字段都必须稳定；默认最小可验收口径为 `occurredAt = job.scheduledAt`。插件显式输出的发生时间只有在该时间来自稳定业务时间且首次生成后可重放一致时才可覆盖；禁止使用每次执行时的当前时间。若一个 job 产生多个标准事件，每个事件必须有稳定 `sequence`/业务 key，且 `eventId`、`biz.bizId`、`occurredAt`、`attributes` 中会进入 payload 比较的内容在重复执行时保持一致。
- P2：Review 指出调度/事件回传 capability 口径不清。根据 `docs/activityplugin-lifecycle.md` 当前只列出 `EVENT_HANDLE`，且任务目标是“插件产生内部事件并进入标准 Event Inbox”，本轮默认最小可验收口径为复用既有 `EVENT_HANDLE`，不新增 `SCHEDULE` / `EVENT_GENERATE` capability。只有注册插件具备 `EVENT_HANDLE`、未禁用、支持该 `activityType`，且是活动绑定插件时，才能注册或执行调度任务。缺失时使用 `PLUGIN_CAPABILITY_MISSING` 拒绝。
- Open Question：`occurredAt` 最终来源已关闭为默认 `job.scheduledAt`；插件稳定业务时间是可选覆盖但不是默认。能力模型 Open Question 已关闭为默认复用 `EVENT_HANDLE`，本需求不引入新的 capability 枚举。
- Score 76 / Decision BLOCK：旧 BLOCK 原因已通过上面两项默认口径关闭，本轮重新评分并给出 PASS。

## Analysis 自检修正

- 上一轮 Analysis 虽给出 `Score=88`、`Decision=PASS`，但存在错误假设：只要 `eventType + bizType + bizId` 稳定即可满足 Event Inbox 幂等。Review 证明该假设不完整，因为 Event Inbox 会比较完整标准 payload，`occurredAt` 等字段变化也会冲突。本轮已将“稳定幂等键”升级为“稳定幂等键 + 稳定标准 payload”。
- 上一轮扣分风险实际集中在幂等语义和插件能力校验。幂等语义已补充到字段级：`eventId`、`eventType`、`sourceSystem`、`tenantId`、`subject`、`biz`、`occurredAt`、`attributes` 均不得在同一幂等键重复执行时变化。插件能力校验已补充为复用 `EVENT_HANDLE`。
- 上一轮待确认问题中“插件业务 payload 是否统一”仍不阻塞，但本轮加严默认约束：放入 `attributes` 的插件字段也属于 payload 比较范围，必须来自 job 快照、活动配置快照或稳定插件业务结果，不得放入当前时间、随机数、临时错误文本、执行器 ID 等每次运行会变化的字段。

## 背景和目标

ExMarketing 的标准处理链路是 Event Intake -> Event Inbox 幂等 -> 活动匹配 -> 规则/插件上下文 -> quota/grant_order/outbox。`docs/api.md` 和 `docs/data-model.md` 明确标准事件幂等键固定为：

```text
tenantId + ":" + sourceSystem + ":" + eventType + ":" + bizType + ":" + bizId
```

重复事件只有在同一幂等键对应的完整 payload 一致时才是重复成功；同 key 不同 payload 是幂等冲突。ActivityPlugin 是受控扩展点，只能构建玩法上下文、校验配置、维护插件状态或提出状态变更意图，不得直接写 participation、grant_order、outbox、quota，也不得调用外部发奖系统。

本需求面向红包雨、抽奖、竞猜等“场次类玩法”。这些玩法除了用户上报事件，还需要系统在固定时间触发场次开始、结束、开奖、结算、补偿扫描等动作。目标是定义最小可用的插件调度与事件回传能力，并保证这些系统触发动作仍统一转成标准事件进入 Event Inbox，不绕过标准幂等、规则、quota 和发奖链路。

最小目标：

- 插件可为活动注册 `START`、`END`、`DRAW`、`SETTLE`、`COMPENSATE` 调度任务。
- 调度任务可 claim、执行、重试、失败停留、人工关闭，并有审计记录。
- 同一 `jobId` 只能产生一次业务成功；重复执行或重放不会重复发奖。
- 插件内部事件 `plugin.event.generated` 必须转换为稳定标准事件 payload 后进入 Event Inbox。
- 场次开始、结束、开奖、结算、补偿状态可通过调度任务或审计查询。

## 参与方

- 活动运营：配置场次类活动，查看场次任务状态，必要时人工关闭失败任务。
- ActivityPlugin：根据活动配置注册调度任务；任务执行时生成插件内部事件或无事件成功结果。
- 调度执行器：扫描到期任务，claim 后执行插件任务，处理状态流转、重试和失败分类。
- Event Inbox：接收插件事件转换后的标准事件，执行标准幂等和后续活动处理。
- 审计/运维：查看任务创建、claim、成功、失败、重试、关闭、事件转换和幂等冲突。
- 后续活动引擎：仍按既有活动状态、审批、规则、quota、participation、grant_order、outbox 处理标准事件。

## 需求范围

### 调度任务模型

最小模型为 `PluginScheduleJob`：

| 字段 | 说明 |
|---|---|
| `jobId` | 调度任务唯一 ID；同一任务全局唯一，是调度幂等主键 |
| `tenantId` | 租户 ID；任务、事件、审计、查询均不得跨租户 |
| `activityId` | 活动 ID；任务只作用于该活动 |
| `pluginCode` | 插件编码；必须是活动绑定插件 |
| `jobType` | 最小枚举 `START` / `END` / `DRAW` / `SETTLE` / `COMPENSATE` |
| `scheduledAt` | 计划执行时间，输入必须带时区，内部按 UTC 比较 |
| `status` | `PENDING` / `RUNNING` / `SUCCESS` / `FAILED` / `RETRY` / `CLOSED` |
| `retryCount` | 已重试次数，从 0 开始递增 |
| `nextRetryAt` | 下次可执行时间 |
| `errorClass` | 失败分类 |
| `errorMessage` | 脱敏错误摘要 |
| `lastRunAt` | 最近一次执行时间 |
| `closedBy` / `closedReason` | 人工关闭时必填 |
| `createdAt` / `updatedAt` | 创建和更新时间 |

最小验收不要求复杂管理 UI；但无论内存仓储还是 PostgreSQL 仓储，都必须能表达、查询和审计上述字段。

### 插件能力和注册校验

- 默认复用既有插件 capability `EVENT_HANDLE` 表示插件具备处理调度事件回传的能力；本需求不新增 `SCHEDULE`、`EVENT_GENERATE` 或其他 capability。
- 只有满足以下条件的插件可注册或执行调度任务：插件已注册、未禁用、支持当前 `activityType`、具备 `EVENT_HANDLE`、且 `pluginCode` 是活动配置绑定插件。
- 插件缺失、禁用、活动类型不兼容、缺少 `EVENT_HANDLE` 时必须拒绝任务注册或执行，并写入明确失败分类。
- 同一 `tenantId + activityId + pluginCode + jobType + scheduledAt` 重复注册时，返回已有任务或幂等成功，不创建多个等价任务。
- `scheduledAt` 不允许为空、无时区、无法解析或是无业务意义的历史时间。补偿历史动作必须用 `COMPENSATE` 表达，并立即具备执行资格。

### 任务执行状态机

允许状态流转：

```text
PENDING -> RUNNING -> SUCCESS
PENDING -> RUNNING -> RETRY -> RUNNING -> SUCCESS
PENDING -> RUNNING -> FAILED -> RETRY
PENDING -> RUNNING -> FAILED -> CLOSED
RETRY   -> RUNNING -> RETRY / SUCCESS / FAILED
FAILED  -> RETRY / CLOSED
```

约束：

- `SUCCESS` 和 `CLOSED` 是终态，不允许再次进入 `RUNNING`。
- 同一 `jobId` 同一时刻只能有一个执行者 claim 成功。
- 执行器扫描 `status=PENDING` 且 `scheduledAt <= now`，以及 `status=RETRY` 且 `nextRetryAt <= now` 的任务。
- 进程异常导致的 `RUNNING` 悬挂任务，最小口径允许补偿扫描转为 `RETRY`；但已经确认 `SUCCESS` 的任务不得重新执行。
- 任务到期不代表一定发奖；调度任务只负责产生标准事件，后续仍由活动引擎判断活动状态、审批、规则、quota 和奖励。

## 插件事件回传

插件执行调度任务后可产生一个或多个 `plugin.event.generated` 内部事件。每个内部事件必须转换为标准事件 payload，并通过 Event Inbox 进入标准处理链路。

标准事件最小字段：

| 标准字段 | 默认来源与稳定性要求 |
|---|---|
| `tenantId` | `job.tenantId`，必须稳定 |
| `eventId` | 稳定生成，默认 `plugin:<jobId>:<sequence>` |
| `eventType` | 由 `jobType` 或插件事件类型稳定映射，如 `PLUGIN_ROUND_STARTED`、`PLUGIN_ROUND_ENDED`、`PLUGIN_DRAWN`、`PLUGIN_SETTLED`、`PLUGIN_COMPENSATED` |
| `sourceSystem` | 固定为 `activity_plugin` |
| `subject.type` | 场次类系统事件默认 `SYSTEM`；若插件事件明确用户主体，必须来自稳定插件业务结果 |
| `subject.id` | 系统事件默认 `pluginCode`；用户事件为稳定用户 ID |
| `biz.bizType` | 默认 `PLUGIN_SCHEDULE_JOB` |
| `biz.bizId` | 单事件默认 `jobId`；多事件默认 `jobId:<sequence>` 或稳定业务 key |
| `occurredAt` | 默认 `job.scheduledAt`，禁止默认使用执行器 `now` |
| `attributes` | 至少包含 `activityId`、`pluginCode`、`jobId`、`jobType`、`scheduledAt`、`sequence`；其他字段必须稳定 |

`occurredAt` 默认规则：

- 默认采用 `job.scheduledAt`，这是本需求的最小可验收口径。
- 插件可显式提供稳定业务发生时间覆盖默认值，但该时间必须来自活动配置、任务快照、round/scene 固定时间或首次生成后可重放的插件状态，不得来自每次执行时的当前时间。
- 对 `COMPENSATE`，默认仍使用被补偿业务对象的稳定计划时间；若无法确定，则使用 `job.scheduledAt`，而不是补偿执行时刻。
- 同一 Event Inbox 幂等键的重复执行、重试、重放必须生成字节语义一致的标准 payload；若无法保证，应视为 `EVENT_MAPPING_INVALID`，不得把不稳定事件写入 Event Inbox。

## 幂等与不重复发奖

Event Inbox 幂等键沿用既有规则：

```text
tenantId + ":" + sourceSystem + ":" + eventType + ":" + bizType + ":" + bizId
```

调度插件事件必须同时满足：

- 幂等键稳定：`tenantId`、`sourceSystem`、`eventType`、`biz.bizType`、`biz.bizId` 在重复执行时不变。
- 标准 payload 稳定：`eventId`、`subject`、`occurredAt`、`attributes` 等进入 payload 比较的字段在重复执行时不变。
- 多事件稳定：同一 job 产生多个事件时，每个事件必须有稳定 `sequence` 或业务 key；数组顺序变化不得导致漏事件、重复事件或 payload 冲突。
- 重复成功语义：Event Inbox 返回“重复且 payload 一致”时，调度任务可视为业务成功。
- 冲突语义：Event Inbox 返回同 key 不同 payload 时，任务不得标记 `SUCCESS`，必须记录 `IDEMPOTENCY_CONFLICT` 并停留为不可自动重试的失败。
- 发奖边界：调度任务和插件执行器不得直接创建 `grant_order`、`outbox`、quota reservation 或调用外部发奖系统。

禁止写入标准 payload 的不稳定字段包括但不限于：当前执行时间、随机数、执行器实例 ID、临时 goroutine/进程信息、每次变化的错误文本、未排序 map 直接序列化结果、非确定性数组顺序。

## 业务规则

- `jobType` 必须在最小枚举内；扩展类型需先更新需求、文档和测试。
- `START` 表示场次开始类系统事件；`END` 表示场次结束类系统事件；`DRAW` 表示开奖/抽取类系统事件；`SETTLE` 表示结算类系统事件；`COMPENSATE` 表示修复漏执行、悬挂或补偿扫描类系统事件。
- `scheduledAt` 到期只说明任务具备执行资格，不代表活动可发奖。正式奖励仍由标准事件链路决定。
- 场次状态应通过调度任务状态、插件状态审计或标准事件处理结果体现，不得用 grant_order 或 outbox 表达场次状态。
- 人工关闭只关闭调度任务，不表示奖励成功、失败或 quota 释放；奖励终态仍由既有 callback、对账或人工发奖流程处理。
- 插件输出空事件：对 `START/END/DRAW/SETTLE` 默认允许成功但必须审计“无事件输出”；若插件声明该 jobType 必须产出事件，则空输出为 `EVENT_MAPPING_INVALID`。`COMPENSATE` 可无事件成功，但必须审计补偿检查结果。

## 输入输出

### 输入

- 活动配置：`tenantId`、`activityId`、`activityType`、`pluginCode`、`config` 中的场次、开奖、结算、补偿时间配置。
- 插件注册元数据：`pluginCode`、`version`、`capabilities`、`supportedActivityTypes`、`disabled`。
- 调度任务：`jobId`、`tenantId`、`activityId`、`pluginCode`、`jobType`、`scheduledAt`、状态和重试字段。
- 执行上下文：任务快照、只读活动/plugin 信息、traceId、执行器当前时间。执行器当前时间只用于到期判断、审计 `lastRunAt` 和重试计划，不得默认进入标准事件 `occurredAt`。
- 插件输出：一个或多个 `plugin.event.generated`、无事件成功结果或结构化错误。

### 输出

- 调度任务状态更新：`RUNNING`、`SUCCESS`、`RETRY`、`FAILED`、`CLOSED`，以及 `retryCount`、`nextRetryAt`、`errorClass`、`errorMessage`。
- 标准 Event Inbox 输入 payload：由插件内部事件转换而来，使用稳定幂等键和稳定完整 payload。
- 审计记录：任务创建、重复注册、claim、成功、失败、重试、人工关闭、插件事件转换、Event Inbox 重复成功或幂等冲突。
- 查询结果：至少能按 `tenantId + activityId + pluginCode` 查询各 `jobType` 的当前状态、计划时间、最近执行时间、失败摘要和关闭信息。

## 失败分类和异常场景

失败分类最小枚举：

| 分类 | 说明 | 默认处理 |
|---|---|---|
| `PLUGIN_NOT_FOUND` | 插件不存在或未注册 | 不可重试，进入 `FAILED` 后可关闭 |
| `PLUGIN_DISABLED` | 插件禁用 | 不可重试，等待配置恢复或关闭 |
| `PLUGIN_CAPABILITY_MISSING` | 插件缺少 `EVENT_HANDLE` | 不可重试 |
| `PLUGIN_ACTIVITY_TYPE_UNSUPPORTED` | 插件不支持该活动类型 | 不可重试 |
| `ACTIVITY_NOT_FOUND_OR_INACTIVE` | 活动不存在或不允许执行 | 默认不可重试，配置恢复后可人工重试 |
| `PLUGIN_EXECUTION_FAILED` | 插件执行异常 | 可重试 |
| `EVENT_MAPPING_INVALID` | 插件事件无法转换为稳定标准事件 | 不可重试 |
| `EVENT_INBOX_FAILED` | Event Inbox 写入或处理失败 | 可重试 |
| `IDEMPOTENCY_CONFLICT` | 同幂等键对应不同 payload | 不可重试，必须人工介入 |
| `TIMEOUT` | 插件或 Event Inbox 超时 | 可重试 |
| `INTERNAL_ERROR` | 未分类内部错误 | 可重试，达到上限后停在 `FAILED` |

异常规则：

- 重复执行已 `SUCCESS` 的 `jobId`：不得再次执行插件或写入新业务结果；可返回重复成功或拒绝执行，并写审计。
- 两个执行器同时 claim：只允许一个成功，失败方不得执行插件。
- `RUNNING` 任务执行器崩溃：补偿扫描只能在确认任务未进入终态后转 `RETRY`。
- Event Inbox 重复：同 key 且 payload 一致，视为幂等成功。
- Event Inbox 幂等冲突：同 key 不同 payload，记录 `IDEMPOTENCY_CONFLICT`，不得自动重试。
- 人工关闭缺少 `operator` 或 `reason`：不允许关闭。
- 错误、审计和指标不得记录 token、Authorization、Cookie、secret 或完整敏感 payload。

## 边界条件

- 多租户隔离：任务、事件、插件状态、审计和查询必须带 `tenantId`。
- 时间语义：输入时间必须为带时区 RFC3339；运行比较统一按 UTC；活动展示可按活动 `timezone`。
- 默认重试上限：无配置时最多 3 次重试；第 3 次后停在 `FAILED`，允许人工关闭或显式重试。
- 默认退避：无配置时使用 `1m, 5m, 15m`；测试可使用可控时钟或缩短间隔，不改变业务语义。
- 活动状态：正式标准事件处理仍只匹配审批通过且 `ACTIVE` 的活动；调度不能绕过审批和活动状态规则。
- Payload 稳定性：任何进入标准事件 payload 的字段都必须来源于任务快照、活动配置快照、稳定插件状态或稳定业务 key。

## 非目标范围

- 不接入 Quartz、Temporal、Airflow、K8s CronJob 等分布式调度平台。
- 不实现复杂日历排班 UI、运营拖拽排期或批量日历管理。
- 不新增 `SCHEDULE`、`EVENT_GENERATE` 等插件 capability；本需求默认复用 `EVENT_HANDLE`。
- 不允许调度任务直接创建 `grant_order`、`outbox`、quota reservation 或调用外部发奖。
- 不新增独立玩法专用发奖通道；插件事件必须走标准 Event Inbox。
- 不重做现有 ActivityPlugin 生命周期、规则引擎、奖励计算、outbox worker 或 reward callback。
- 不承诺实现所有玩法的完整业务逻辑；本需求只提供插件调度与插件事件回传基础能力。

## 风险点

- 标准 payload 稳定性是最高风险点；只稳定幂等键但让 `occurredAt` 或 `attributes` 变化，会触发 Event Inbox 幂等冲突。
- `RUNNING` 悬挂任务补偿不严谨，可能与实际成功结果冲突。
- 插件事件映射过于自由，会破坏标准事件字段约束、多租户隔离和审计可追溯。
- 调度成功与 Event Inbox 成功边界不清，会造成任务显示成功但业务事件未进入标准链路。
- 能力校验不严会导致任意注册插件创建或执行调度任务。
- 审计和错误摘要若未脱敏，会引入安全和合规风险。

## 验收标准

- 能创建或注册 `START/END/DRAW/SETTLE/COMPENSATE` 类型的插件调度任务，并按 `tenantId + activityId + pluginCode` 查询状态。
- 只有活动绑定、未禁用、支持活动类型且具备 `EVENT_HANDLE` 的插件可以注册和执行调度任务；缺失能力时返回或记录 `PLUGIN_CAPABILITY_MISSING`。
- 到期任务可被执行器 claim，并从 `PENDING/RETRY` 推进到 `RUNNING`，最终进入 `SUCCESS`、`RETRY`、`FAILED` 或 `CLOSED`。
- 并发 claim 同一任务时，只有一个执行者成功。
- 同一 `jobId` 重复执行、重试或重放时，不会重复生成会导致重复发奖的标准事件。
- 同一插件调度事件重复执行时，生成的标准 payload 必须保持一致；特别是 `occurredAt` 默认等于 `job.scheduledAt`，不得等于每次执行时的当前时间。
- Event Inbox 对同 key 同 payload 返回重复时，调度任务可视为成功；对同 key 不同 payload 返回冲突时，任务必须记录 `IDEMPOTENCY_CONFLICT` 且不得标记 `SUCCESS`。
- 插件产生的 `plugin.event.generated` 必须转换为标准事件 payload，并进入 Event Inbox；不得绕过 Event Inbox 直接进入发奖链路。
- 插件任务不得直接创建 `grant_order`、`outbox` 或 quota reservation。
- 失败任务记录失败分类、脱敏错误摘要、`retryCount`、`nextRetryAt`；达到默认重试上限后可人工关闭。
- 人工关闭必须记录 `operator` 和 `reason`，并写审计。
- 场次开始、结束、开奖、结算、补偿状态可通过调度任务查询或审计还原。
- `go test ./...` 必须通过；涉及 PostgreSQL 持久化语义时，应补充 PostgreSQL repository 验证。

## 待确认问题

以下问题不阻塞本轮研发，均已给出默认最小可验收口径：

- 是否需要对外开放完整调度管理 API？默认不要求复杂 UI/API；最小验收可通过 repository、测试和已有活动/审计查询证明。
- 每个 `jobType` 的插件私有 payload 是否统一？默认只规定标准事件外壳和必备 `attributes`，插件私有字段可放入 `attributes.pluginPayload`，但必须稳定且脱敏。
- 重试次数和退避是否租户可配置？默认固定 3 次、`1m/5m/15m`，后续再扩展配置化。
- `START/END/DRAW/SETTLE` 空事件是否允许？默认允许但必须审计；插件声明必须产出事件时按 `EVENT_MAPPING_INVALID`。
- 是否未来新增独立调度 capability？本轮默认不新增，复用 `EVENT_HANDLE`；未来如需细分权限，应另开需求迁移。

## Score

```text
91
```

评分说明：

- 需求清晰度：19/20，调度模型、插件事件模型、查询和审计对象明确。
- 业务规则完整：19/20，状态机、能力校验、幂等、标准链路边界和空事件口径明确。
- 边界异常清楚：18/20，失败分类、并发、悬挂、重复、冲突、脱敏均覆盖。
- 验收标准明确：18/20，包含 Review 打回的 `occurredAt` 稳定性和 `EVENT_HANDLE` capability 验收。
- 约束一致性：17/20，与现有 ActivityPlugin、Event Inbox、grant_order/outbox 非直写约束一致；具体 API 形态仍允许实现期按现有边界落地。

## Decision

```text
PASS
```

需求：2026-05-29-activityplugin-scheduler-events
