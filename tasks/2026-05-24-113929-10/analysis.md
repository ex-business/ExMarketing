# Analysis

## 需求 ID

2026-05-24-113929-10

## 标题

对账查询能力

## Review 反馈处理

- `review.md` 当前为模板占位，`P0/P1/P2` 未填写具体问题，`Open Questions` 仅为“待填写”，`Decision` 为 `PASS / BLOCK` 占位，未提供可逐条修复的打回意见。
- 本轮按“Review 未给出实质打回项”处理，但仍补齐可审核、可验收的需求口径，避免继续停留在模板状态。

## Analysis 自检修正

- 上一轮 `analysis.md` 为模板，`现状/目标理解/方案/改动范围/风险点/验收方式` 均为“待填写”：本轮已补齐背景目标、参与方、输入输出、业务规则、边界条件、异常场景、非目标、风险和验收标准。
- 上一轮 `Score = 0`：原因是没有需求细化内容，不能进入研发。本轮基于 `task.md`、`README.md`、`CLAUDE.md`、`docs/architecture.md`、`docs/data-model.md`、`docs/api.md`、`docs/operations.md`、`docs/roadmap.md` 和 `22-独立活动营销中台设计.md` 重新定义需求。
- 上一轮 `Decision = PASS / BLOCK` 占位：本轮给出明确 `Decision PASS`。默认最小可验收口径采用既有 API 文档中的 `GET /admin/v1/reward/reconciliation` 查询能力，不要求新建独立对账任务、自动修复或外部系统拉账。
- 旧 BLOCK 关闭依据：现有材料已明确 ExMarketing 只维护营销系统自己的事件、参与、发奖、outbox、quota、审计数据；对账查询的目标是定位链路断点，不负责实际发奖或修改外部钱包/积分/优惠券状态。

## 背景目标

ExMarketing 的核心链路是：

```text
标准事件 -> event inbox -> 活动匹配/规则/奖励 -> participation -> quota reservation -> grant_order -> outbox -> 外部发奖系统 -> callback -> 状态更新/审计
```

本需求要提供一个面向运营、排障和测试的管理端对账查询能力，按活动、用户/主体、事件、参与记录、发奖单和 outbox 等维度串起同一条营销发奖链路，帮助定位以下典型问题：

- 事件已接收但未生成参与记录。
- 参与记录已生成但未生成发奖单。
- 发奖单已生成但 outbox 未生成或投递失败。
- outbox 已发送或进入 DLQ，但发奖单仍未终结。
- 外部发奖 callback 显示成功，但 participation 或 grant_order 状态未正确更新。
- quota 长期处于冻结状态，无法判断是否应等待 callback、人工关闭或走超时释放。

## 参与方

- 运营/财务对账人员：按活动、用户、事件或发奖单查询链路状态，确认发奖是否可解释。
- 技术支持/研发排障人员：定位 event、participation、grant_order、outbox、quota、audit 之间的断点。
- 外部发奖系统：只通过 command out 和 callback 与 ExMarketing 交互；本需求不要求它提供在线查询接口。
- ExMarketing Admin API：提供租户隔离的只读对账查询。

## 输入

对账查询入口采用既有文档口径：

```http
GET /admin/v1/reward/reconciliation
```

必填参数：

- `tenantId`：租户 ID，所有查询必须按 tenant 隔离。

至少提供一种定位条件：

- `commandId`：发奖指令号，对应 `marketing_grant_order.bill_no`。
- `grantOrderId`：发奖单 ID。
- `activityId`：活动 ID。
- `subjectType + subjectId`：用户或其他主体。
- `eventId`：外部事件 ID。
- `idempotencyKey`：标准事件幂等键，格式为 `tenantId:sourceSystem:eventType:bizType:bizId`。
- `participationId`：参与记录 ID。
- `outboxId`：outbox ID。

分页与范围参数：

