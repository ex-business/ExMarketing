# Analysis

## 需求编号

2026-05-24-113929-5

## Review 反馈处理

- P1：Review 指出上一版只定义了 Outbox `PENDING/FAILED/SENT/DLQ/DLQ_CLOSED` 的重试和 DLQ 流转，没有明确 `marketing_grant_order` 的 `RETRY/CLOSED` 如何随投递失败、DLQ、人工 close 同步，可能导致 outbox 已满足验收但 grant_order 长期停在 `PENDING/SENDING`。本轮已补充“Outbox 与发奖单状态同步规则”：首次投递领取时 grant_order 进入 `SENDING`；可重试投递失败时进入 `RETRY`；到期重试再次领取时回到 `SENDING`；投递成功 `SENT` 后 grant_order 保持 `SENDING` 等待 callback；不可重试或超过最大重试次数进入 Outbox `DLQ` 时，grant_order 保持 `RETRY` 作为人工处理态；运营 replay 后重新投递进入 `SENDING`；运营 close 后进入 `CLOSED` 并释放仍冻结的 quota。由此覆盖任务原文“可重试错误进入 RETRY，不可重试错误进入人工处理或 CLOSED”的核心验收。
- Open Question：非重试投递失败是否立即把 grant_order 置为 `CLOSED`，还是 Outbox 处于 `DLQ` 时保留非终态等待运营选择？默认最小可验收口径选择后者：Outbox `DLQ` 表示投递层进入人工处理队列，grant_order 同步为 `RETRY` 并保留冻结 quota，允许运营修复配置或目标后 replay；只有运营明确执行 DLQ close、人工最终关闭或超时释放时，grant_order 才进入 `CLOSED` 或既有 `FAILED` 终态并释放 quota。依据是 docs/api 已定义 `DLQ` 可 replay、`DLQ_CLOSED` 不允许 replay；如果进入 DLQ 时立即关闭 grant_order，会破坏 replay 的恢复语义并提高重复开单风险。
- 扣分项：上一轮已修正 Outbox `FAILED` 自动领取冲突、callback `FAILED` 与 DLQ replay 层级边界；本轮新增 grant_order `RETRY/CLOSED` 同步规则、边界条件、异常场景和验收标准，关闭 Score 76/100 的 BLOCK 根因。
- Decision：本轮基于上述默认口径重新评分；不存在需等待 Owner 决策的旧 Review BLOCK。

## Analysis 自检修正

- 更早一轮 `analysis.md` 只有模板，`Score = 0`，`Decision = PASS / BLOCK` 未决，属于未完成分析；上一轮虽给出 `PASS`，但 Review 发现 grant_order 状态同步规则仍不足。
- 修正方式：保留已补齐的背景目标、参与方、输入输出、业务规则、边界条件、异常场景、非目标范围、风险、验收标准和待确认问题，并进一步补充 grant_order `RETRY/CLOSED` 与 Outbox retry/DLQ/replay/close 的同步口径。
- 旧 BLOCK 关闭口径：现有文档已明确 `marketing_grant_order` 状态包含 `PENDING/SENDING/SUCCESS/FAILED/RETRY/CLOSED`，`marketing_outbox` 状态包含 `PENDING/SENDING/SENT/FAILED/DLQ`，README/API/运维文档已说明 retry、exponential backoff、DLQ、replay、close、callback 和 quota 释放语义。本轮基于这些材料细化，不引入代码实现假设；无法从材料唯一确定的参数和状态边界均采用默认最小可验收口径并写明依据。

## 资料依据

- `tasks/2026-05-24-113929-5/task.md`
- `README.md`
- `CLAUDE.md`
- `docs/architecture.md`
- `docs/data-model.md`
- `docs/api.md`
- `docs/operations.md`
- `/Users/tong/Documents/GitHub/OracleBetX-Projects/act-analysis/22-独立活动营销中台设计.md`

## 背景目标

ExMarketing 是独立活动营销中台，只负责活动配置、规则判断、奖励计算、预算库存、参与记录、发奖编排、幂等、审计和对账，不直接修改外部业务系统余额或权益。

