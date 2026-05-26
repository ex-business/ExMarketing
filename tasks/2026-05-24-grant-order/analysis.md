# Analysis

## 资料范围

本轮只做需求分析和需求细化，未读取实现代码，未设计具体代码改法，未修改业务代码，未提交。

已读取：

- `tasks/2026-05-24-grant-order/review.md`
- `tasks/2026-05-24-grant-order/analysis.md`
- `tasks/2026-05-24-grant-order/task.md`
- `docs/architecture.md`
- `docs/data-model.md`
- `docs/api.md`
- `docs/operations.md`
- `tasks/2026-05-24-outbox-worker-dlq/analysis.md`
- `/Users/tong/Documents/GitHub/OracleBetX-Projects/act-analysis/22-独立活动营销中台设计.md`

未发现独立 `PRD/` 目录。本分析以任务说明、上游设计文档、现有 API / 数据模型 / 运维文档、Outbox DLQ 需求分析和 Review 打回意见为依据。

## Review 反馈处理

- P1：上一版把 `RETRY` 定义为“投递失败后仍未获得外部最终结果”，但又禁止 `RETRY -> SUCCESS/FAILED` callback，导致发送超时、网络失败、响应丢失后，外部系统已经按同一 `bill_no` 发奖并回调时，本地会拒绝真实终态，形成 grant order 和 quota reservation 长期悬挂。
  - 本轮修正：`RETRY` 明确定义为“本地未获得外部最终结果”，不等于外部未收到命令。只要 grant order 仍未落终态，且 quota reservation 仍为 `FROZEN`，callback 命中 `RETRY` 时允许直接原子落 `SUCCESS` 或 `FAILED`。
  - `RETRY -> SUCCESS/FAILED` 只允许由外部 callback 驱动，不允许由 outbox worker 根据 HTTP 投递失败自行判断外部业务终态。
  - 当外部系统对重复 `commandId` 只返回幂等结果、不保证再次 callback 时，本系统仍能接受第一次发送后的迟到 callback，关闭“外部已成功、本地仍 RETRY/DLQ、quota 仍 FROZEN”的风险。
- P2：上一版没有定义 `SENDING` 悬挂恢复，只说不作为硬性范围。worker 在 `PENDING/RETRY -> SENDING` 后崩溃或进程被杀时，可能长期卡在 `SENDING`。
  - 本轮修正：给出默认最小可验收口径。由 outbox worker 或同等后台恢复任务扫描超时 `SENDING` 发奖单，默认阈值 `15 分钟`；超过阈值且未收到终态 callback 时，恢复为 `RETRY`，递增 `retry_count`，记录错误摘要，quota reservation 保持 `FROZEN`。
  - 恢复前后均接受合法 callback：callback 命中 `SENDING` 或 `RETRY` 都可在 reservation 仍 `FROZEN` 且终态未落定时进入 `SUCCESS/FAILED`；恢复任务不得覆盖已经 `SUCCESS/FAILED/CLOSED` 的 grant order。
- Open Question：`RETRY` 是否表示“外部未收到命令”还是“本地没有拿到投递成功确认”？
  - 已关闭。`RETRY` 表示本地没有拿到外部最终结果，可能包含外部已经收到并处理但本地未收到 HTTP 响应或 callback 的场景。
- Open Question：外部系统重复 `commandId` 幂等返回时是否保证重新发送 callback？
  - 默认最小口径：不假设外部系统会重新发送 callback。因此本系统必须接受第一次发送后的迟到 callback，包含 grant order 已从 `SENDING` 恢复为 `RETRY` 或 outbox 已进入 `DLQ` 的场景。

## Analysis 自检修正

- 上一版错误假设：认为 callback 只能从 `SENDING` 落终态，`RETRY` 必须先经过重新投递回到 `SENDING`。这忽略了 HTTP/Webhook 场景中“外部已收到并处理，本地投递结果未知”的可靠投递语义。
  - 已修正为：`SENDING` 和 `RETRY` 都可接受首次合法 callback 进入 `SUCCESS/FAILED`；`PENDING` 仍不接受 callback 落终态。