- `limit`：默认 100，最大 500。
- `cursor`：可选游标；没有游标时按最新更新时间或创建时间倒序返回第一页。
- `from` / `to`：可选时间范围。按链路对象的创建/更新时间过滤，默认不限制；用于 `activityId` 或 `subjectId` 等宽查询时建议使用。

默认最小口径：

- 若同时提供多个定位条件，必须在同一 tenant 下取交集；条件互相冲突时返回空结果，不跨租户兜底查询。
- 若只提供 `tenantId` 且无任何定位条件，应拒绝，避免全表扫描和信息泄露。

## 输出

返回应以“链路记录”为核心，而不是只返回单表列表。每条链路记录至少包含：

- `tenantId`
- `eventInbox`：事件 inbox 摘要，包含 `id`、`eventId`、`eventType`、`sourceSystem`、`subject`、`biz`、`idempotencyKey`、`processStatus`、`errorMessage`、`createdAt`、`updatedAt`。
- `participation`：参与记录摘要，包含 `id`、`activityId`、`status`、`rejectReason`、`idempotencyKey`、`createdAt`、`updatedAt`。
- `grantOrder`：发奖单摘要，包含 `id`、`billNo/commandId`、`rewardType`、`rewardAmount`、`grantStatus`、`grantChannel`、`retryCount`、`errorMessage`、`idempotencyKey`、`createdAt`、`updatedAt`。
- `outbox`：outbox 摘要，包含 `id`、`messageType`、`targetChannel`、`targetRef`、`sendStatus`、`retryCount`、`nextRetryAt`、`errorMessage`、`createdAt`、`updatedAt`。
- `quotaReservations`：与发奖单相关的冻结/确认/释放记录摘要；若不限额或历史数据没有 reservation，明确返回空数组。
- `auditTimeline`：相关审计时间线，按时间升序展示 `action`、`resource`、`resourceId`、`actor`、`createdAt` 和脱敏后的错误/来源摘要。
- `diagnostics`：机器可读诊断项列表，用于指出链路断点和建议排查方向。

输出不得包含：

- Authorization、Cookie、token、secret、callback 签名、broker 凭据。
- 完整敏感 attributes 或 provider 响应。需要展示时只能返回已脱敏摘要。

## 业务规则

1. 查询必须租户隔离，`tenantId` 必填，所有对象匹配都必须限定在该 tenant 内。
2. 对账查询只读，不得修改 event inbox、participation、grant_order、outbox、quota 或 audit。
3. 对账链路以现有幂等关系串联：
   - event inbox 幂等键：`tenantId:sourceSystem:eventType:bizType:bizId`。
   - participation 幂等键：`tenantId:activityId:eventInbox.idempotencyKey`。
   - grant order 幂等键：`participation.idempotencyKey:rewardCode`。
   - grant order 的 `billNo` 即外部发奖 `commandId`。
4. `eventId` 查询应返回该事件在当前 tenant 下关联的 event inbox、所有命中的 participation，以及其下游 grant_order/outbox。
5. `idempotencyKey` 查询优先按标准事件幂等键定位 event inbox；如果传入的是 grant order 幂等键，也应允许定位到对应 grant_order 并回溯 participation/event inbox。
6. `activityId` 查询返回该活动下的链路记录，必须支持分页，默认按最新业务更新时间倒序。
7. `subjectType + subjectId` 查询返回该主体在当前 tenant 下的链路记录，必须支持分页。
8. `commandId/grantOrderId` 查询返回单个或少量发奖单链路，并回溯 participation/event inbox、关联 outbox、quota reservation 和 audit。
9. `participationId` 查询返回该参与记录及其所有发奖单和 outbox。
10. `outboxId` 查询返回该 outbox 及其可关联的 grant_order、participation、event inbox；若历史 outbox 缺少直接外键，应通过 payload 中的 `commandId` 或 `idempotencyKey` 做最小回溯。
11. outbox `SENT` 只表示发奖指令已到达 HTTP/Webhook/MQ 边界或 broker ack，不等于外部权益已发放；真实发奖成功以 callback 使 grant_order 进入 `SUCCESS` 为准。
12. `DLQ` 和 `DLQ_CLOSED` 必须作为可见状态返回，便于判断消息失败和人工关闭。
13. 审计时间线只展示与本次链路相关的记录，不应返回整个 tenant 审计列表。