当前活动命中后会生成 participation、grant_order 和 outbox。Outbox 负责把标准发奖指令投递到 HTTP / WEBHOOK / MQ 目标，外部发奖系统再通过 callback 返回真实发奖结果。需求目标是在这条链路中明确失败重试策略，避免“业务记录已生成但发奖指令丢失”、重复投递、无限重试、错误信息不可追踪、不可恢复失败无法人工处理等问题。

## 目标理解

本需求要求细化两类失败：

1. Outbox 投递失败：发奖指令尚未成功送达 HTTP / WEBHOOK / MQ 目标，应按可重试错误进入自动重试，记录重试次数、退避间隔、下一次执行时间和错误信息；超过最大次数或不可重试时进入 DLQ，等待人工 replay 或 close。
2. 发奖结果失败：发奖指令已送达或外部系统已处理，但 callback 返回失败或人工超时释放认定失败，应更新 grant_order / participation / quota 终态。默认最小口径下，外部 callback `FAILED` 视为最终失败，不自动重复发奖，避免外部权益重复。Outbox DLQ replay 不适用于 callback `FAILED` 后的补发；如业务后续需要重新发放，应另行设计人工补发、重新开单或发奖结果申诉流程。

## 参与方

- Event 上报方：外部业务系统上报标准事件，不直接参与重试策略。
- ExMarketing Engine：在活动命中时创建 participation、grant_order、outbox，并保持幂等与 quota 冻结。
- Outbox Worker：领取待投递 outbox，执行 HTTP / WEBHOOK / MQ 投递，按错误类型更新状态和重试计划。
- 外部发奖系统：接收发奖指令，最终通过 `/openapi/v1/reward/callback` 返回 `SUCCESS` 或 `FAILED`。
- 运营 / OPS：查看 DLQ、错误信息、重放或关闭不可自动恢复的消息。
- 审计 / 对账方：按 grant_order、outbox、callback、quota reservation 检查链路一致性。

## 输入

- 已创建的 `marketing_outbox` 记录：包含 `tenant_id`、`message_type`、`target_channel`、`target_ref`、`payload_json`、`send_status`、`retry_count`、`next_retry_at`、`error_message`。
- 已创建的 `marketing_grant_order` 记录：包含 `tenant_id`、`bill_no/commandId`、`grant_status`、`grant_channel`、`retry_count`、`error_message`、`idempotency_key`。
- Worker 运行时配置：是否启用 Outbox worker、各通道 sender 配置、默认最大重试次数和退避策略。
- 外部投递结果：HTTP/Webhook 响应、网络错误、超时、MQ ack / publisher confirm / send ack。
- 发奖 callback：`tenantId`、`commandId`、`status`、`externalGrantId`、`message`、`finishedAt`。
- DLQ 人工操作：`tenantId`、`operator`、`reason`、目标 outbox ID。

## 输出

- Outbox 状态变化：`PENDING/SENDING` -> `SENT`、`FAILED`、`DLQ`；人工关闭时进入 `DLQ_CLOSED`。
- Grant order 状态变化：`PENDING/SENDING` -> `SUCCESS`、`FAILED`、`RETRY`、`CLOSED`。
- 重试调度字段：`retry_count` 累加、`next_retry_at` 写入下次执行时间、`error_message` 记录最新错误摘要。
- Quota reservation 变化：发奖 `SUCCESS` 时确认冻结额度；grant_order 最终 `FAILED/CLOSED` 或超时释放时释放冻结额度；重复终结不得二次移动 quota。
- 审计记录：DLQ replay、DLQ close、quota timeout release、关键失败上下文需要可追踪。
- API / 运维可见性：DLQ 查询、replay、close、对账查询可看到状态、次数、错误和时间线。

## 默认最小可验收口径

在现有资料没有给出唯一参数值时，本轮采用以下默认值，作为进入研发和验收的最小约束：