- 上一版扣分项：`SENDING` 悬挂恢复阈值和动作未定义。
  - 已修正为默认阈值 `15 分钟`，恢复动作为 `SENDING -> RETRY`、递增 `retry_count`、保留 `FROZEN` reservation、继续接受 callback。
- 上一版待确认：外部奖励系统业务失败 callback 后是否允许人工复核重发。
  - 已关闭为非目标。业务失败 callback 进入 `FAILED` 并释放 quota；同单重发、重新冻结或补偿单另立需求。
- 上一版待确认：独立 grant order 人工关闭 Admin API 是否必需。
  - 已关闭为默认最小口径。本需求只要求 outbox DLQ close 能联动 grant order `CLOSED`；独立 grant order 管理入口另立需求。
- 上一版 Score `90` 与 Review 新发现的 P1/P2 不匹配。本轮按可靠投递闭环后的口径重新评分。

## 背景目标

ExMarketing 是独立活动营销中台，只接收标准事件、匹配活动、执行规则、计算奖励、处理 quota、生成 participation、grant order 和 outbox。系统不直接修改用户余额，也不连接外部业务数据库；真实发奖由外部奖励系统按 Reward Command 执行，并通过 callback 回传最终结果。

本需求补齐 `marketing_grant_order` 的状态机，覆盖 `PENDING`、`SENDING`、`SUCCESS`、`FAILED`、`RETRY`、`CLOSED`，并明确 `bill_no`、`idempotency_key`、`retry_count`、callback、Outbox DLQ replay / close、`SENDING` 悬挂恢复与 quota reservation 的关系。

核心目标：

- 同一 participation 的同一 reward 只能生成一张 grant order，重复创建不能重复发奖。
- 同一 grant order 的首次投递、自动 retry、悬挂恢复后 retry、DLQ replay 必须复用同一 `bill_no` / `commandId`。
- 投递失败不等于外部业务失败；外部最终成功 / 失败以 callback 为准。
- `SENDING` 和 `RETRY` 均可接受迟到 callback 落终态，避免 quota 永久冻结。
- 人工 close 能结束不再处理的悬挂单，并释放冻结 quota。

## 参与方

- 外部事件上报方：提交标准事件和 `Idempotency-Key`。
- ExMarketing 活动引擎：匹配活动、执行规则、计算奖励、预留 quota、创建 participation、grant order 和 outbox。
- Outbox Worker：claim outbox、推进 grant order 到 `SENDING`、投递发奖指令、记录 retry / DLQ，并按默认阈值恢复悬挂 `SENDING`。
- 外部奖励系统：按 `commandId` / `bill_no` 幂等处理发奖，并通过 callback 返回 `SUCCESS` 或 `FAILED`。
- 运维 / 人工处理方：对 outbox `DLQ` 做 replay 或 close；本需求只定义状态联动和业务语义，不扩展完整工单后台。
- 对账 / 审计使用方：按活动、用户、事件、发奖单和 outbox 查询链路状态。

## 输入

- 标准事件字段：`tenantId`、`eventId`、`eventType`、`sourceSystem`、`subject`、`biz`、`occurredAt`、`attributes`。
- 外部事件幂等键：`tenantId:sourceSystem:eventType:bizType:bizId`。
- 活动配置：活动、规则、奖励、发奖模式、发奖通道、`rewardConfig.targetRef`。
- 奖励计算结果：`rewardCode`、`rewardType`、`calculateType`、金额或非金额权益 payload、币种、流水倍数、解释字段。
- quota reservation 结果：预算、库存、用户次数、每日次数的冻结记录。
- Outbox 投递结果：发送前 claim、发送成功、可重试失败、不可重试投递失败、重试耗尽、DLQ replay、DLQ close、`SENDING` 悬挂恢复。
- Reward callback：`tenantId`、`commandId`、`status`、`externalGrantId`、`message`、`finishedAt`。

## 输出

