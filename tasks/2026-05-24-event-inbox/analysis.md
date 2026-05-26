# Analysis

## Review 反馈处理

`review.md` 当前为模板状态：`P0/P1/P2` 未填写具体问题，`Open Questions` 为“待填写”，`Decision` 为 `PASS / BLOCK` 占位。因此本轮没有可逐条修复的实际 Review 打回意见。

处理方式：

- 对空白 `P0/P1/P2`：视为 Review Agent 尚未给出具体缺陷，本轮不引入额外假设。
- 对 `Open Questions: 待填写`：按任务材料、backlog、README、API 文档、数据模型、运维文档和源设计文档补全需求口径；没有发现必须阻塞研发的缺口。
- 对 `Decision: PASS / BLOCK` 占位：本轮重新给出 Analysis 决策，供下一轮 Review Agent 审核。

## Analysis 自检修正

上一轮 `analysis.md` 为模板状态，存在以下问题：

- `现状 / 目标理解 / 方案 / 改动范围 / 风险点 / 验收方式` 均为“待填写”。
- `Score = 0`，低于 80，按规则必须 BLOCK。
- `Decision = PASS / BLOCK` 占位，没有可执行结论。

本轮修正：

- 基于 `task.md` 和 `requirements/backlog.json` 明确需求来源为“22-独立活动营销中台设计”，任务目标为 `marketing_event_inbox` 事件幂等处理。
- 基于 `README.md`、`docs/api.md`、`docs/data-model.md`、`docs/operations.md` 和源设计文档，补齐幂等键格式、重复事件响应、冲突事件响应、参与/发奖/预算副作用边界、验收标准和非目标范围。
- 将上一轮模板导致的 BLOCK 修正为可审核需求；本轮不读取代码、不提出具体代码改法。

## 需求说明

为标准事件接入链路补齐 Event Inbox 幂等语义。外部系统通过 `POST /openapi/v1/events` 上报标准事件时，系统必须以稳定的 `Idempotency-Key` 建立唯一收件箱记录，保证同一业务事件重复推送时不会重复参与活动、不会重复生成发奖单、不会重复写 outbox、不会重复扣减预算/库存/用户次数。

最小需求口径：

- 请求头 `Idempotency-Key` 必填。
- `Idempotency-Key` 必须等于 `tenantId:sourceSystem:eventType:bizType:bizId`。
- `marketing_event_inbox.idempotency_key` 必须具备唯一性语义。
- 完全重复的标准事件使用相同 key 重放时，返回 `200 OK` 且 `duplicate=true`，不产生新的业务副作用。
- 相同 key 对应不同事件内容时，返回 `400 INVALID_IDEMPOTENCY_KEY`，不重新处理。

## 背景目标

ExMarketing 是独立活动营销中台，只接收标准事件、判断活动配置、生成参与记录、发奖单和发奖指令，不直接修改外部业务余额。

源设计文档要求统一幂等：`tenant_id + source_system + event_type + biz_type + biz_id`。该 key 必须稳定，保证外部系统重复推送事件时不会重复发奖、重复计数。现有 API 文档也已经约定重复事件返回 `duplicate=true`，相同 key 但 payload 不一致返回 `INVALID_IDEMPOTENCY_KEY`。

本需求的目标不是新增一种事件入口，而是让现有标准事件入口的幂等行为成为可依赖、可验证、可审计的业务契约。

## 参与方

- 外部业务系统：按标准事件模型上报事件，负责提供稳定的业务标识 `bizType + bizId` 和请求头 `Idempotency-Key`。
- ExMarketing OpenAPI：接收事件、校验认证/事件格式/幂等键，返回成功、重复、冲突或错误响应。
- Event Inbox：保存外部事件原始 payload、处理状态和幂等键，作为后续处理的唯一入口。
- 活动处理链路：基于 inbox 事件匹配活动、执行规则、创建 participation、扣 quota、创建 grant_order 和 outbox。
- 运营/排障人员：通过 audit、inbox 和后续链路记录排查重复事件、key 冲突、quota 不足等问题。

## 输入输出

输入：

- HTTP 方法和路径：`POST /openapi/v1/events`。
- Header：`Content-Type: application/json`，`Idempotency-Key`，生产开启鉴权时还需要 `Authorization: Bearer <token>`。
- Body 顶层字段仅允许：`eventId`、`eventType`、`sourceSystem`、`tenantId`、`subject`、`biz`、`occurredAt`、`attributes`。
- `subject` 仅允许 `type`、`id`。
- `biz` 仅允许 `bizType`、`bizId`。
- `occurredAt` 必须为带时区的 RFC3339 时间。
- 业务扩展字段只能放入 `attributes`。