- 默认最大自动重试次数：`5` 次。依据：任务明确要求“最大重试次数”，README 已要求超过次数进入 DLQ；5 次能覆盖短暂网络抖动，又避免长时间无限占用队列。
- 默认退避策略：指数退避，初始间隔 `1 分钟`，倍数 `2`，最大单次间隔 `30 分钟`。下一次执行时间按当前处理时间加退避间隔计算。
- `retry_count` 语义：只统计失败后安排的自动重试次数；首次投递失败并进入重试后为 `1`。当 `retry_count >= maxRetry` 且再次失败时进入 DLQ，不再设置新的自动执行时间。
- Outbox 待自动投递状态：`PENDING` 表示首次待投递或 DLQ replay 后待投递；`FAILED` 表示投递层发生可重试失败且已设置 `next_retry_at` 的待重试状态。两者都是 Worker 可自动领取的候选状态，但必须分别满足时间条件。
- Grant order 投递层状态映射：`PENDING` 表示已创建但尚未被 Outbox worker 领取；`SENDING` 表示发奖指令正在投递或已投递成功但尚未收到 callback；`RETRY` 表示投递层发生可重试失败或已进入 DLQ 人工处理队列但尚未人工关闭；`CLOSED` 表示投递层经人工确认不再 replay 的关闭态。`SUCCESS/FAILED` 只由 callback 或超时释放等发奖结果终结流程写入，不由普通 Outbox 投递成功直接写入。
- 不可重试投递失败默认边界：Outbox 进入 `DLQ`，grant_order 进入 `RETRY` 并保留冻结 quota，等待运营 replay 或 close；不在进入 `DLQ` 的瞬间自动置为 `CLOSED`。只有运营执行 DLQ close、人工最终关闭或超时释放时，grant_order 才进入 `CLOSED` 或既有 `FAILED` 终态，并释放仍冻结的 quota。
- 错误信息：记录最新失败的稳定分类、通道、目标引用摘要、外部状态码或错误码、短错误消息；不得记录 token、Authorization、broker 凭据或完整 secret。
- 可重试错误默认分类：网络连接失败、DNS 临时失败、连接超时、读写超时、HTTP `429`、HTTP `5xx`、MQ broker 临时不可用、MQ ack/confirm/send 超时、sender 临时不可用。
- 不可重试错误默认分类：HTTP `400/401/403/404/409/422`、payload schema 错误、目标地址配置非法、租户或 commandId 不匹配、外部明确业务拒绝、callback 冲突、已关闭记录、超过最大重试次数。
- Outbox `CLOSED` 术语映射：现有 API 使用 `DLQ_CLOSED` 表示人工关闭的 Outbox；任务中的 “CLOSED” 对 Outbox 侧按 `DLQ_CLOSED` 验收，对 grant_order 侧按 `CLOSED` 验收。

## 业务规则

### Outbox 投递规则

- Worker 自动领取条件必须覆盖两类候选：
  - `send_status=PENDING` 且 `next_retry_at` 为空或不晚于当前时间；
  - `send_status=FAILED` 且 `next_retry_at` 不为空且不晚于当前时间。
- `FAILED` 在 Outbox 侧不是最终失败，而是“可重试失败、等待下次自动投递”的状态；只有进入 `DLQ` 或 `DLQ_CLOSED` 后才退出自动投递链路。
- `send_status=FAILED` 但 `next_retry_at` 为空属于异常脏数据，不得被无限循环领取；应进入运维可见的人工排查范围，或按不可重试异常进入 DLQ 并记录错误原因。
- Worker 领取 outbox 后进入 `SENDING`，投递期间其他 worker 不应重复处理同一条消息。
- HTTP / WEBHOOK 投递收到成功响应后，outbox 进入 `SENT`；MQ 投递收到 Kafka ack、RabbitMQ publisher confirm 或 Pulsar send ack 后，outbox 进入 `SENT`。
- MQ 的 `SENT` 只表示发奖指令到达 broker，不表示真实权益已发放；真实发奖成功仍以 callback 为准。
- 可重试失败进入 `FAILED`，并写入 `retry_count`、`next_retry_at`、`error_message`；到达 `next_retry_at` 后按上述领取条件重新具备自动投递资格。
- 到达 `next_retry_at` 后，消息重新具备处理资格。
- 不可重试失败或超过最大自动重试次数后进入 `DLQ`，不再被自动 worker 投递。
- `DLQ` 记录可由运营 replay，成功后同一 outbox 回到 `PENDING`，payload 不变；可由运营 close，成功后进入 `DLQ_CLOSED`。
- `DLQ_CLOSED` 不允许 replay，除非后续另有明确需求新增解封流程。

### Outbox 与发奖单状态同步规则

