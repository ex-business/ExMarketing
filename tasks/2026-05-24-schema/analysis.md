# Analysis

## 需求 ID

2026-05-24-schema

## Review 反馈处理

- `review.md` 当前是 Review Agent 模板，`Findings` 中仅有空的 `P0/P1/P2` 占位，没有具体问题描述；本次无需针对实际 P0/P1/P2 修复，但已按高风险需求补齐可审核需求。
- `Open Questions` 当前为“待填写”，未列出具体问题；本次分析在“待确认问题”中补充了仍需产品/接口 Owner 明确的字段约束边界。
- `Decision` 当前为 `PASS / BLOCK` 模板占位，没有实际打回结论；本次不视为 Review 打回，但仍保留本小节，便于后续 Review Agent 核对。

## 背景目标

ExMarketing 是独立活动营销中台，只通过标准事件接收外部业务系统事实，不连接外部业务库、不引用外部业务代码包、不内置订单/充值/会员/游戏等具体业务模型。标准事件是活动匹配、规则判断、奖励计算、幂等、参与记录、发奖单和 outbox 的入口数据。

本需求目标是为标准事件模型建立明确的 Schema 校验规则，保证进入系统的事件只包含统一字段：

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

业务扩展字段只能放入 `attributes`。系统不得因为某个业务场景而把 `amount`、`currency`、`orderId`、`vipLevel`、`ip`、`deviceId` 等字段提升为顶层字段或内置业务模型。

## 参与方

- 外部业务系统：按标准事件协议上报用户/业务事件。
- OpenAPI 事件接入方：接收 `POST /openapi/v1/events` 请求并返回明确错误。
- 活动匹配和规则引擎：只消费通过校验的标准事件；通过 `attributes.*`、`state.*`、`facts.*` 等路径读取规则数据。
- 事件收件箱：保存通过校验的原始事件 payload、标准字段和幂等键。
- API 文档和测试 Owner：补充字段级约束、错误示例和单元测试覆盖。

## 输入输出

### 输入

HTTP 标准事件上报：

```http
POST /openapi/v1/events
Content-Type: application/json
Idempotency-Key: tenantId:sourceSystem:eventType:bizType:bizId
```

请求体必须是 JSON object，且顶层只允许：

```json
{
  "eventId": "evt_20260524_000001",
  "eventType": "DEPOSIT_SUCCESS",
  "sourceSystem": "payment",
  "tenantId": "site_001",
  "subject": {"type": "USER", "id": "10001"},
  "biz": {"bizType": "DEPOSIT", "bizId": "pay_123"},
  "occurredAt": "2026-05-24T12:00:00Z",
  "attributes": {
    "amount": "299.00",
    "currency": "CNY",
    "isFirstDeposit": true
  }
}
```

### 输出

- 校验通过：进入既有事件接入流程，后续幂等、inbox、活动匹配、规则执行、奖励和 outbox 行为不因本需求改变。
- 校验失败：返回 `400 INVALID_EVENT`，错误信息必须能定位到字段和原因。
- 幂等键缺失、格式不匹配或 header/payload 不一致：仍返回 `400 INVALID_IDEMPOTENCY_KEY`，不混入 `INVALID_EVENT`。

## 字段 Schema 规则

### 顶层字段

- 必填字段：`eventId`、`eventType`、`sourceSystem`、`tenantId`、`subject`、`biz`、`occurredAt`、`attributes`。
- 不允许任何未知顶层字段。示例：`amount`、`currency`、`orderId`、`userId`、`deviceId` 作为顶层字段必须拒绝。
- 每个必填字符串字段必须是非空字符串；仅空白字符视为非法。
- 顶层字段不能使用 `null` 代替必填值。

### `eventId`

- 类型：非空字符串。
- 业务含义：外部事件 ID，用于追踪和排查；不作为唯一幂等主键。
- 非目标：本需求不新增 `eventId` 全局唯一性约束。

### `eventType`

- 类型：非空字符串。
- 业务含义：活动触发事件类型，例如 `DEPOSIT_SUCCESS`、`ORDER_PAID`。
- 合法性边界：校验结构和值形态，不在 Schema 层内置具体业务枚举。是否能触发活动由活动配置 `triggerEvents` 决定。

### `sourceSystem`

- 类型：非空字符串。
- 业务含义：事件来源系统，例如 `payment`、`order-system`。
- 合法性边界：不在 Schema 层内置来源系统白名单，除非后续另有接入安全需求。

### `tenantId`

- 类型：非空字符串。
- 业务含义：租户隔离字段，必须参与幂等键。
- 校验通过后，所有后续处理必须以该租户维度隔离。

### `subject`

- 类型：JSON object。
- 必填子字段：`type`、`id`。
- 只允许子字段：`type`、`id`。
- `subject.type`：非空字符串，表示主体类型，例如 `USER`。
- `subject.id`：非空字符串，表示主体 ID。
- 非法 subject 示例：缺少 `type` 或 `id`、字段类型不是字符串、为空字符串、`subject` 不是 object、包含 `vipLevel`/`registerDays` 等业务字段。
- 本需求不把 `subject.type` 限死为 `USER`。系统未来可能支持 `DEVICE`、`IP` 或其他主体，Schema 层只校验标准结构。