- `marketing_grant_order` 记录：
  - `bill_no`：发奖指令号，也是 callback 的 `commandId`。
  - `grant_status`：`PENDING` / `SENDING` / `SUCCESS` / `FAILED` / `RETRY` / `CLOSED`。
  - `retry_count`：grant order 维度的投递失败或悬挂恢复次数，随自动重试、恢复、replay 后失败累计。
  - `idempotency_key`：`participation.idempotency_key + ":" + rewardCode`。
  - `reward_payload_json`：发奖和对账所需解释字段。
  - `error_message`：最近一次投递失败、悬挂恢复、callback 失败或人工关闭原因摘要。
- `marketing_outbox` 发奖指令：与 grant order 指向同一 `bill_no` / `commandId`，负责可靠投递、retry、DLQ、replay、close。
- 对外发奖命令 payload：至少包含 `commandId`、`tenantId`、`activityId`、`participationId`、`subject`、`reward`、`reason`、`idempotencyKey`。
- callback 处理结果：成功、重复成功 / 失败、状态冲突、找不到发奖单、已关闭不可更新、quota reservation 冲突等机器可识别结果。
- 审计 / 对账信息：DLQ replay / close、`SENDING` 悬挂恢复、callback 成功 / 失败、状态冲突和 quota reservation 变更应可被排查。

## 业务规则

### 发奖单创建

- 只有规则通过且至少存在一个实际可发奖励时才创建 grant order。
- 必须先创建 participation，再创建 grant order，禁止绕过 participation 和 grant order 直接发奖。
- `REWARDABLE` participation、quota reservation、grant order、outbox 必须在同一个 activity 级事务中完成。
- quota 不足时整笔 activity 级事务失败，不保留 `REWARDABLE` participation、grant order 或 outbox 脏数据。
- 金额类 `BONUS` / `CASH` 金额为 `0.0000` 且无其他实际权益时，不创建 grant order / outbox，不冻结或扣减 quota。
- 非金额类奖励只要数量类权益为正，即使 `reward_amount=0.0000` 也创建 grant order / outbox，并按配置影响 STOCK / USER_LIMIT / DAILY_LIMIT。

### 幂等规则

- 外部事件幂等键固定为 `tenantId:sourceSystem:eventType:bizType:bizId`。
- participation 幂等键固定为 `tenantId:activityId:eventInbox.idempotency_key`。
- grant order 幂等键固定为 `participation.idempotency_key + ":" + rewardCode`。
- 同一 participation 的同一 rewardCode 重复创建时，必须返回或复用既有 grant order，不得创建第二张 grant order，不得创建第二条等价 outbox。
- 同一事件重复上报且 payload 相同，应返回 duplicate 语义，不新增 participation、grant order 或 outbox。
- 同一幂等键对应不同 payload，应拒绝为幂等冲突，不得创建或更新 grant order。
- 并发重复请求下，最终也只能有一张 grant order 和一条等价 outbox 生效，且 quota 不能重复冻结、确认或释放。

### `bill_no` / `commandId`

- `bill_no` 是 ExMarketing 发给外部奖励系统的唯一指令号，并作为 callback 的 `commandId` 定位 grant order。
- `bill_no` 必须在同一 tenant 内稳定唯一；同一 grant order 的首次投递、自动 retry、悬挂恢复后 retry 和 DLQ replay 必须使用同一个 `bill_no`。
- 重复创建命中既有 grant order 时不得重新生成 `bill_no`。
- `bill_no` 不得依赖每次发送时间或每次随机重算结果；默认最小口径为与 grant order 幂等键一一对应的稳定指令号。
- callback 更新必须用 `tenantId + commandId` 定位，禁止跨 tenant 更新。

### 状态定义

| 状态 | 含义 | 是否终态 |
|---|---|---|
| `PENDING` | grant order 已创建，等待 outbox worker 首次发送；按本地状态尚未进入外部投递窗口 | 否 |
| `SENDING` | outbox worker 已 claim 并准备发送或正在发送，同一 `bill_no` 可能已到达外部系统 | 否 |
| `RETRY` | 本地未获得外部最终结果；可能是投递失败、发送超时、响应丢失、`SENDING` 悬挂恢复，或 outbox `DLQ` 暂停等待人工 replay / close | 否 |
| `SUCCESS` | 外部奖励系统 callback 确认发放成功 | 是 |
| `FAILED` | 外部奖励系统 callback 明确确认发放失败，且本单不再用同一 grant order 自动或人工 replay | 是 |
| `CLOSED` | 人工或运维确认不再继续处理；通常来自 DLQ close 或发送前人工结案 | 是 |