- 创建 participation、grant_order 和 outbox 时，grant_order 初始状态为 `PENDING`，outbox 初始状态为 `PENDING`；两者通过同一发奖指令号 / commandId 可对账。
- Worker 成功领取 outbox 并进入 `SENDING` 时，关联 grant_order 必须同步进入 `SENDING`，表示发奖指令正在对外投递。
- 可重试投递失败时，outbox 进入 `FAILED` 并写入 `retry_count/next_retry_at/error_message`；关联 grant_order 必须同步进入 `RETRY`，写入同一错误摘要或可追踪错误引用。此时 quota reservation 保持 `FROZEN`，participation 不进入失败终态。
- `send_status=FAILED` 到达 `next_retry_at` 后再次被 Worker 领取时，outbox 进入 `SENDING`，关联 grant_order 从 `RETRY` 回到 `SENDING`；再次失败则按错误分类重新进入 `RETRY` 或 DLQ 人工处理。
- HTTP/Webhook 成功响应或 MQ ack/confirm/send ack 后，outbox 进入 `SENT`；关联 grant_order 保持 `SENDING`，等待外部 callback `SUCCESS/FAILED` 终结。Outbox `SENT` 不得直接把 grant_order 置为 `SUCCESS`。
- 不可重试投递失败或超过最大自动重试次数时，outbox 进入 `DLQ`；关联 grant_order 保持或更新为 `RETRY`，该 `RETRY` 表示“自动重试已停止，等待人工处理”，不得释放 quota，也不得把 participation 标记为失败。
- 运营 replay 只允许 outbox 当前为 `DLQ` 且关联 grant_order 当前为 `RETRY`；replay 成功后同一 outbox 回到 `PENDING`，payload 不变，关联 grant_order 保持 `RETRY` 直到 Worker 领取后进入 `SENDING`。
- 运营 close 只允许 outbox 当前为 `DLQ`；close 成功后 outbox 进入 `DLQ_CLOSED`，关联 grant_order 进入 `CLOSED`，释放仍为 `FROZEN` 的 quota reservation，并将 participation 更新为 `FAILED`，审计中记录 operator 和 reason。
- 如果 outbox 已 `SENT` 且后续 callback `SUCCESS`，grant_order 进入 `SUCCESS`，quota 从 `FROZEN` 确认为 `CONFIRMED`；如果 callback `FAILED`，grant_order 进入 `FAILED`，quota 从 `FROZEN` 释放为 `RELEASED`。这一路径不经过 Outbox DLQ replay。
- 如果 outbox 已 `SENT` 但长期没有 callback，超时释放入口可按既有规则将 grant_order 置为 `FAILED` 并释放 quota；本需求不新增自动把 `SENT` 回退到 `FAILED/PENDING` 的投递重试。
- 如果 grant_order 已处于 `SUCCESS/FAILED/CLOSED` 任一终态，Outbox worker、DLQ replay、DLQ close 或 callback 不得覆盖该终态；应返回重复或冲突结果，并保留审计/错误信息。

### 发奖单状态规则

- grant_order 创建后默认为待发奖状态，发奖指令通过 outbox 投递。
- grant_order `RETRY` 是投递层待重试或人工处理态，不表示外部发奖结果失败；在 `RETRY` 期间 quota 保持冻结，等待自动重试、DLQ replay 或人工 close。
- grant_order `CLOSED` 是人工确认不再继续投递/补发的关闭态；进入 `CLOSED` 时必须释放仍冻结的 quota，并阻止后续 Outbox replay 或 callback 覆盖。
- Outbox 投递过程中的临时失败不代表发奖最终失败，不应直接释放 quota。
- 外部 callback `SUCCESS` 是发奖成功终态，必须按 `tenantId + commandId` 定位 grant_order，确认 quota reservation，并更新 participation。
- 外部 callback `FAILED` 默认视为发奖最终失败：释放仍冻结的 quota reservation，更新 grant_order / participation 为失败终态，并记录错误信息。
- 相同最终态重复 callback 返回 duplicate，不得重复确认或释放 quota。
- 与现有终态冲突的 callback 返回机器可识别错误，不得覆盖已成功、已失败或已关闭状态。
- 超时释放入口认定的长期未终结发奖单进入 `FAILED`，错误信息包含 `TIMEOUT_RELEASE`，并释放对应冻结 quota。
- 自动重试只覆盖“指令投递失败”这一层；外部已处理后的发奖失败，默认不自动重复发奖，以降低重复发放风险。
- callback `FAILED` 后的人工补发、重新开单、改判成功或外部结果申诉不属于 Outbox DLQ replay，本轮不定义自动状态回退。

