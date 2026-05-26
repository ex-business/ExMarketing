# Analysis: 2026-05-24-dlq

## 资料范围

本轮只做需求分析和需求细化，未读取实现代码，未设计具体代码改法，未修改业务代码，未提交。

已读取：

- `tasks/2026-05-24-dlq/task.md`
- `tasks/2026-05-24-dlq/review.md`
- `tasks/2026-05-24-dlq/analysis.md` 上一轮内容
- `README.md`
- `CLAUDE.md`
- `docs/architecture.md`
- `docs/api.md`
- `docs/data-model.md`
- `docs/operations.md`
- `/Users/tong/Documents/GitHub/OracleBetX-Projects/act-analysis/22-独立活动营销中台设计.md`
- 相关任务资料：`tasks/2026-05-24-outbox-worker-dlq/task.md`、`tasks/2026-05-24-outbox-worker-dlq/analysis.md`

未发现独立 `PRD/` 目录。本分析以任务说明、上游设计文档、README、架构文档、API 文档、数据模型、运维文档和相关 Outbox/DLQ 任务产物为依据。

## Review 反馈处理

`tasks/2026-05-24-dlq/review.md` 当前仍是模板：

- `P0:` 空。
- `P1:` 空。
- `P2:` 空。
- `Open Questions` 为“待填写”，没有具体问题。
- `Decision` 为 `PASS / BLOCK` 模板，没有明确打回原因。

处理结论：

- 未发现 Review Agent 的实质打回意见，无需逐条修复 Review 缺陷。
- 本轮仍按“返工”要求补齐 `Review 反馈处理` 小节，说明没有可执行的 Review blocker。
- 后续 Review Agent 可直接按本轮细化后的需求、边界、验收标准和评分重新审核。

## Analysis 自检修正

上一轮 `analysis.md` 是空模板，存在以下自检问题：

- `现状`、`目标理解`、`方案`、`改动范围`、`风险点`、`验收方式` 均为“待填写”。
- `Score` 为 `0`，低于 80。
- `Decision` 为 `PASS / BLOCK` 模板，未给出明确结论。
- 未回应任务中“查看失败原因、手动重放、关闭、重新进入 outbox、审计、防重复扣预算、防重复生成 grant_order”的验收语义。

修正方式：

- 本轮补齐背景目标、参与方、输入输出、业务规则、边界条件、异常场景、非目标范围、风险、验收标准和待确认问题。
- 对未唯一明确的信息给出默认最小可验收口径：DLQ 管理以 Admin API 为主，查看范围按 tenant 隔离，replay/close 只允许当前 `DLQ` 状态，replay 只把同一 outbox 重新置为可投递，不重新执行活动链路。
- 用上述默认口径关闭旧 `Score 0` 和模板化 Decision 的 BLOCK。

## 结论摘要

本需求可以进入 Review。

本期需求不是重新实现活动匹配、奖励计算、Quota 扣减、grant_order 生成或 Outbox Worker 投递策略，而是细化并验收 DLQ 管理能力：

- 查看 DLQ 失败消息和失败原因。
- 手动 replay，使同一 outbox 重新进入可投递流程。
- 手动 close，使同一 outbox 进入机器可判定的关闭状态。
- replay / close 必须写审计。
- replay 不得重复扣减预算、库存、用户次数，也不得重复创建 participation、grant_order 或 outbox。
- replay 必须复用原 outbox payload 和原 `commandId` / `bill_no`，依赖外部发奖系统按同一 command 幂等处理。

默认最小可验收口径：

- 操作入口以 Admin API 为准：`GET /admin/v1/outbox/dlq`、`POST /admin/v1/outbox/{id}/replay`、`POST /admin/v1/outbox/{id}/close`。
- 管理页面展示、批量处理、告警、导出、重新打开 `DLQ_CLOSED` 不属于本期必做。

## 背景目标

ExMarketing 是独立活动营销中台。系统不直接给用户加钱，不连接外部业务数据库，只接收标准事件，完成活动判断、奖励计算、预算库存控制、参与记录、发奖单和发奖指令编排。