重要约束：

- `FAILED` 不表示 HTTP/Webhook 投递失败、目标不可达、发送超时、响应丢失、`SENDING` 悬挂或 outbox 重试耗尽；这些属于投递层结果未知，对 grant order 默认落在 `RETRY`。
- outbox `DLQ` 不是 grant order 状态枚举；它与 grant order `RETRY` 配合表达“自动投递暂停，等待人工处理”。
- `SUCCESS`、`FAILED`、`CLOSED` 是终态。终态重复更新只能返回 duplicate 或 conflict，不得反向改写。

### 合法状态流转

默认最小可验收状态流转：

```text
CREATE -> PENDING
PENDING -> SENDING
SENDING -> SUCCESS
SENDING -> FAILED
SENDING -> RETRY
RETRY -> SENDING
RETRY -> SUCCESS
RETRY -> FAILED
RETRY -> CLOSED
PENDING -> CLOSED
```

流转说明：

- `SENDING -> SUCCESS/FAILED` 由 callback 驱动。
- `RETRY -> SUCCESS/FAILED` 只允许由 callback 驱动，适用于发送超时、响应丢失、worker 恢复为 `RETRY`、outbox 已 `DLQ` 但外部迟到 callback 到达的场景。
- `SENDING -> RETRY` 由可重试投递失败、发送结果未知或 `SENDING` 悬挂恢复驱动。
- `RETRY -> SENDING` 由自动 retry 或 DLQ replay 后重新投递驱动，必须复用同一 `bill_no`。
- `PENDING -> SUCCESS/FAILED` 禁止。callback 命中 `PENDING` 默认返回状态冲突，不自动补齐 `SENDING`。
- `FAILED -> CLOSED` 不作为本任务必需流转。外部业务失败已经是终态；如果后续需要复核或补偿，应另开补偿需求。
- `PENDING -> CLOSED` 只用于尚未发送前人工确认不再处理的异常情况，不代表外部发奖失败。

终态重复处理：

```text
SUCCESS + duplicate SUCCESS callback -> duplicate, 状态保持 SUCCESS
FAILED + duplicate FAILED callback -> duplicate, 状态保持 FAILED
CLOSED + duplicate close -> conflict 或 already_closed, 状态保持 CLOSED
```

禁止流转：

```text
SUCCESS -> any other status
FAILED -> SUCCESS
FAILED -> SENDING
FAILED -> RETRY
CLOSED -> any other status
PENDING -> SUCCESS
PENDING -> FAILED
```

### `retry_count`

- `retry_count` 初始为 `0`。
- 投递失败并进入下一轮重试或 DLQ 语义时递增。
- `SENDING` 超过默认悬挂阈值并恢复为 `RETRY` 时递增，因为本地投递尝试结果未知，需要被计入可靠投递风险。
- 成功 callback 不递增 `retry_count`。
- 失败 callback 不递增 `retry_count`，因为它表示外部业务终态，不是投递失败。
- DLQ replay 后如果再次投递失败，继续在同一 grant order 上累计 `retry_count`，不得创建新 grant order 重置计数。
- 重复投递同一 `bill_no` 必须保留同一 grant order 和累计 `retry_count`。

### 与 Outbox 的关系