### 错误分类与人工处理

- 可重试错误必须进入自动重试链路，不能直接关闭。
- 不可重试投递错误必须进入 Outbox `DLQ` 和 grant_order `RETRY` 人工处理态，不能无限重试；只有运营 close、人工最终关闭、callback `FAILED` 或超时释放等终结动作才进入 grant_order `CLOSED/FAILED`。
- Outbox delivery DLQ replay 适用于目标恢复、临时配置修复、外部通道恢复等“投递层未成功完成”的场景；它不用于重放已经 `SENT` 且 callback 返回 `FAILED` 的发奖结果。
- DLQ close 适用于 payload 永久非法、业务拒绝、目标不存在、租户配置错误且不准备补发等场景。
- 人工操作必须要求 `operator` 和 `reason`，并写入审计。

## 边界条件

- 重复事件不应创建重复 participation、grant_order 或 outbox。
- 同一 participation 的同一 rewardCode 只能生成一张 grant_order。
- Outbox payload replay 时不得修改 payload，避免人工重放绕过原始发奖幂等语义。
- `targetRef` 中不得包含 broker 地址、账号、密码、token、query 或 fragment；错误日志也不得泄露凭据。
- 多 worker 并发时，同一 outbox 在同一时刻只能有一个 worker 投递。
- 系统时间应使用统一时区语义保存 `next_retry_at`，验收按 RFC3339/数据库时间可比较为准。
- `error_message` 应保留最新错误；历史完整时间线依赖审计或日志，不要求在本需求中新增多条错误明细表。
- 未配置 quota 的不限额 grant_order 可以正常 callback 终结，不要求 quota reservation。
- 回调与人工超时释放并发时，只允许一个终结操作成功移动 quota，另一个必须识别为重复或冲突。
- Outbox 进入 `SENT` 后，不再因投递层重试策略自动回退为待发送；后续发奖结果由 callback / 对账处理。
- callback `FAILED` 与 Outbox `DLQ` 是不同层级的失败：前者表示外部发奖结果终态，后者表示发奖指令投递层终态。两者不得互相隐式转换。
- Outbox `DLQ` 期间 grant_order 必须处于 `RETRY` 人工处理态，且 quota 保持冻结；若业务不准备继续投递，必须通过 DLQ close 或超时释放等显式终结动作释放冻结 quota。
- DLQ replay 不得作用于 grant_order 已为 `SUCCESS/FAILED/CLOSED` 的记录，避免已终结发奖单再次投递。
- grant_order `CLOSED` 后收到 callback，默认按 `GRANT_ORDER_CLOSED` 冲突处理，不得重新确认或释放 quota。

## 异常场景

- HTTP/Webhook 网络超时：outbox 进入 `FAILED`，grant_order 进入 `RETRY`，设置下一次执行时间，quota 保持冻结。
- `FAILED` 待重试消息到达 `next_retry_at`：重新进入 Worker 自动领取范围并再次投递，grant_order 从 `RETRY` 回到 `SENDING`。
- HTTP `5xx` 或 `429`：进入自动重试。
- HTTP `4xx`：默认 outbox 进入 `DLQ`，grant_order 进入 `RETRY` 人工处理态，等待人工修复 replay 或 close。
- MQ sender 未配置或 broker 临时不可用：如果属于运行时临时不可用，进入自动重试，grant_order 进入 `RETRY`；如果目标 URI 永久非法，outbox 进入 `DLQ`，grant_order 进入 `RETRY` 人工处理态。
- Worker 投递中进程退出：未完成终态的 outbox 后续应能重新进入可处理范围，不能永久卡死在 `SENDING`。
- 超过最大重试次数：outbox 进入 `DLQ`，grant_order 保持或更新为 `RETRY`，保留最后错误信息。
- 运营 replay DLQ：同一 outbox 回到 `PENDING`，grant_order 保持 `RETRY`，Worker 领取后进入 `SENDING`。
- 运营 close DLQ：outbox 进入 `DLQ_CLOSED`，grant_order 进入 `CLOSED`，释放仍冻结 quota，后续 replay 和 callback 覆盖均被拒绝。
- 外部 callback `SUCCESS` 缺少 `externalGrantId`：拒绝并返回错误，不更新 grant_order。
- callback 找不到 `tenantId + commandId`：返回 `GRANT_ORDER_NOT_FOUND`。
- callback 与既有终态冲突：返回冲突错误，不移动 quota。
- DLQ replay 遇到同一条消息再次失败：重新按 retry 规则计算次数和下一次执行时间，最终仍可再次进入 DLQ。
- callback `FAILED` 后业务方希望再次给用户发奖：本轮不通过 DLQ replay 处理；需走后续独立人工补发或重新开单需求。