标准链路：

```text
Event In -> Idempotency -> Campaign Match -> Rule Evaluation
-> Quota / Budget / Inventory -> Grant Order -> Outbox
-> External Reward System -> Callback In
```

Outbox 保证发奖指令不会因为 HTTP、Webhook、MQ 或外部目标失败而丢失。DLQ 是 Outbox 自动投递失败后的人工处理入口，用于处理长期失败、配置错误、目标不可达或业务拒绝的发奖指令。

本任务目标：

- 让运营或运维能够按 tenant 查看 DLQ 消息、失败原因和关键上下文。
- 对可恢复失败执行 replay，使消息重新进入 outbox 可投递流程。
- 对确认不再处理的失败消息执行 close，形成最终状态。
- 所有人工操作必须可审计、可追踪、可复核。
- replay 不得导致营销侧重复发奖编排，不得重复扣减预算、库存或次数，不得重复生成 grant_order。

## 参与方

- 外部业务系统：上报标准事件；不参与 DLQ 管理。
- ExMarketing 活动引擎：生成 participation、grant_order、outbox；本需求不改变活动链路。
- Outbox Worker：按已有规则投递 outbox；本需求只要求 replay 后消息重新进入 worker 可处理范围。
- 外部发奖系统：接收 Reward Command，并通过 callback 回传真实发奖结果；必须按 `commandId` / `bill_no` 幂等处理重复投递。
- 运营 / 运维人员：查看 DLQ，判断失败原因，执行 replay 或 close，填写 operator 和 reason。
- 审计查询方：通过审计记录追踪 DLQ replay / close 的操作者、原因、资源和时间线。

## 输入输出

### 输入

DLQ 查询输入：

- `tenantId`：必填，限定租户范围。
- `includeClosed`：可选，默认 `false`；为 `true` 时同时返回 `DLQ_CLOSED`。
- 默认最小口径不强制分页、排序、导出和复杂筛选；如已有 `limit` 能力可沿用，但不是本期硬门槛。

DLQ 操作输入：

```json
{
  "tenantId": "site_001",
  "operator": "ops",
  "reason": "target recovered"
}
```

字段规则：

- `tenantId` 必填，用于租户隔离和防止跨租户操作。
- `operator` 必填，记录实际操作人或系统操作者。
- `reason` 必填，记录 replay 或 close 的业务原因。
- `outbox id` 来自路径参数。

被操作对象：

- `marketing_outbox` 中 `send_status = DLQ` 的单条记录。
- 记录必须保留原 `payload_json`、`target_channel`、`target_ref`、`retry_count`、`error_message`、`tenant_id`、`created_at`、`updated_at` 等上下文。

### 输出

DLQ 查询输出至少包含：

- outbox id
- tenant id
- message type
- target channel
- target ref
- send status：`DLQ` 或 `DLQ_CLOSED`
- retry count
- next retry time，如存在
- error message / failed reason 摘要
- created time
- updated time

replay 成功输出：

- 同一 outbox 的当前状态已回到可投递状态，默认口径为 `PENDING`。
- 原 `payload_json` 不变。
- 原 `commandId` / `bill_no` 不变。
- 不创建新的 participation、grant_order 或 outbox。
- 写入 `OUTBOX_DLQ_REPLAY` 审计。

close 成功输出：

- 同一 outbox 进入 `DLQ_CLOSED`。
- 该状态为机器可判定的最终关闭状态。
- 后续不允许 replay。
- 写入 `OUTBOX_DLQ_CLOSE` 审计。

失败输出：

- 非 `DLQ` 状态执行 replay / close 必须失败。
- tenant 不匹配必须失败或不可见。
- 缺少 operator 或 reason 必须失败。
- 找不到 outbox 必须返回可识别的不存在错误。
- 并发状态已变化时必须返回可识别的状态冲突或不可执行结果。

## 业务规则

### DLQ 状态语义

本需求认可的最小状态流转：

```text
PENDING -> SENDING -> SENT
PENDING -> SENDING -> FAILED -> PENDING
FAILED -> DLQ
DLQ -> PENDING
DLQ -> DLQ_CLOSED
```