### `biz`

- 类型：JSON object。
- 必填子字段：`bizType`、`bizId`。
- 只允许子字段：`bizType`、`bizId`。
- `biz.bizType`：非空字符串，表示业务对象类型，例如 `DEPOSIT`、`ORDER`。
- `biz.bizId`：非空字符串，表示业务对象 ID。
- 非法 biz 示例：缺少 `bizType` 或 `bizId`、字段类型不是字符串、为空字符串、`biz` 不是 object、包含 `amount`/`orderStatus` 等业务字段。
- 本需求不内置具体 `bizType` 枚举，避免把业务模型固化进营销系统。

### `occurredAt`

- 类型：非空字符串，内容必须是可解析的 RFC3339 时间。
- 允许带时区偏移，例如 `2026-05-24T12:00:00Z`、`2026-05-24T12:00:00+08:00`。
- 非法时间必须返回明确错误，包括空值、非字符串、无法解析、缺少时区、日期越界。
- 本需求不定义事件发生时间与当前服务时间的最大偏移窗口；超远过去/未来是否拒绝列为待确认问题。

### `attributes`

- 类型：JSON object。
- 允许为空 object。
- 业务字段必须放在 `attributes` 内，且系统不内置具体业务字段模型。
- `attributes` 内允许 JSON 基础类型、object、array，以支持规则字段路径读取；金额类值建议由业务系统以字符串传递，避免浮点精度歧义。
- `attributes` 不能是 array、string、number、boolean 或 null。
- 本需求不校验 `attributes.amount`、`attributes.currency`、`attributes.vipLevel` 等业务字段是否存在或取值是否合法；这些属于活动规则、奖励配置或业务接入契约范围。

## 幂等键规则

`Idempotency-Key` Header 必填，且必须等于：

```text
tenantId + ":" + sourceSystem + ":" + eventType + ":" + biz.bizType + ":" + biz.bizId
```

示例：

```text
site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_123
```

边界：

- 缺少 header、header 为空、header 与 payload 标准字段不一致，返回 `400 INVALID_IDEMPOTENCY_KEY`。
- 同一 key 重复提交相同 payload，保持既有行为：`200 OK` 且 `duplicate=true`，不重复创建 participation、grant_order 或 outbox。
- 同一 key 但 payload 不一致，保持既有行为：`400 INVALID_IDEMPOTENCY_KEY`。

## 业务规则

- Schema 校验必须发生在写入 `marketing_event_inbox`、活动匹配、规则执行和奖励计算之前。
- 校验失败不得写入 inbox、participation、grant_order、outbox、quota 或 audit 中会被误认为业务成功的记录。
- API 错误必须面向接入方可定位：至少包含错误码、字段路径和原因。
- 未知字段错误必须明确指出未知字段路径，例如顶层 `amount`、`subject.vipLevel`、`biz.amount`。
- 类型错误必须明确指出期望类型和实际问题，例如 `attributes must be object`、`subject.id must be non-empty string`。
- 业务字段只能进入 `attributes` 的约束同时适用于正式事件上报和模拟运行中使用的标准事件 payload。
- Fact Query 中携带的标准事件 payload 也应保持同一事件 Schema 语义；若本次实现范围不覆盖 Fact Query，必须在文档中说明后续对齐计划。

## 边界条件

- 请求体不是合法 JSON：`400 INVALID_EVENT`。
- 请求体是 JSON 但不是 object：`400 INVALID_EVENT`。
- 缺任一必填字段：`400 INVALID_EVENT`，错误中指出缺失字段。
- 必填字段为 `null`：`400 INVALID_EVENT`。
- 字符串字段为空或仅空白：`400 INVALID_EVENT`。
- 顶层存在未知字段：`400 INVALID_EVENT`。
- `subject` 或 `biz` 存在未知字段：`400 INVALID_EVENT`。
- `attributes` 为空 object：允许。
- `attributes` 内存在任意业务字段：允许。
- `attributes` 内嵌 object/array：允许，只要整体是 JSON object。
- 时间带合法时区偏移：允许。
- 时间格式不可解析或缺少时区：拒绝。

## 异常场景

- `amount` 放在顶层：拒绝，提示业务字段必须放入 `attributes`。
- `subject` 写成 `"10001"`：拒绝，提示 `subject must be object`。
- `subject` 包含 `{"type":"USER","id":"10001","vipLevel":3}`：拒绝，提示 `subject.vipLevel` 未知。
- `biz` 缺少 `bizId`：拒绝。
- `occurredAt` 为 `2026-99-99`：拒绝。
- `attributes` 为 `null`：拒绝。
- `attributes` 为 `[]`：拒绝。
- `Idempotency-Key` 使用了不同 `bizId`：返回 `INVALID_IDEMPOTENCY_KEY`。

## 非目标范围