## 诊断规则

`diagnostics` 至少覆盖以下最小规则：

- `EVENT_RECEIVED_NO_PARTICIPATION`：event inbox 存在且非重复，但没有 participation。
- `PARTICIPATION_REJECTED`：participation 为 `REJECTED`，展示 `rejectReason`，这是业务拒绝而非发奖异常。
- `PARTICIPATION_REWARDABLE_NO_GRANT_ORDER`：participation 为 `REWARDABLE` 但没有 grant_order，疑似链路写入异常。
- `GRANT_ORDER_NO_OUTBOX`：grant_order 存在但没有关联 outbox，疑似发奖指令未落 outbox。
- `OUTBOX_FAILED_OR_DLQ`：outbox 为 `FAILED`、`DLQ` 或 `DLQ_CLOSED`，说明消息投递失败或已被人工关闭。
- `OUTBOX_SENT_GRANT_PENDING`：outbox 为 `SENT`，但 grant_order 仍为 `PENDING/SENDING/RETRY`，说明需等待 callback 或排查外部发奖系统。
- `GRANT_SUCCESS_PARTICIPATION_NOT_GRANTED`：grant_order 为 `SUCCESS`，但 participation 未进入 `GRANTED`，对应验收中的“发奖成功但状态未更新”。
- `GRANT_FAILED_QUOTA_STILL_FROZEN`：grant_order 为 `FAILED/CLOSED`，但 quota reservation 仍为 `FROZEN`，疑似释放异常。
- `GRANT_PENDING_QUOTA_FROZEN_TOO_LONG`：grant_order 未终结且 reservation 长期 `FROZEN`，建议结合超时释放 dry-run 处理。
- `ZERO_REWARD_SKIPPED`：participation 为 `QUALIFIED` 且 reject reason 表示 0 奖励跳过，不应被误判为发奖失败。

## 边界条件

- 重复事件：重复上报同一幂等键不会重复创建 participation/grant_order/outbox；对账查询应能显示 duplicate 或只读到原始链路，不得把“没有新发奖”判断为异常。
- 规则拒绝：`REJECTED` participation 是正常终态，应展示拒绝原因。
- 0 奖励：`QUALIFIED + ZERO_REWARD_SKIPPED` 是正常终态，不生成 grant_order/outbox。
- 不限额活动：可以没有 quota row 或 quota reservation，对账结果应返回空数组并标注 `unlimited` 或“无 reservation”，不得报错。
- 非金额奖励：grant_order `rewardAmount=0.0000` 不代表未发奖，应从 `rewardPayload` 摘要展示权益数量或类型。
- 多奖励活动：一个 participation 可能有多个 grant_order/outbox，对账结果必须全部返回，不能只返回第一条。
- 多活动命中：一个事件可能命中多个活动，按 event 查询时应返回多条 participation 链路。
- 宽查询：activity 或 subject 查询可能数据量大，必须分页；默认 limit 100，最大 500。
- 历史不完整数据：若缺少某一环节，只返回能定位到的上游/下游并给出诊断，不因局部缺失导致整次查询失败。

## 异常场景与响应

- 缺少 `tenantId`：`400 INVALID_RECONCILIATION_QUERY`。
- 没有任何定位条件：`400 INVALID_RECONCILIATION_QUERY`。
- `limit` 非法或超过最大值：`400 INVALID_RECONCILIATION_QUERY`。
- `from/to` 时间格式非法或 `from > to`：`400 INVALID_RECONCILIATION_QUERY`。
- `subjectType` 和 `subjectId` 只传其一：`400 INVALID_RECONCILIATION_QUERY`。
- 查询条件合法但无数据：`200 OK`，返回空 `items` 和空 `diagnostics`。
- 多条件互相冲突：`200 OK`，返回空 `items`，不得跨条件放宽查询。
- 发生内部存储错误：`500 RECONCILIATION_QUERY_FAILED`，错误响应不得泄露 SQL、secret 或完整 payload。