- grant order 负责发奖业务状态，outbox 负责消息可靠投递状态。
- 创建 grant order 时必须创建对应 outbox；二者指向同一 `bill_no` / `commandId`。
- worker claim outbox 并准备发送前，必须先把 grant order 从 `PENDING` 或 `RETRY` 推进到 `SENDING`。
- 可重试投递失败、发送超时、响应丢失时，outbox 进入等待重试，grant order 进入 `RETRY` 并更新 `retry_count` 与 `error_message`。
- 投递不可恢复或重试耗尽时，outbox 进入 `DLQ`，grant order 保持或进入 `RETRY`，不得进入 `FAILED`。
- DLQ replay 只允许对 outbox `DLQ` 且 grant order `RETRY` 的记录执行；成功后同一 outbox 回到可投递状态，同一 grant order 后续应从 `RETRY -> SENDING`。
- DLQ close 只允许对 outbox `DLQ` 且 grant order `RETRY` 的记录执行；成功后 outbox 进入 `DLQ_CLOSED`，grant order 进入 `CLOSED`。
- `DLQ_CLOSED` 不允许 replay；grant order `CLOSED` 不允许 callback 改写为成功或失败。
- outbox 已 `DLQ` 不阻止 grant order 接受迟到 callback；如果 callback 合法落 `SUCCESS/FAILED`，后续 replay / close 必须因 grant order 已终态而失败。

### `SENDING` 悬挂恢复

默认最小可验收口径：

- `SENDING` 悬挂阈值为 `15 分钟`，以 grant order 或对应 outbox 进入 `SENDING` 的更新时间为准。
- 恢复任务由 outbox worker 或同等后台能力承担；不要求新增独立管理页面。
- 仅当 grant order 当前仍为 `SENDING`、未落终态、未关闭，且超过阈值时，才允许恢复为 `RETRY`。
- 恢复时递增 grant order `retry_count`，记录 `error_message`，quota reservation 保持 `FROZEN`，outbox 回到可重试或 DLQ 规则允许的状态。
- 恢复后仍使用同一 `bill_no` retry / replay，依赖外部奖励系统按 `commandId` 幂等防重。
- 如果恢复任务与 callback 并发，必须以当前状态和 reservation 条件为准：callback 已落 `SUCCESS/FAILED` 后，恢复任务不得覆盖终态；恢复先完成后，迟到 callback 命中 `RETRY` 仍可落终态。

### 与 callback 的关系

- callback 必须按 `tenantId + commandId` 定位 grant order，找不到返回 `GRANT_ORDER_NOT_FOUND`。
- `status` 只接受 `SUCCESS` / `FAILED`；`SUCCESS` callback 必须提供 `externalGrantId`。
- 首次 `SUCCESS` callback 命中 `SENDING` 或 `RETRY` 时，若 quota reservation 仍为 `FROZEN`，grant order 进入 `SUCCESS`，reservation 进入 `CONFIRMED`。
- 首次 `FAILED` callback 命中 `SENDING` 或 `RETRY` 时，若 quota reservation 仍为 `FROZEN`，grant order 进入 `FAILED`，reservation 进入 `RELEASED`。
- 相同最终态重复 callback 返回 duplicate，不重复确认或释放 quota，不重复更新 participation。
- 相反最终态 callback 返回冲突，不得覆盖既有终态。
- callback 命中 `PENDING` 时返回 `CALLBACK_STATUS_CONFLICT` 或等价机器可识别错误，不改写状态。
- callback 命中 `CLOSED` 时返回 `GRANT_ORDER_CLOSED` 或等价冲突，不改写状态。
- callback 命中 `FAILED` 后收到 `SUCCESS`，或命中 `SUCCESS` 后收到 `FAILED`，返回 `CALLBACK_STATUS_CONFLICT`。
- callback 命中 `RETRY` 但 reservation 已缺失、已 `RELEASED` 或状态不一致时，不得盲目落终态，应返回 quota reservation 冲突并保留可审计错误。

### 与 quota reservation 的关系

默认最小可验收口径：

- 创建 rewardable grant order 时，相关 quota reservation 进入 `FROZEN` 或等价冻结语义。
- outbox 投递失败、自动重试、发送超时、响应丢失、`SENDING` 悬挂恢复、重试耗尽进入 `DLQ`、DLQ replay 均不释放 quota reservation。
- grant order `SENDING` / `RETRY` 要求 quota reservation 仍可被确认或释放；若 reservation 已释放或缺失，retry / replay / callback 必须返回机器可识别冲突。
- callback `SUCCESS`：grant order 进入 `SUCCESS`，quota reservation 从 `FROZEN` 变为 `CONFIRMED`，quota 聚合从 frozen 转入 used 或等价确认消耗。
- callback `FAILED`：grant order 进入 `FAILED`，quota reservation 从 `FROZEN` 变为 `RELEASED`，释放 frozen quota。
- DLQ close / grant order `CLOSED`：释放仍冻结的 quota reservation，避免预算 / 库存永久冻结。
- 已 `RELEASED` 的同一 grant order 不允许通过 replay 重新冻结；重新发奖或补偿必须是后续单独需求。