## 非目标范围

- 不新增新的发奖通道类型；仍限定 HTTP / WEBHOOK / MQ。
- 不设计外部发奖系统的内部重试、余额变更或权益发放逻辑。
- 不改变标准事件模型、活动配置模型、reward payload 基本结构。
- 不引入人工补发新单或修改 outbox payload 的能力。
- 不支持 callback `FAILED` 后自动补发、自动重新打开 grant_order、自动从 `SENT` 回退到 `PENDING/FAILED` 或通过 DLQ replay 重发已送达的发奖指令。
- 不要求在 outbox 进入 `DLQ` 的瞬间自动关闭 grant_order；本轮默认保留 `RETRY` 人工处理态以支持 replay，关闭必须来自 DLQ close、人工最终关闭或超时释放等显式动作。
- 不要求本需求新增独立错误历史表；保留最新错误和审计即可满足最小验收。
- 不要求为不同租户、活动或通道提供后台可配置重试策略；本轮按全局默认策略验收。
- 不要求实现生产告警系统，但运维查询必须能识别 DLQ、重试次数、下一次执行时间和错误。
- 不改变 callback 只接受 `SUCCESS/FAILED` 的现有协议。

## 风险

- 重复发奖风险：Outbox retry 和 DLQ replay 可能导致外部系统多次收到同一 command。缓解要求是 commandId / bill_no 稳定，payload 不变，外部发奖系统按 commandId 幂等。
- 待重试消息漏投风险：如果 Worker 只领取 `PENDING` 而不领取到期 `FAILED`，可重试失败会永久停留。缓解要求是验收明确覆盖 `FAILED + next_retry_at <= now` 的自动领取。
- grant_order 状态漂移风险：如果只更新 Outbox 状态而不更新 grant_order，发奖单可能长期停在 `PENDING/SENDING`，对账无法识别 `RETRY/CLOSED`。缓解要求是每个 Outbox retry/DLQ/replay/close/callback 场景都有明确的 grant_order 同步验收。
- 层级混淆风险：callback `FAILED` 被误当作 Outbox DLQ 可重放，可能导致外部已处理失败的权益被重复发起。缓解要求是文档、验收和运维查询区分 Outbox delivery DLQ 与发奖结果失败。
- 预算冻结漂移风险：投递失败不能释放 quota，最终失败或超时释放必须释放 quota；成功 callback 必须确认 quota。验收必须覆盖重复 callback 和并发终结。
- 无限重试风险：必须有最大次数和 DLQ 终态。
- 错误分类误判风险：HTTP 4xx 中部分错误可能临时可恢复，但默认按不可重试进入人工处理，优先保护资金和权益安全。
- 可观测性不足风险：如果错误信息过短，OPS 无法处理；如果过长或包含敏感信息，会泄露凭据。验收要求错误摘要可定位且脱敏。
- 多 worker 并发风险：同一消息被重复领取会造成重复投递。验收必须覆盖并发 claim。

## 验收标准