## 非目标范围

- 不实现自动对账任务、定时扫描、报表导出或财务清结算。
- 不直接调用外部钱包、积分、优惠券或礼品系统查询真实发放结果。
- 不修改发奖单、参与记录、outbox、quota 或审计状态。
- 不提供 DLQ replay/close 或 quota timeout-release 的新操作能力；这些属于既有/其他运维接口。
- 不新增业务专用字段、业务专用 controller 或外部业务库依赖。
- 不解决历史数据补写、跨租户汇总、生产 Admin 鉴权设计。

## 风险

- 宽查询可能带来性能风险，需要分页、limit 上限和必要索引支持。
- 对账信息涉及用户、活动、发奖和错误上下文，必须做 tenant 隔离和敏感字段脱敏。
- 不同链路对象可能没有直接外键，尤其 outbox 可能需要从 payload 摘要回溯，需求上只要求“能定位时关联，不能定位时返回孤立 outbox 并给出诊断”。
- `SENT`、`SUCCESS`、`FAILED` 的语义容易混淆，返回中必须明确 outbox 投递状态不等于真实发奖状态。
- 多奖励、多活动、多 outbox 的结果结构需要避免丢失下游对象。

## 验收标准

1. 缺少 `tenantId` 或缺少定位条件时，请求被拒绝，返回机器可识别错误。
2. 按 `activityId` 查询时，能分页返回该活动下 event inbox、participation、grant_order、outbox、quota reservation 和 audit 时间线摘要。
3. 按 `subjectType + subjectId` 查询时，只返回当前 tenant 下该主体相关链路，不返回其他用户或其他租户数据。
4. 按 `eventId` 或标准 `idempotencyKey` 查询时，能从事件回溯/下钻到所有 participation、grant_order 和 outbox。
5. 按 `participationId` 查询时，能返回该参与记录及其所有发奖单和 outbox。
6. 按 `commandId` 或 `grantOrderId` 查询时，能返回发奖单、上游 participation/event inbox、关联 outbox、quota reservation 和审计时间线。
7. 按 `outboxId` 查询时，能返回 outbox 及其可关联的发奖链路；若无法关联上游，仍返回 outbox 本身并给出诊断。
8. 构造“grant_order 存在但 outbox 为 FAILED/DLQ”的数据时，返回 `OUTBOX_FAILED_OR_DLQ`，可定位“业务日志写了但消息失败”。
9. 构造“grant_order=SUCCESS 但 participation 未 GRANTED”的数据时，返回 `GRANT_SUCCESS_PARTICIPATION_NOT_GRANTED`，可定位“发奖成功但状态未更新”。
10. 构造“participation=REJECTED”或“ZERO_REWARD_SKIPPED”时，对账结果标识为正常业务终态，不误报为消息失败。
11. 多奖励 participation 返回多个 grant_order/outbox，不丢记录。
12. 返回内容不包含 secret、token、完整 Authorization、Cookie、callback 签名或 broker 凭据。

## 评分

```text
需求清晰度        18/20
范围可控          18/20
风险可控          17/20
验收标准明确      19/20
架构一致性        19/20
总分              91/100
```

扣分说明：

- Admin 鉴权生产化不在本需求内，仍依赖后续统一安全设计。
- outbox 与 grant_order 的直接关联方式在现有需求材料中未完全强约束，因此保留“可关联时回溯、不可关联时返回孤立 outbox”的最小口径。

## 待确认问题

- 是否需要在后续需求中增加对账导出、定时扫描和告警能力。本需求默认不做。
- 是否需要定义对账查询的生产 Admin 鉴权与角色权限。本需求默认沿用现有 Admin API 边界，不单独设计权限。
- 是否要求对接外部发奖系统的在线查单接口。本需求默认不查外部系统，只展示 ExMarketing 内部链路和 callback 结果。

## Decision

```text
PASS
```