## 边界条件

- 一个 participation 命中多个 enabled reward 时，每个 rewardCode 最多一张 grant order。
- 不同 rewardCode 可以各自生成 grant order，但必须共享同一个 participation 上下文。
- 任一 reward 的 quota 不足时，该 activity 本次 rewardable 链路整体失败，不生成部分 grant order / outbox。
- 不同 activity 命中同一外部事件时，按 activity 维度各自产生 participation 和 grant order 幂等键。
- 缺少 quota 行表示无限制，不影响 grant order 状态机。
- 模拟运行只输出 grant order preview，不写真实 grant order，不消耗 `bill_no`，不冻结或扣减 quota。
- MQ、MANUAL_EXPORT、MANUAL 当前不是可执行发奖范围，本需求不扩展这些通道的完整状态机。
- 内存仓储可用于本地快速验证，涉及唯一约束、事务、并发、callback、`SENDING` 恢复和 quota reservation 的验收必须以 PostgreSQL 为准。
- 外部系统已成功处理但 HTTP 响应丢失时，outbox 可能进入 retry / DLQ；后续 retry / replay 仍必须使用同一 `bill_no`，依赖外部系统幂等防重。
- callback 先于 worker 写回 outbox 成功到达时，只要 grant order 已是 `SENDING`，callback 可合法落终态；outbox 后续不得再推动 grant order 反向变化。
- callback 在 worker 将 `SENDING` 恢复为 `RETRY` 后到达时，只要 reservation 仍 `FROZEN`，callback 仍可合法落终态。

## 异常场景

- 重复创建：返回既有 grant order，不新增记录，不新增等价 outbox，不重新生成 `bill_no`。
- 幂等键冲突：同一 key 但 payload 不同，拒绝处理，不创建或更新 grant order。
- quota 不足：返回 quota exceeded 语义，回滚 participation / grant order / outbox。
- worker 发送前推进状态失败：不得发送外部发奖指令，避免已发送但本地仍为 `PENDING`。
- worker 发送超时、网络失败或响应丢失：grant order 进入 `RETRY`，累计 `retry_count`，outbox 按规则等待下次发送或进入 `DLQ`，quota reservation 保持 `FROZEN`。
- worker 重试耗尽：outbox 进入 `DLQ`，grant order 保持 `RETRY`，quota reservation 保持 `FROZEN`。
- `SENDING` 超过 15 分钟未落终态：恢复任务将 grant order 恢复为 `RETRY`，递增 `retry_count`，保留同一 `bill_no` 和 `FROZEN` reservation。
- DLQ replay 成功 claim：同一 outbox 重新进入可投递流程，同一 grant order 从 `RETRY -> SENDING` 后发送同一 `bill_no`。
- DLQ replay 时 grant order 已是 `FAILED`、`SUCCESS`、`CLOSED` 或 quota reservation 已释放：返回状态冲突，不发送。
- DLQ close：outbox 进入 `DLQ_CLOSED`，grant order 进入 `CLOSED`，释放冻结 quota，写审计。
- callback 找不到 `tenantId + commandId`：返回 `GRANT_ORDER_NOT_FOUND`。
- callback 命中 `PENDING`：返回状态冲突，不改写状态。
- callback 命中 `SENDING`：按首次终态 callback 规则进入 `SUCCESS/FAILED`。
- callback 命中 `RETRY`：按首次终态 callback 规则进入 `SUCCESS/FAILED`，用于迟到 callback 闭环。
- callback 命中 `CLOSED`：返回 `GRANT_ORDER_CLOSED` 或等价冲突，不改写状态。
- callback 终态冲突：已 `SUCCESS` 后收到 `FAILED`，或已 `FAILED` 后收到 `SUCCESS`，返回冲突。
- 外部奖励系统重复收到同一命令：必须依赖同一 `bill_no` / `idempotencyKey` 幂等处理，ExMarketing 不生成新命令号规避重复。