- 可重试 Outbox 错误会更新为待重试状态，`retry_count` 增加，`next_retry_at` 晚于当前时间，`error_message` 有脱敏错误摘要。
- 可重试 Outbox 错误必须同时把关联 grant_order 更新为 `RETRY`，不得停留在 `PENDING/SENDING`，且 quota reservation 保持 `FROZEN`。
- 到达 `next_retry_at` 后，`send_status=FAILED` 的同一 outbox 能被 worker 再次领取并投递；验收必须证明 Worker 领取条件不是只包含 `PENDING`。
- 到达 `next_retry_at` 后，待重试 outbox 被 Worker 领取时，关联 grant_order 从 `RETRY` 回到 `SENDING`；再次失败时重新按规则进入 `RETRY` 或 DLQ 人工处理。
- `send_status=PENDING` 且 `next_retry_at` 为空或到期的 outbox 能被 worker 领取；`send_status=FAILED` 但 `next_retry_at` 未到期的 outbox 不能提前领取。
- 自动重试达到最大次数后仍失败时，outbox 进入 `DLQ`，不再被自动投递。
- 自动重试达到最大次数后仍失败时，关联 grant_order 必须进入或保持 `RETRY` 人工处理态，不得自动 `SUCCESS/FAILED/CLOSED`。
- 不可重试 Outbox 错误直接进入 `DLQ`，不继续自动重试；关联 grant_order 必须进入或保持 `RETRY` 人工处理态，等待运营 replay 或 close。
- DLQ 查询能返回 tenant 下的 `DLQ`，`includeClosed=true` 时包含 `DLQ_CLOSED`。
- DLQ replay 只允许 outbox 为 `DLQ` 且 grant_order 为 `RETRY` 的记录；成功后同一 outbox 回到 `PENDING`，payload 不变，grant_order 保持 `RETRY` 等待 Worker 领取，并写审计。
- DLQ close 只允许 outbox 为 `DLQ` 的记录；成功后 outbox 进入 `DLQ_CLOSED`，grant_order 进入 `CLOSED`，释放仍冻结 quota，`DLQ_CLOSED` 不允许 replay，并写审计。
- HTTP/Webhook 成功响应和 MQ ack/confirm/send ack 后，outbox 进入 `SENT`，关联 grant_order 进入或保持 `SENDING` 等待 callback，不得直接写 `SUCCESS`。
- MQ outbox `SENT` 不被误判为发奖成功；真实发奖成功必须等待 callback。
- callback `SUCCESS` 能按 `tenantId + commandId` 更新 grant_order 成功终态，并确认 quota reservation。
- callback `FAILED` 能按 `tenantId + commandId` 更新 grant_order 失败终态，并释放 quota reservation。
- callback `FAILED` 后 outbox 不得因 DLQ replay 自动回退；已 `SENT` 的 outbox 仍保持投递层终态，后续补发不属于本轮验收范围。
- grant_order 已为 `SUCCESS/FAILED/CLOSED` 时，Outbox worker、DLQ replay/close 和 callback 不得覆盖终态；应返回 duplicate 或机器可识别冲突。
- 重复相同终态 callback 不重复移动 quota。
- 相反终态 callback、已关闭发奖单 callback、缺少 reservation 或 quota 聚合冲突时返回机器可识别错误，不覆盖终态。
- 多 worker 并发处理时，同一 outbox 不会被同时投递两次。
- 日志、错误信息、审计 payload 不包含 Authorization、token、broker 凭据或完整 secret。
- PostgreSQL 模式下通过 Outbox retry、DLQ replay/close、callback 成功/失败、quota 释放/确认的集成验证；内存仓储只能作为补充验证。

## 待确认问题

- 是否需要后续支持按 tenant / channel / activity 配置不同 `maxRetry` 和退避参数？本轮默认不做，仅保留全局默认。
- 外部 callback 是否未来需要扩展 `retryable=true/false` 或错误码分类？本轮不扩展协议，`FAILED` 默认最终失败。
- `SENDING` 卡死的超时阈值是否需要运营可配置？本轮只要求不能永久卡死，具体阈值可用系统默认值。

这些问题不阻塞本轮研发；以上默认最小口径已覆盖任务验收所需的重试次数、退避间隔、下一次执行时间、错误信息、可重试与不可重试失败流转，并明确关闭 Review 提出的 grant_order `RETRY/CLOSED` 同步边界问题。

## 评分

```text
需求清晰度        19/20
范围可控          18/20
风险可控          18/20
验收标准明确      20/20
架构一致性        19/20
总分              94/100
```

扣分说明：

- 仍有少量策略参数未来可能需要产品化配置，本轮以全局默认值关闭阻塞。
- callback 失败是否可自动补发缺少外部协议支持，本轮按资金安全优先默认不自动补发。
- Review 指出的 grant_order `RETRY/CLOSED` 同步缺口已关闭，但仍需实现和测试严格保持 Outbox 投递层、grant_order 投递态和 callback 结果终态的边界。

## Decision

```text
PASS
```