输出：

- 首次有效事件：按现有事件处理响应返回命中活动、参与、发奖或 quota 结果。
- 完全重复事件：`200 OK`，响应包含 `duplicate=true`；按当前 API 文档，`participations`、`grantOrders`、`outboxMessages` 可为 `null`。
- 缺少 key、key 与 payload 推导结果不一致、或相同 key 对应不同事件内容：`400 INVALID_IDEMPOTENCY_KEY`。
- 非法事件模型：`400 INVALID_EVENT`。
- JSON 不可解析：`400 INVALID_JSON`。
- 超过 body 限制：`413 REQUEST_TOO_LARGE`。
- Content-Type 非 JSON：`415 UNSUPPORTED_MEDIA_TYPE`。
- 认证/授权失败：沿用 `401 UNAUTHORIZED`、`403 TENANT_FORBIDDEN`、`403 SOURCE_FORBIDDEN`。
- Quota 不足：沿用 `409 QUOTA_EXCEEDED`，且不能留下 rewardable participation、grant_order 或 outbox 脏记录。

## 业务规则

1. 幂等键生成规则固定为：

```text
tenantId + ":" + sourceSystem + ":" + eventType + ":" + biz.bizType + ":" + biz.bizId
```

2. Header `Idempotency-Key` 必须与 payload 推导出的 key 完全一致。`eventId` 用于追踪，不作为幂等主键。

3. 同一 `idempotency_key` 只能对应一个标准事件。事件内容一致时视为重复；事件内容不一致时视为 key 冲突。

4. 事件一致性比较的业务口径：JSON 空白、字段顺序不构成差异；标准事件字段值变化构成差异，包括但不限于 `eventId`、`eventType`、`sourceSystem`、`tenantId`、`subject`、`biz`、`occurredAt`、`attributes` 内任意业务值变化。

5. 完全重复事件不得再次进入会产生副作用的处理链路，不得新增 participation、grant_order、outbox，不得再次扣减 BUDGET、STOCK、USER_LIMIT。

6. 首次事件若命中活动，参与幂等键仍按数据模型约定为：

```text
tenantId + ":" + activityId + ":" + eventInbox.idempotency_key
```

同一标准事件命中同一活动只能生成一条 participation。

7. 发奖单幂等键仍按数据模型约定为：

```text
participation.idempotency_key + ":" + rewardCode
```

同一 participation 的同一奖励配置只能生成一张 grant_order。

8. 规则拒绝时可以写 `REJECTED` participation，但重复事件不得重复写拒绝参与记录。

9. 规则通过但奖励为 `0.0000` 时可以写 `QUALIFIED` participation，但重复事件不得重复写 qualified 记录，也不得生成 grant_order/outbox 或扣 quota。

10. 可发奖事件必须保持活动级原子语义：`REWARDABLE` participation、quota 扣减、grant_order、outbox 要么作为同一业务结果成立，要么在 quota 不足等失败场景下不留下部分成功副作用。

11. 并发重复请求必须满足最终业务幂等：即使两个相同 key 的请求同时到达，也最多有一个请求产生首次处理副作用，其余请求只能表现为重复或稳定的幂等结果，不允许因竞态产生双参与、双发奖、双扣减或 500。

12. 失败响应不得泄露完整 token、完整 Authorization header、内部配置或敏感 payload。

## 边界条件

- 不同租户相同 `sourceSystem/eventType/bizType/bizId` 是不同事件，因为 `tenantId` 是 key 的一部分。
- 同一租户下不同 `sourceSystem` 的同一业务 ID 是不同事件。
- 同一业务对象如果产生不同事件类型，应使用不同 `eventType`，因此 key 不冲突。
- 业务方更正事件内容时不能复用旧 key；复用旧 key 且 payload 变化必须被拒绝。
- `attributes` 可以为空对象 `{}`，但不能缺失。
- `biz.bizId` 为空、`biz.bizType` 为空、`tenantId/sourceSystem/eventType` 为空时，无法形成合法 key，应按事件格式错误处理。
- 对内存仓储和 PostgreSQL 仓储，外部可观察的幂等行为应一致；PostgreSQL 场景必须具备数据库唯一性保障。
- 已进入 `FAILED` 状态的 inbox 是否允许人工重放不在本需求内；本需求只约束同一 OpenAPI 重复上报的处理结果。

## 异常场景