## 非目标范围

- 不实现外部钱包、积分、优惠券、礼品或权益系统的真实发放逻辑。
- 不直接修改用户余额。
- 不新增业务专用 controller、业务专用发奖逻辑或外部业务数据库依赖。
- 不扩展 MQ、MANUAL_EXPORT、MANUAL 的完整 sender、导出或人工发奖流程。
- 不实现完整人工审核后台、二级审批、运营工单系统或独立 grant order 管理页面。
- 不把 HTTP/Webhook 投递失败、发送超时、响应丢失、`SENDING` 悬挂或 outbox `DLQ` 定义为外部发奖业务失败。
- 不允许 `FAILED` grant order 通过本任务 replay 重新发奖；外部业务失败后的复核重发、重新冻结或补偿单另立需求。
- 不实现 `CLOSED` reopen。
- 不要求外部奖励系统必须在重复 `commandId` 幂等返回时重新发送 callback；本系统通过接受迟到 callback 兜底。
- 不用代码实现细节替代上述业务状态定义。

## 风险

- 重复发奖风险高：网络超时、响应丢失、worker 崩溃恢复、DLQ replay 都可能使同一 `bill_no` 多次到达外部系统；必须依赖外部奖励系统按 `commandId` 幂等。
- 资金 / 权益冻结风险高：投递失败后如果错误释放 quota，replay 或迟到成功 callback 会无 reservation 可确认；如果人工 close 不释放 quota，会造成预算 / 库存永久冻结。
- 迟到 callback 风险高：如果 `RETRY` 拒绝 callback，会形成外部已成功、本地仍悬挂的不一致；本轮已通过允许 `RETRY -> SUCCESS/FAILED` callback 降低风险。
- 悬挂恢复风险中高：`SENDING` 恢复过早会增加重复投递概率，恢复过晚会延长冻结时间；默认 15 分钟是最小可验收口径，后续可配置化。
- 并发风险高：重复事件、重复 grant order 创建、多 worker claim、SENDING 恢复 / callback 并发、DLQ replay / close 并发都必须以 tenant、状态和 reservation 条件判定。
- 审计风险中：DLQ close / replay、SENDING 恢复、callback 终态、quota reservation 释放 / 确认必须可排查，否则无法解释发奖单长期悬挂或预算漂移。
- 验收风险中：只用内存仓储无法覆盖 PostgreSQL 唯一约束、事务、行锁、reservation 和并发语义。

## 验收标准