规则：

- `DLQ` 表示自动投递已停止，等待人工处理。
- `DLQ_CLOSED` 表示人工关闭后的最终状态。
- 只有 `DLQ` 可以 replay。
- 只有 `DLQ` 可以 close。
- `DLQ_CLOSED` 不允许 replay。
- 是否支持 `DLQ_CLOSED` reopen 不在本期范围内。

### 查看失败原因

- 查询必须能展示最近一次失败原因或错误摘要。
- 错误摘要必须便于判断是目标不可达、HTTP 非 2xx、payload/target 配置错误、重试耗尽或其他失败。
- 错误摘要不得泄露 secret、token、完整 Authorization header、签名密钥、broker 凭据或完整敏感响应体。
- 查询结果必须能区分 `DLQ` 和 `DLQ_CLOSED`。
- 默认只查未关闭 `DLQ`；`includeClosed=true` 时包含 `DLQ_CLOSED`。

### 手动 Replay

- replay 只对当前状态仍为 `DLQ` 的同一 outbox 生效。
- replay 的业务含义是“重新投递原发奖指令”，不是“重新处理原事件”。
- replay 后同一 outbox 重新进入可投递流程，默认状态为 `PENDING`。
- replay 不重新执行活动匹配、规则判断、奖励计算、Quota 扣减、participation 创建、grant_order 创建或 outbox 创建。
- replay 必须复用原 `payload_json`、原 `target_channel`、原 `target_ref` 和原 `commandId` / `bill_no`。
- replay 后如果再次失败，继续按既有 retry / DLQ 规则处理。
- replay 成功必须写审计，记录 operator、reason、操作前状态、操作后状态、outbox id、tenant、时间。

### 手动 Close

- close 只对当前状态仍为 `DLQ` 的同一 outbox 生效。
- close 表示人工确认该失败消息不再自动或手动重放。
- close 后状态为 `DLQ_CLOSED`。
- close 不改变 participation、grant_order、quota reservation 的业务状态。
- 如果真实发奖状态仍悬挂，需要由发奖回调、对账或 quota 超时释放等独立流程处理；本需求不把 close 定义为发奖失败补偿。
- close 成功必须写审计，记录 operator、reason、操作前状态、操作后状态、outbox id、tenant、时间。

### 审计

replay / close 必须写 `marketing_audit`。

审计最小字段：

- `tenant_id`
- `actor` = operator
- `action` = `OUTBOX_DLQ_REPLAY` 或 `OUTBOX_DLQ_CLOSE`
- `resource` = `marketing_outbox` 或等价资源名
- `resource_id` = outbox id
- `payload_json`：至少包含 reason、beforeStatus、afterStatus、targetChannel、targetRef、retryCount、errorMessage 摘要、commandId / bill_no 如可从 payload 解析
- `created_at`

审计写入失败时的默认口径：

- replay / close 是人工高风险操作，审计失败不得静默成功。
- 如果审计无法写入，操作应整体失败或返回可识别错误，避免形成无审计状态变更。

### 防重复扣减和防重复发奖编排

- 同一标准事件幂等键仍固定为 `tenantId:sourceSystem:eventType:bizType:bizId`。
- 同一 participation 幂等键仍固定为 `tenantId:activityId:eventInbox.idempotency_key`。
- 同一 grant_order 幂等键仍固定为 `participation.idempotency_key:rewardCode`。
- replay 不触发事件接入链路，因此不能创建新的 inbox、participation、grant_order、quota 扣减或 outbox。
- replay 只可能导致同一外部发奖指令被再次投递；真实发奖侧必须按相同 `commandId` / `bill_no` 幂等。
- 若外部系统已成功处理但响应丢失，replay 后外部系统应返回幂等成功或重复处理结果；营销侧不得生成第二张 grant_order 来规避该问题。

### 租户隔离

- 查询、replay、close 都必须按 `tenantId` 限定。
- 路径中的 outbox id 即使存在，只要 tenant 不匹配，也不得被当前请求读取或操作。
- 审计记录必须写入对应 tenant。