- 缺少 `Idempotency-Key`：返回 `400 INVALID_IDEMPOTENCY_KEY`，不写业务副作用。
- Header key 与 payload 推导 key 不一致：返回 `400 INVALID_IDEMPOTENCY_KEY`，不写业务副作用。
- 相同 key、相同事件重复上报：返回 `200 OK duplicate=true`，不重复处理。
- 相同 key、不同 payload：返回 `400 INVALID_IDEMPOTENCY_KEY`，不重复处理。
- 并发相同 key 重复上报：最终只能保留一份 inbox 和一套业务副作用。
- 首次事件处理过程中 quota 不足：返回 `409 QUOTA_EXCEEDED`，不得生成 grant_order/outbox，不得保留 `REWARDABLE` 脏参与记录；重复上报同一事件不得造成额外扣减。
- 首次事件 JSON 非法或字段非法：按现有错误码拒绝，不进入幂等业务处理。
- 认证/授权失败：应先拒绝未授权请求，不允许绕过租户或来源系统权限写入 inbox。

## 非目标范围

- 不新增 Kafka/RabbitMQ/Pulsar 消费入口；MQ 入口由后续任务处理。
- 不新增 Webhook Receiver 映射、签名或来源校验；Webhook 入口由独立任务处理。
- 不改变活动匹配、规则引擎、奖励计算、Quota 计算公式或 Outbox 投递协议。
- 不设计人工重放 `FAILED` inbox 的后台操作。
- 不改变 callback 幂等语义。
- 不引入外部业务数据库依赖，也不把业务字段提升为标准事件顶层字段。
- 不要求重复事件返回首次处理时完整 participation/grant_order/outbox 列表；按当前 API 文档，返回 `duplicate=true` 即满足最小稳定结果。

## 风险

- 高风险：幂等失效会直接导致重复参与、重复发奖、重复预算/库存/次数扣减。
- 并发风险：仅靠应用层先查后写会在并发重复请求下产生竞态，必须以唯一性语义保证最终只有一份业务副作用。
- 数据一致性风险：inbox 去重、participation 幂等、grant_order 幂等和 quota 原子扣减必须共同成立，否则仍可能局部重复。
- 兼容风险：如果历史事件已存在但 payload 保存格式不一致，判断“相同 payload”时需要避免把 JSON 格式化差异误判为冲突。
- 排障风险：重复和冲突都属于幂等相关结果，但业务含义不同，错误码和 audit/inbox 状态需要可区分。

## 验收标准

功能验收：

- 使用合法 key 首次上报标准事件，系统正常完成现有活动处理链路。
- 使用相同 key 和相同事件再次上报，返回 `200 OK` 且 `duplicate=true`。
- 重复上报后，`marketing_event_inbox` 对该 `idempotency_key` 只有一条唯一记录。
- 重复上报后，不新增第二条同活动 participation。
- 重复上报后，不新增第二张同 reward grant_order。
- 重复上报后，不新增第二条对应 outbox 消息。
- 重复上报后，BUDGET、STOCK、USER_LIMIT 的 used 值不再次增加。
- 相同 key 但修改 `attributes`、`subject`、`occurredAt` 或其他标准事件字段时，返回 `400 INVALID_IDEMPOTENCY_KEY`。
- Header key 与 payload 推导 key 不一致时，返回 `400 INVALID_IDEMPOTENCY_KEY`。
- 缺少 Header key 时，返回 `400 INVALID_IDEMPOTENCY_KEY`。
- 并发提交相同 key 的相同事件时，最终只产生一套业务副作用，不能出现 500、重复扣减或重复发奖。

验证要求：

- 需要覆盖无奖励、规则拒绝、正常发奖、quota 不足、重复请求、key 冲突、并发重复请求场景。
- 涉及 PostgreSQL 唯一约束和事务语义时，必须使用 PostgreSQL 仓储验证，不能只依赖内存仓储。
- 基础验证命令仍为 `go test ./...` 和 `go build ./...`。
- 如涉及 schema 变更，需验证 migration 后 `marketing_event_inbox.idempotency_key` 的唯一性语义存在且可阻止重复写入。

## 待确认问题

无阻塞问题。以下为后续可优化项，不影响本需求进入研发：

- 是否需要让重复事件响应回放首次处理的完整业务结果，而不是仅返回 `duplicate=true`。当前 README/API 已定义最小稳定结果为 `duplicate=true`，本需求按该口径验收。
- 是否需要为 `FAILED` inbox 提供人工重放或关闭能力。当前任务只覆盖 OpenAPI 重复上报幂等，不覆盖失败事件运维状态机。

## Score

```text
需求清晰度        18/20
范围可控          19/20
风险可控          18/20
验收标准明确      19/20
架构一致性        19/20
总分              93/100
```

## Decision

```text
PASS
```

需求：2026-05-24-event-inbox