- 创建奖励型活动后，上报符合规则的标准事件，会生成 participation、grant order 和 outbox。
- 新建 grant order 初始状态为 `PENDING`，`retry_count=0`，`bill_no` 非空且与同一 grant order 后续投递 / retry / 悬挂恢复 / replay 保持一致。
- grant order 的 `idempotency_key` 符合 `participation.idempotency_key + ":" + rewardCode`。
- 相同事件和相同 `Idempotency-Key` 重复上报，不新增 participation、grant order 或 outbox，不重复 quota reservation。
- 同一 participation + rewardCode 重复创建 grant order，不产生第二张发奖单。
- 金额类 0 奖励无实际权益时不生成 grant order / outbox；非金额类正数量权益即使 `reward_amount=0.0000` 也生成 grant order / outbox。
- worker 发送前必须先把 grant order 从 `PENDING` 或 `RETRY` 推进到 `SENDING`；若推进失败，不得发送外部发奖指令。
- callback `SUCCESS` 命中 `SENDING` 时更新为 `SUCCESS`，确认 quota reservation；重复成功 callback 返回 duplicate。
- callback `FAILED` 命中 `SENDING` 时更新为 `FAILED`，释放 quota reservation；重复失败 callback 返回 duplicate。
- callback `SUCCESS` 命中 `RETRY` 且 reservation 仍 `FROZEN` 时，必须合法更新为 `SUCCESS` 并确认 quota reservation。
- callback `FAILED` 命中 `RETRY` 且 reservation 仍 `FROZEN` 时，必须合法更新为 `FAILED` 并释放 quota reservation。
- callback 命中 `PENDING` 返回状态冲突，不改写状态。
- 相反终态 callback 返回冲突，不覆盖既有终态。
- worker 可重试投递失败会使 grant order 进入 `RETRY`，递增 `retry_count`，且 quota reservation 保持 `FROZEN`。
- outbox 重试耗尽或不可恢复投递失败进入 `DLQ` 时，grant order 不进入 `FAILED`，而是保持 `RETRY`，并保留错误信息。
- `SENDING` 超过默认 15 分钟未落终态时，可由恢复任务恢复为 `RETRY`，递增 `retry_count`，quota reservation 保持 `FROZEN`。
- `SENDING` 恢复为 `RETRY` 后收到迟到成功 callback，grant order 可合法进入 `SUCCESS`，quota reservation 可合法确认。
- DLQ replay 复用同一 outbox、同一 grant order、同一 `bill_no`；replay 后发送前状态流转为 `RETRY -> SENDING`。
- outbox 已 `DLQ` 且 grant order 为 `RETRY` 时收到迟到成功 callback，grant order 可合法进入 `SUCCESS`，reservation 可确认，后续 replay / close 必须因 grant order 已终态而失败。
- DLQ replay 在 quota reservation 已 `RELEASED`、grant order 已 `FAILED/SUCCESS/CLOSED` 或 outbox 非 `DLQ` 时必须失败。
- DLQ close 后 outbox 进入 `DLQ_CLOSED`，grant order 进入 `CLOSED`，释放冻结 quota，并禁止后续 replay / callback 改写。
- quota 不足时不留下 `REWARDABLE` participation、grant order 或 outbox。
- PostgreSQL 模式下唯一约束、事务、callback tenant 隔离、outbox claim、`SENDING` 悬挂恢复、DLQ replay / close 状态条件和 quota reservation 语义通过验证；不能只以内存仓储作为资金链路验收依据。
- 研发交付最低验证项包括 `go test ./...`、`go build ./...`，以及与 PostgreSQL 相关的幂等、状态流转、迟到 callback、SENDING 恢复、DLQ replay / close 和 quota reservation 测试。

## 待确认问题

- 是否需要独立的 grant order 人工关闭 Admin API？
  - 默认最小口径：不需要。本任务只要求 outbox DLQ close 能联动 grant order `CLOSED`；独立管理入口另立需求。
- `bill_no` 对外格式是否要求固定前缀如 `cmd_`？
  - 默认最小口径：格式不强制，但必须稳定、唯一、非空，并可作为 callback `commandId`。
- 外部奖励系统业务失败 callback 后是否允许人工复核重发？
  - 默认最小口径：不允许同一 grant order replay。`FAILED` 为业务失败终态，已释放 quota；重发、重新冻结或补偿发奖另立需求。
- `SENDING` 悬挂恢复阈值是否需要按 tenant 或 channel 配置？
  - 默认最小口径：本任务固定使用 `15 分钟`，后续如需按 tenant / channel 配置另立需求。

上述问题不阻塞本轮研发，因为默认口径已经给出可验收约束。

## Score

```text
需求清晰度        18/20
范围可控          18/20
风险可控          18/20
验收标准明确      19/20
架构一致性        18/20
总分              91/100
```

扣分原因：本轮已关闭 Review 的 P1/P2 和 Open Questions，`RETRY` 可靠投递语义、迟到 callback、`SENDING` 悬挂恢复、DLQ replay / close 与 quota reservation 口径可闭环。剩余扣分来自独立 grant order 管理入口、`bill_no` 外部格式和业务失败后的补偿重发仍需后续任务明确，但这些不阻塞本任务按默认最小口径开发和验收。

## Decision

```text
PASS
```

PASS 原因：Score 为 91/100；Review 打回的 `RETRY` callback 冲突已通过“`RETRY` 表示本地未获得外部最终结果，允许迟到 callback 在 reservation 仍 `FROZEN` 时落 `SUCCESS/FAILED`”关闭；`SENDING` 悬挂恢复已给出 15 分钟默认阈值和恢复动作。需求细节清楚、可审核、可验收。