### 并发规则

- replay 与 close 并发操作同一 DLQ 时，只有一个操作可以基于当前 `DLQ` 状态成功。
- replay / close 与后台 worker 或其他状态修复流程并发时，以状态条件为准；只有当前仍为 `DLQ` 才允许成功。
- 重复 replay 同一条消息时，第一笔成功后状态已离开 `DLQ`，后续请求必须失败。
- 重复 close 同一条消息时，第一笔成功后状态变为 `DLQ_CLOSED`，后续请求必须失败。

## 边界条件

- `send_status = SENT` 的 outbox 不允许 replay 或 close。
- `send_status = PENDING / SENDING / FAILED` 的 outbox 不允许通过 DLQ 管理接口 replay 或 close。
- `send_status = DLQ_CLOSED` 的 outbox 不允许 replay，也不允许重复 close。
- `target_channel` 可以是 HTTP、WEBHOOK 或已配置的 MQ；DLQ 管理本身不按通道重新解释业务，只操作 outbox 状态和原 payload。
- replay 后能否立即实际投递取决于对应 worker / sender 是否启用和配置完整；本需求验收 replay 的最小口径是重新进入可投递 outbox 流程。
- 历史异常数据中 `payload_json` 缺少 commandId / bill_no 时，查询和 close 仍可处理；replay 是否能投递应按 worker 既有 payload 校验失败规则进入失败或 DLQ，不允许生成新 command。
- 错误信息为空时，查询仍应返回记录，失败原因可显示为空或“unknown”，不得阻断列表。
- 管理 API 当前定位为本地/内部 Admin 边界；生产鉴权和权限模型不是本期目标，但本需求不得削弱现有安全边界。

## 异常场景

- outbox id 不存在：返回不存在错误，不写 replay / close 审计。
- tenantId 缺失：请求失败。
- tenantId 与 outbox tenant 不匹配：请求失败或不可见。
- operator 缺失或空白：请求失败。
- reason 缺失或空白：请求失败。
- 对非 `DLQ` 状态 replay：请求失败。
- 对非 `DLQ` 状态 close：请求失败。
- 对 `DLQ_CLOSED` replay：请求失败。
- replay / close 并发冲突：只有一个成功，另一个返回状态冲突或不可执行。
- 审计写入失败：操作不得静默成功。
- replay 后目标继续失败：按既有 retry / DLQ 规则再次进入失败或 DLQ。
- replay 后外部系统返回重复 command：营销侧按 outbox 投递结果处理，不重新创建业务记录。
- close 后外部 callback 到达：由 callback 既有规则按 `tenantId + commandId` 处理 grant_order，不由 DLQ close 拦截或改写。

## 非目标范围

- 不重新设计 Outbox Worker 的 claim、retry、指数退避、失败分类或 sender adapter。
- 不重新处理原始事件。
- 不重新执行活动匹配、规则判断、奖励计算、Quota 扣减或 participation 生成。
- 不重新生成 grant_order、bill_no、commandId 或 outbox。
- 不直接修改用户余额、积分、优惠券、权益或外部发奖系统状态。
- 不实现外部发奖系统的幂等能力。
- 不实现批量 replay、批量 close、导出、告警、通知、工单流或审批流。
- 不实现 `DLQ_CLOSED` reopen。
- 不实现完整生产 Admin 鉴权和 RBAC；如现有 Admin 是本地开发入口，本任务不扩大其生产暴露范围。
- 不把 close 等同于发奖失败补偿；quota reservation 释放、grant_order 终态修复、对账和 callback 冲突处理由已有或独立需求负责。

## 风险

- 重复外部投递风险：外部系统成功处理但响应丢失时，replay 会再次发送同一 command；必须依赖外部 `commandId` / `bill_no` 幂等。
- 误 close 风险：close 后消息不再 replay，可能导致真实权益未发放；必须强制 operator、reason 和审计。
- 审计缺失风险：人工操作无审计会破坏追责；审计失败不得静默成功。
- 租户越权风险：仅凭 outbox id 操作可能跨 tenant；必须以 tenant 条件限制查询和更新。
- 状态竞态风险：replay / close 并发会造成不一致；必须用状态条件保证只有当前 `DLQ` 可变更。
- 语义混淆风险：replay 容易被误解为重新跑活动；需求明确 replay 只重投原 outbox payload。
- 悬挂补偿风险：close 不处理 grant_order / quota reservation 终态，可能仍需对账或超时释放流程衔接。