- 不新增具体业务字段模型或枚举，例如不校验 `amount`、`currency`、`vipLevel`、`riskTag` 的业务含义。
- 不把 `subject.type` 固定为单一枚举，除非后续产品明确要求。
- 不新增外部系统 source allowlist、租户鉴权或签名机制。
- 不改变活动匹配、规则执行、奖励计算、quota 扣减、grant_order、outbox、callback 的既有业务流程。
- 不新增数据库表或改变核心数据模型语义；若实现需要保存额外错误明细，应另开需求评估。
- 不处理 MQ Event In 或 Webhook Receiver 的完整接入；本需求以现有 HTTP Event In 为主。

## 风险

- 高风险：标准事件是全链路入口，校验过松会把业务字段泄漏到顶层并形成隐性业务模型；校验过严会阻断合法接入方。
- 高风险：`occurredAt` 时间解析不一致会影响活动窗口判断、审计和对账。
- 中风险：模拟运行、Fact Query 和正式事件上报若使用不同事件校验口径，会导致“模拟通过、正式失败”或相反结果。
- 中风险：错误码混淆 `INVALID_EVENT` 与 `INVALID_IDEMPOTENCY_KEY` 会降低接入方排障效率。
- 中风险：未知字段拒绝可能影响已有未按文档接入的调用方；需要在 API 文档中给出迁移示例。

## API 文档要求

必须补充或更新 `docs/api.md`：

- 标准事件字段表：字段、类型、是否必填、是否允许未知字段、说明。
- `subject`、`biz` 子字段约束。
- `occurredAt` 时间格式要求和合法/非法示例。
- 顶层业务字段被拒绝的示例。
- `attributes` 是唯一业务扩展入口的说明。
- `INVALID_EVENT` 与 `INVALID_IDEMPOTENCY_KEY` 的区别和示例响应。

## 测试要求

需要补充单元测试或接口级测试覆盖：

- 完整合法事件通过。
- 缺少每个必填字段时返回 `INVALID_EVENT`。
- 顶层未知字段返回 `INVALID_EVENT`。
- `subject` 缺字段、类型错误、未知字段返回 `INVALID_EVENT`。
- `biz` 缺字段、类型错误、未知字段返回 `INVALID_EVENT`。
- `occurredAt` 非法格式、缺少时区、非字符串返回 `INVALID_EVENT`。
- `attributes` 缺失、为 null、非 object 返回 `INVALID_EVENT`。
- `attributes` 为空 object 通过。
- 业务字段放在 `attributes` 内通过，放在顶层/subject/biz 内失败。
- `Idempotency-Key` 缺失或与 payload 不匹配返回 `INVALID_IDEMPOTENCY_KEY`。
- 完全重复事件仍返回 `duplicate=true`，且不重复产生后续业务记录。

## 验收标准

- 标准事件只接受 `eventId`、`eventType`、`sourceSystem`、`tenantId`、`subject`、`biz`、`occurredAt`、`attributes` 八个顶层字段。
- 缺字段、类型错误、非法时间、非法 `subject`、非法 `biz` 均返回 `400 INVALID_EVENT`，且错误能定位字段和原因。
- 业务字段只能放入 `attributes`；放在顶层、`subject` 或 `biz` 内必须被拒绝。
- `attributes` 作为 object 时可以承载任意业务字段，系统不内置具体业务模型。
- `Idempotency-Key` 校验仍按标准格式执行，错误码与 Schema 错误区分清楚。
- 正式事件上报和模拟运行的标准事件 Schema 口径一致，或文档明确说明本次只覆盖正式事件并列出后续对齐项。
- API 文档包含字段级规则、错误码、合法示例和非法示例。
- 测试覆盖上述测试要求，最低通过 `go test ./...` 和 `go build ./...`；若触及 PostgreSQL 持久化语义，还需用 Docker PostgreSQL 验证事件写入和失败不落库。

## 待确认问题

- 是否需要为标准字段增加统一命名格式限制，例如只允许 `[A-Za-z0-9_.:-]` 或长度上限？当前文档只要求非空字符串。
- 是否需要限制 `occurredAt` 与服务当前时间的最大偏移，例如拒绝未来超过 N 分钟或过去超过 N 天的事件？当前需求仅要求格式合法。
- Fact Query 的 event payload 是否必须在本需求中同步套用完全相同的 Schema 校验，还是作为后续兼容性任务处理？
- 错误响应体是否已有统一结构字段名规范，例如 `code`、`error`、`field`、`details`？若无，Dev/Test 需在不扩大范围的前提下保持现有 API 风格。

## 评分

```text
需求清晰度        18/20
范围可控          18/20
风险可控          17/20
验收标准明确      19/20
架构一致性        20/20
总分              92/100
```

扣分原因：

- 标准字段命名格式、长度上限和事件时间偏移窗口尚未在产品文档中明确。
- Fact Query 是否同步复用本次 Schema 校验仍需确认。

## Decision

```text
PASS
```

理由：核心需求已可审核、可实现、可测试、可验收；待确认问题不阻断 HTTP 标准事件 Schema 校验主流程，且不得用具体业务模型假设替代标准事件约束。