## 验收标准

### 查询

- 当 tenant 下存在 `DLQ` outbox 时，`GET /admin/v1/outbox/dlq?tenantId=<tenant>` 返回这些记录。
- 默认不返回 `DLQ_CLOSED`。
- `includeClosed=true` 时返回 `DLQ` 和 `DLQ_CLOSED`。
- 查询结果至少包含 outbox id、tenant、message type、target channel、target ref、send status、retry count、error message、created time、updated time。
- 不同 tenant 的 DLQ 不可通过当前 tenant 查询看到。

### Replay

- 对当前状态为 `DLQ` 的 outbox 执行 replay，且请求包含匹配 tenant、operator、reason 时，请求成功。
- replay 成功后同一 outbox 状态回到 `PENDING` 或等价可投递状态。
- replay 后 `payload_json` 不变，`commandId` / `bill_no` 不变。
- replay 后 participation 数量不增加。
- replay 后 grant_order 数量不增加。
- replay 后 outbox 数量不增加。
- replay 后 quota 已用、冻结或次数扣减不因 replay 本身重复增加。
- replay 写入 `OUTBOX_DLQ_REPLAY` 审计，审计能看到 operator、reason、outbox id、beforeStatus、afterStatus。

### Close

- 对当前状态为 `DLQ` 的 outbox 执行 close，且请求包含匹配 tenant、operator、reason 时，请求成功。
- close 成功后同一 outbox 状态为 `DLQ_CLOSED`。
- `DLQ_CLOSED` 不允许 replay。
- close 不创建 participation、grant_order 或 outbox。
- close 不移动 quota。
- close 写入 `OUTBOX_DLQ_CLOSE` 审计，审计能看到 operator、reason、outbox id、beforeStatus、afterStatus。

### 错误与并发

- replay / close 缺少 tenantId、operator 或 reason 时失败。
- replay / close tenant 不匹配时失败或不可见。
- replay / close 非 `DLQ` 状态时失败。
- replay `DLQ_CLOSED` 时失败。
- 两个并发请求同时 replay / close 同一条 `DLQ`，最多一个成功。
- 审计写入失败时，replay / close 不得表现为成功。

### 安全和可追踪

- 错误摘要、查询结果和审计 payload 不包含 secret、token、完整 Authorization header、签名密钥、broker 凭据或完整敏感响应体。
- 所有成功人工操作都可通过审计查询追踪。
- 本需求不要求公网生产暴露无鉴权 Admin 操作。

## 待确认问题

当前不存在阻塞进入研发的待确认问题。以下为非阻塞产品增强项，默认不纳入本期验收：

- 是否需要 DLQ 列表分页、排序、按 channel/status/error 过滤。
- 是否需要 Admin 页面可视化操作，而不仅是 Admin API。
- 是否需要批量 replay / close。
- 是否需要 close 时联动 grant_order 置为 CLOSED 或触发 quota 超时释放。
- 是否需要对 operator 接入真实登录用户，而不是请求体传入。

默认约束：

- 本期按 Admin API 完成最小闭环。
- close 不联动 grant_order / quota。
- operator 由请求体传入并写审计。
- 批量和页面能力后续单独排期。

## Score

```text
需求清晰度        18/20
范围可控          19/20
风险可控          18/20
验收标准明确      19/20
架构一致性        19/20
总分              93/100
```

扣分原因：

- 生产级 Admin 鉴权和真实 operator 来源仍未在本任务中定义，按非目标处理。
- close 是否联动 grant_order / quota 存在产品扩展空间，本期用“不联动”默认口径关闭。

## Decision

```text
PASS
```

需求：2026-05-24-dlq
