# Analysis

## 需求 ID

2026-05-24-webhook-receiver

## Review 反馈处理

本次返工已先读取 `tasks/2026-05-24-webhook-receiver/review.md`。该文件仍是 Review Agent 模板，没有有效 `BLOCK` 结论、P0/P1/P2 条目、Open Questions 或具体打回原因。

| Review 项 | 处理结果 |
| --- | --- |
| `P0` | 占位为空，未列出实际 P0。无可逐条修正项。 |
| `P1` | 占位为空，未列出实际 P1。无可逐条修正项。 |
| `P2` | 占位为空，未列出实际 P2。无可逐条修正项。 |
| `Open Questions` | 为“待填写”，未列出实际问题。无可逐条修正项。 |
| `Decision` | 仍是 `PASS / BLOCK` 模板，不是有效审核结论；本次不沿用该占位结论。 |

## Analysis 自检修正

本次返工已读取上一版 `analysis.md`。上一版为 `Decision BLOCK`、`Score 79/100`，核心扣分项是 Webhook 对外路径、receiver 定位、配置来源、映射规则、签名/header、来源校验、专属错误码和审计字段缺 Owner 决策。

本次按返工要求处理：不能等待 Owner 决策，不能只列“需补充决策”后停止。对于现有材料没有唯一答案的点，基于标准 Event API、Event Inbox、数据模型、运维文档和原始设计文档，给出默认最小可验收口径，并用该口径关闭旧 BLOCK。

| 上轮 BLOCK / 扣分项 | 本次修正 |
| --- | --- |
| Webhook Receiver 对外路径未定。 | 默认收口为 `POST /openapi/v1/webhooks/{receiverCode}`。依据：现有外部入口统一在 `/openapi/v1/*` 下，Webhook 需要按 receiver 区分配置。 |
| receiver 标识来源未定。 | 默认只允许 path 参数 `{receiverCode}` 定位 receiver。query、header、域名不参与定位；缺失、空值或冲突均拒绝。 |
| receiver 配置来源未定。 | 默认使用启动配置 `EX_MARKETING_WEBHOOK_RECEIVERS_JSON`。本期不做数据库表、Admin API 或 UI。依据：现有 OpenAPI token 已使用环境变量配置，最小实现可避免新增管理面。 |
| 是否新增 receiver 配置模型和管理 API 未定。 | 本期非目标明确为不新增持久化配置模型和管理 API；至少支持一个 enabled receiver 配置即可验收。 |
| 映射 DSL / 映射表达方式未定。 | 默认使用受限 JSONPath 字段映射表：每个目标字段支持 `path` 或 `const` 二选一；`path` 仅支持 `$.a.b.c` 和对象字段读取，不支持表达式、过滤器、数组通配、脚本或默认值。 |
| 映射能力边界未定。 | 默认支持常量、字段重命名、显式 attributes 白名单映射；不支持隐式全量透传、复杂类型转换、表达式、脚本和 provider adapter。 |
| 签名算法和签名串未定。 | 默认 HMAC-SHA256，签名串为 `<timestamp>.<rawBody>`，rawBody 必须使用请求原始字节。 |
| 签名 header 未定。 | 默认 `X-ExMarketing-Timestamp` 和 `X-ExMarketing-Signature`。签名值为小写 hex HMAC。 |
| receiver secret 粒度未定。 | 默认每 receiver 一个 secret。不同 receiver 的 secret、tenant/sourceSystem 允许范围和映射规则互相隔离。 |
| 重放窗口未定。 | 默认 300 秒。timestamp 与服务当前时间差超过 300 秒拒绝。 |
| 来源校验依据未定。 | 默认来源校验由 receiver enabled、path receiverCode、HMAC secret、timestamp 重放窗口、映射后 tenantId/sourceSystem allowlist 共同构成；可选 `allowedIPs` 为空时不做 IP 限制。 |
| Webhook 专属错误码未定。 | 本次定义稳定错误码：`WEBHOOK_RECEIVER_NOT_FOUND`、`WEBHOOK_RECEIVER_DISABLED`、`WEBHOOK_SIGNATURE_MISSING`、`WEBHOOK_SIGNATURE_INVALID`、`WEBHOOK_TIMESTAMP_INVALID`、`WEBHOOK_SOURCE_FORBIDDEN`、`WEBHOOK_MAPPING_FAILED`、`WEBHOOK_AUDIT_FAILED`。 |
| 失败响应字段名不一致。 | 收口为 `errorCode/message`，旧 `code/error` 示例视为文档遗留差异。 |
| 审计 action / payload 未定。 | 本次定义 Webhook 审计 action 和脱敏 payload 字段，见“审计要求”。 |
| `Idempotency-Key` header 是否允许未定。 | 默认允许但不要求；若请求携带，必须等于映射后标准事件派生键，否则返回 `INVALID_IDEMPOTENCY_KEY`。 |
| `occurredAt` 缺失是否可兜底未定。 | 收口为不兜底。映射后 `occurredAt` 必须存在且为带时区 RFC3339。 |
| 本期是否只支持 JSON 未定。 | 收口为只支持 JSON Webhook。 |
| receiver 级 body size 是否沿用 `1 MiB` 未定。 | 默认沿用标准 Event API `1 MiB`。receiver 级覆盖不纳入本期。 |

结论：旧 BLOCK 已关闭。本次默认协议足够形成可研发、可审核、可验收的最小需求，不再要求 Owner 先补充决策。

本次未读取业务代码，未读取 `implementation.md`、`test-plan.md`、`test.md`，未写代码，未改代码，未提交。

## 已读取依据

- `tasks/2026-05-24-webhook-receiver/review.md`
- `tasks/2026-05-24-webhook-receiver/analysis.md`
- `tasks/2026-05-24-webhook-receiver/task.md`
- `requirements/backlog.json`
- `docs/architecture.md`
- `docs/api.md`
- `docs/data-model.md`
- `docs/operations.md`
- `docs/roadmap.md`
- `tasks/2026-05-24-schema/analysis.md`
- `tasks/2026-05-24-http-event-api/analysis.md`
- `tasks/2026-05-24-event-inbox/analysis.md`
- `/Users/tong/Documents/GitHub/OracleBetX-Projects/act-analysis/22-独立活动营销中台设计.md`

## 背景目标

ExMarketing 是独立活动营销中台。外部业务系统应通过标准事件、Fact Query、Reward Command / Callback 等边界与营销系统交互，营销系统不连接外部业务数据库、不引用外部业务代码包、不内置订单、支付、会员、游戏流水等外部业务模型。

现有标准事件入口 `POST /openapi/v1/events` 已定义事件模型、幂等键、Content-Type、body size、错误码、重复事件和审计语义。Webhook Receiver 的目标是在标准事件入口之前增加 SaaS 化接入层：外部系统推送自身 JSON Webhook payload，ExMarketing 根据 receiver 配置完成安全校验和字段映射，再把结果作为标准事件交给既有事件处理链路。

Webhook Receiver 不得绕过 event inbox、活动匹配、规则执行、quota、participation、grant_order、outbox 和 audit。

## 参与方

| 参与方 | 职责 |
| --- | --- |
| 外部 Webhook Provider | 按 receiver 约定向 ExMarketing 推送原始 JSON Webhook payload，并携带 timestamp 与 HMAC signature。 |
| Webhook Receiver | 接收请求，定位 receiver，完成 Content-Type、size、JSON、签名、重放、来源、映射、标准事件校验、失败响应和审计。 |
| 标准 Event Engine | 处理映射后的标准事件，执行幂等、活动匹配、规则、奖励、quota、participation、grant_order、outbox。 |
| 运维 / 接入配置方 | 配置 receiver、secret、tenant/sourceSystem allowlist 和字段映射。 |
| Review / Test Agent | 审核协议、边界、风险和验收覆盖。 |

## 需求说明

本期实现最小 Webhook Receiver 接入协议：

```http
POST /openapi/v1/webhooks/{receiverCode}
Content-Type: application/json
X-ExMarketing-Timestamp: <unix_seconds>
X-ExMarketing-Signature: <lowercase_hex_hmac_sha256>
Idempotency-Key: <optional>
```

`receiverCode` 是唯一 receiver 定位方式。系统按 `receiverCode` 读取启动配置，验证 receiver 已启用、签名正确、timestamp 未超窗、映射后的 tenantId/sourceSystem 在 receiver 允许范围内，然后把外部 payload 映射为标准事件。

映射后的标准事件只允许以下顶层字段：

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

`subject` 只允许 `type`、`id`；`biz` 只允许 `bizType`、`bizId`。业务扩展字段只能进入 `attributes`。

幂等键固定为：

```text
tenantId:sourceSystem:eventType:bizType:bizId
```

外部 provider event id、trace id、signature timestamp 可用于追踪、`eventId` 或审计 payload，但不得替代标准幂等键。

## 输入输出

### 配置输入

本期 receiver 配置来源固定为启动环境变量：

```text
EX_MARKETING_WEBHOOK_RECEIVERS_JSON
```

该配置是 JSON array，至少支持以下字段：

```json
[
  {
    "receiverCode": "payment-webhook",
    "enabled": true,
    "secret": "receiver-secret",
    "allowedTenantIds": ["site_001"],
    "allowedSourceSystems": ["payment"],
    "allowedIPs": [],
    "mapping": {
      "eventId": {"path": "$.id"},
      "eventType": {"const": "DEPOSIT_SUCCESS"},
      "sourceSystem": {"const": "payment"},
      "tenantId": {"const": "site_001"},
      "subject.type": {"const": "USER"},
      "subject.id": {"path": "$.user.id"},
      "biz.bizType": {"const": "DEPOSIT"},
      "biz.bizId": {"path": "$.payment.id"},
      "occurredAt": {"path": "$.createdAt"},
      "attributes": {
        "amount": {"path": "$.payment.amount"},
        "currency": {"path": "$.payment.currency"},
        "providerEventId": {"path": "$.id"}
      }
    }
  }
]
```

配置规则：

- `receiverCode` 必填、非空、唯一。
- `enabled=false` 的 receiver 必须拒绝请求。
- `secret` 必填、非空，每个 receiver 独立。
- `allowedTenantIds`、`allowedSourceSystems` 必填且非空。
- `allowedIPs` 可为空数组；为空时不做 IP allowlist。非空时请求来源 IP 必须命中。
- `mapping` 必须能生成完整标准事件。
- 映射项只能使用 `path` 或 `const` 二选一。
- `path` 仅支持从 JSON object 根读取点分字段，例如 `$.payment.amount`；不支持脚本、函数、过滤器、数组通配、表达式或外部查询。
- `attributes` 只允许显式白名单映射，不允许默认全量透传 raw payload。

### 请求输入

| 输入 | 需求 |
| --- | --- |
| URL | `POST /openapi/v1/webhooks/{receiverCode}`。 |
| `Content-Type` | 必须是 `application/json`，允许 `application/json; charset=utf-8`。 |
| `X-ExMarketing-Timestamp` | 必填 Unix seconds，必须在服务当前时间前后 300 秒内。 |
| `X-ExMarketing-Signature` | 必填小写 hex HMAC-SHA256。 |
| `Idempotency-Key` | 可选；如果携带，必须等于映射后标准事件派生键。 |
| Raw body | 外部原始 JSON payload。签名必须基于原始字节。 |

签名串：

```text
<X-ExMarketing-Timestamp> + "." + <rawBody>
```

签名算法：

```text
hex(HMAC-SHA256(receiver.secret, signingString))
```

### 成功输出

- 首次有效事件：返回与标准 Event API 一致的成功响应，至少包含 `duplicate=false`，并沿用 participation、grant_order、outbox 摘要语义。
- 完全重复事件：返回 `200 OK` 且 `duplicate=true`，不得重复创建 participation、grant_order、outbox，不得重复扣 quota。
- 标准事件处理阶段 quota 不足：沿用现有 `409 QUOTA_EXCEEDED` 语义。

### 失败输出

失败响应必须是稳定 JSON object：

```json
{
  "errorCode": "WEBHOOK_SIGNATURE_INVALID",
  "message": "webhook signature invalid"
}
```

错误码：

| HTTP | errorCode | 场景 |
| --- | --- | --- |
| 400 | `WEBHOOK_RECEIVER_NOT_FOUND` | receiverCode 缺失、为空、格式非法或不存在。 |
| 403 | `WEBHOOK_RECEIVER_DISABLED` | receiver 存在但 disabled。 |
| 400 | `WEBHOOK_SIGNATURE_MISSING` | timestamp 或 signature header 缺失。 |
| 400 | `WEBHOOK_TIMESTAMP_INVALID` | timestamp 格式非法或超过 300 秒重放窗口。 |
| 401 | `WEBHOOK_SIGNATURE_INVALID` | HMAC 摘要不匹配或 signature 格式非法。 |
| 403 | `WEBHOOK_SOURCE_FORBIDDEN` | IP allowlist 未命中，或映射后 tenantId/sourceSystem 不在 receiver allowlist。 |
| 400 | `WEBHOOK_MAPPING_FAILED` | 映射路径不存在、映射结果为空、类型不符或无法生成完整标准事件。 |
| 400 | `INVALID_JSON` | JSON 不可解析。 |
| 400 | `INVALID_EVENT` | 映射后的标准事件字段非法。 |
| 400 | `INVALID_IDEMPOTENCY_KEY` | 可选 `Idempotency-Key` 与映射结果不一致，或同 key payload 不一致。 |
| 413 | `REQUEST_TOO_LARGE` | 请求体超过 `1 MiB`。 |
| 415 | `UNSUPPORTED_MEDIA_TYPE` | 缺少 Content-Type 或不是 JSON。 |
| 409 | `QUOTA_EXCEEDED` | 标准事件处理阶段 quota 不足。 |
| 500 | `WEBHOOK_AUDIT_FAILED` | 需要审计的结果无法写审计且不能伪装成功。 |
| 500 | `INTERNAL_ERROR` | 非预期内部错误，不得泄露内部细节。 |

失败响应、日志、审计不得暴露完整 token、完整 Authorization header、完整签名、receiver secret、内部配置或完整敏感 payload。

## 映射规则

映射后的标准事件字段要求：

| 字段 | 要求 |
| --- | --- |
| `eventId` | 必填非空字符串，可来自 provider event id。 |
| `eventType` | 必填非空字符串。 |
| `sourceSystem` | 必填非空字符串，且必须在 receiver `allowedSourceSystems` 内。 |
| `tenantId` | 必填非空字符串，且必须在 receiver `allowedTenantIds` 内。 |
| `subject.type` | 必填非空字符串。 |
| `subject.id` | 必填非空字符串。 |
| `biz.bizType` | 必填非空字符串。 |
| `biz.bizId` | 必填非空字符串。 |
| `occurredAt` | 必填 RFC3339 string，必须包含时区；不得用接收时间兜底。 |
| `attributes` | 必填 object，可为空 `{}`；只包含映射白名单字段。 |

映射失败规则：

- `path` 不存在、路径中间节点不是 object、目标必填字段为 null、空字符串或类型不符，返回 `WEBHOOK_MAPPING_FAILED`。
- 标准字段需要 string 时，不做隐式 number/bool/object 转 string。
- `attributes` 中的映射值保留 JSON 原类型，但 `attributes` 本身必须是 object。
- 映射后出现标准事件未知顶层字段、`subject` 未知字段或 `biz` 未知字段，返回 `INVALID_EVENT`。
- 不允许映射层连接外部业务数据库、调用外部业务系统 SDK 或执行脚本。

## 业务规则

1. Receiver 必须先由 path `receiverCode` 唯一定位；不得使用默认 receiver、默认 secret 或默认映射兜底。
2. Content-Type、body size、JSON、receiver、enabled、timestamp、signature、IP allowlist、映射、tenant/source allowlist 必须在写入 event inbox 和业务处理前完成。
3. HMAC 签名必须基于原始请求 body 字节，不得用重序列化 JSON 参与验签。
4. timestamp 重放窗口为 300 秒；超窗请求必须拒绝。
5. receiver secret 粒度为每 receiver 一个 secret。
6. 映射结果必须是完整标准事件；缺少必填字段、字段类型不匹配或出现未知顶层字段时必须拒绝。
7. `occurredAt` 必须由 payload 或 `const` 映射提供，且带时区；不得以服务接收时间兜底。
8. 幂等键必须由映射后的标准事件字段派生，固定为 `tenantId:sourceSystem:eventType:bizType:bizId`。
9. 如果请求携带 `Idempotency-Key` header，header 必须与映射后的标准事件派生值一致；不一致返回 `INVALID_IDEMPOTENCY_KEY`。
10. 完全相同 payload、相同幂等键重复推送必须按 duplicate 成功处理。
11. 相同幂等键但标准事件 payload 不同必须返回 `INVALID_IDEMPOTENCY_KEY`，不得进入发奖链路。
12. 映射成功后必须复用既有标准事件链路，不得新增绕过 event inbox、quota、participation、grant_order 或 outbox 的发奖逻辑。
13. 成功、重复、安全失败、映射失败、标准事件校验失败、幂等冲突和 quota 失败都必须可审计。
14. Webhook Receiver 不得连接外部业务数据库，不得引用外部业务系统 SDK，不得内置某个 provider 的业务模型。

## 审计要求

Webhook Receiver 必须写入 `marketing_audit` 或等价审计记录。审计失败不得把安全失败吞掉，也不得伪装业务成功。

默认 action：

| action | 场景 |
| --- | --- |
| `WEBHOOK_RECEIVED` | 合法 Webhook 首次接收并进入标准事件链路。 |
| `WEBHOOK_DUPLICATE` | 合法重复 Webhook。 |
| `WEBHOOK_RECEIVER_REJECTED` | receiver 不存在、disabled 或来源不允许。 |
| `WEBHOOK_SIGNATURE_FAILED` | timestamp/signature 缺失、格式非法、超窗或摘要不匹配。 |
| `WEBHOOK_MAPPING_FAILED` | JSON 合法但映射失败。 |
| `WEBHOOK_EVENT_INVALID` | 映射后标准事件校验失败或幂等键不一致。 |
| `WEBHOOK_QUOTA_EXCEEDED` | 标准事件处理阶段 quota 不足，沿用 quota 审计信息。 |

默认审计字段：

| 字段 | 口径 |
| --- | --- |
| `tenant_id` | 映射成功后使用标准事件 `tenantId`；映射前失败可为空或使用 receiver 允许租户中的唯一值。 |
| `actor` | 固定 `webhook:<receiverCode>`。 |
| `resource` | 固定 `WEBHOOK_RECEIVER`。 |
| `resource_id` | `receiverCode`。 |
| `payload_json.receiverCode` | receiverCode。 |
| `payload_json.providerEventId` | 若映射到 eventId 或 attributes.providerEventId，可记录。 |
| `payload_json.eventId` | 映射成功后记录。 |
| `payload_json.idempotencyKey` | 映射成功后记录标准派生幂等键。 |
| `payload_json.rawPayloadSha256` | 原始 body SHA-256 摘要，不保存完整 raw payload。 |
| `payload_json.signatureSha256` | signature header 的 SHA-256 摘要，不保存完整签名。 |
| `payload_json.errorCode` | 失败时记录稳定错误码。 |
| `payload_json.message` | 失败摘要，不包含 secret、完整签名或敏感 payload。 |

## 边界条件

- 本期只支持 JSON Webhook；XML、form-urlencoded、CloudEvents、provider 特定二进制格式不纳入。
- 请求体大小固定沿用标准 Event API `1 MiB`；receiver 级独立覆盖不纳入本期。
- Query 参数不参与 receiver 定位、签名、来源校验或映射。
- JSON number、金额和 Decimal 语义与现有标准事件处理保持一致；金额计算仍由奖励计算链路处理。
- `attributes` 可以为空对象 `{}`，但不能缺失。
- `allowedIPs` 为空数组时不做 IP 限制；非空时必须命中请求来源 IP。代理链路下真实 IP 可信来源策略不在本期扩展。
- 高并发重复 Webhook 必须依赖既有幂等和持久化状态兜底，不得重复发奖、重复扣 quota 或重复发送 outbox。
- 同一 receiverCode 重复出现在配置中时，配置无效；服务不得随机选择其中一个。

## 异常场景

| 场景 | 期望行为 |
| --- | --- |
| receiverCode 缺失、为空或不存在 | `400 WEBHOOK_RECEIVER_NOT_FOUND`，不处理事件，写审计。 |
| receiver 已禁用 | `403 WEBHOOK_RECEIVER_DISABLED`，不处理事件，写审计。 |
| Content-Type 缺失或不支持 | `415 UNSUPPORTED_MEDIA_TYPE`。 |
| body 超限 | `413 REQUEST_TOO_LARGE`。 |
| JSON 不合法 | `400 INVALID_JSON`。 |
| timestamp 或 signature header 缺失 | `400 WEBHOOK_SIGNATURE_MISSING`。 |
| timestamp 非 Unix seconds 或超出 300 秒窗口 | `400 WEBHOOK_TIMESTAMP_INVALID`。 |
| signature 格式非法或摘要不匹配 | `401 WEBHOOK_SIGNATURE_INVALID`。 |
| 来源 IP 不在 allowlist | `403 WEBHOOK_SOURCE_FORBIDDEN`。 |
| 映射后 tenantId/sourceSystem 不在 receiver allowlist | `403 WEBHOOK_SOURCE_FORBIDDEN`。 |
| 映射路径不存在、类型不符、必填字段为空 | `400 WEBHOOK_MAPPING_FAILED`，不写标准事件。 |
| 映射后标准事件字段非法 | `400 INVALID_EVENT`。 |
| `Idempotency-Key` 与映射结果不一致 | `400 INVALID_IDEMPOTENCY_KEY`。 |
| 同幂等键不同 payload | `400 INVALID_IDEMPOTENCY_KEY`。 |
| 完全重复 payload | `200 OK duplicate=true`，不重复发奖。 |
| quota 不足 | 沿用 `409 QUOTA_EXCEEDED`。 |
| 审计记录失败 | `500 WEBHOOK_AUDIT_FAILED` 或 `500 INTERNAL_ERROR`，不得伪装成功，且不得泄露敏感信息。 |

## 非目标范围

- 不实现 MQ Event In。
- 不改变现有 `/openapi/v1/events` 标准事件接口语义。
- 不改变标准事件字段、幂等键格式、重复事件语义或 quota 语义。
- 不改变活动匹配、规则执行、奖励计算、participation、grant_order、outbox 或 DLQ 业务规则。
- 不实现外部业务数据库查询。
- 不接入外部业务系统 SDK。
- 不为单个 provider 写死业务字段、专用活动逻辑或专用发奖逻辑。
- 不实现 receiver 数据库表、Admin 配置 API 或 Admin UI。
- 不实现 XML、form-urlencoded、CloudEvents 等多格式兼容。
- 不实现复杂 JSONPath、脚本映射、表达式映射、数组过滤、外部 adapter 或 provider 模板库。
- 不实现密钥轮换、双 secret 过渡、secret 过期、WAF、mTLS、OAuth2、JWT 或生产级指标告警。
- 不定义多错误同时存在时的全局优先级；验收用例应构造单一失败原因。

## 风险

| 风险 | 级别 | 说明 | 缓解 |
| --- | --- | --- | --- |
| 映射层绕过标准事件处理 | P0 | 会导致重复发奖、跨 tenant 污染或 quota/outbox 不一致。 | 明确映射成功后必须复用既有标准事件链路。 |
| HMAC rawBody 使用错误 | P1 | 重序列化 JSON 会导致签名不稳定或被绕过。 | 签名串固定为 timestamp + rawBody。 |
| receiver 配置错误 | P1 | 错误映射可能写错 tenant/source/bizId。 | 启动配置必须校验，缺必填或重复 receiverCode 视为无效配置。 |
| allowedIPs 与代理链路 | P2 | 生产反代下真实 IP 可信边界复杂。 | 本期 allowedIPs 可为空；代理可信链另开生产化任务。 |
| 环境变量保存 secret | P2 | 本期不做密钥管理后台或加密存储。 | 日志、错误和审计禁止输出 secret；密钥轮换列为非目标。 |
| 映射能力过窄 | P2 | 复杂 provider payload 可能需要数组或表达式。 | 本期只做最小可验收白名单映射；复杂 adapter 后续扩展。 |
| 多错误优先级不统一 | P2 | 同时缺 header 和非法 JSON 时响应可能有差异。 | 验收固定单一失败原因，多错误优先级非目标。 |

## 验收标准

1. `POST /openapi/v1/webhooks/{receiverCode}` 是唯一 Webhook Receiver URL。
2. path `{receiverCode}` 是唯一 receiver 定位方式；query、header、域名不参与定位。
3. `EX_MARKETING_WEBHOOK_RECEIVERS_JSON` 能配置至少一个 enabled receiver，包含 secret、tenant/source allowlist 和 mapping。
4. receiverCode 缺失、为空、不存在、重复配置或 disabled 时返回稳定 Webhook 错误码，且不进入标准事件链路。
5. 缺少 Content-Type 或非 JSON Content-Type 返回 `415 UNSUPPORTED_MEDIA_TYPE`。
6. 请求体超过 `1 MiB` 返回 `413 REQUEST_TOO_LARGE`。
7. JSON 不可解析返回 `400 INVALID_JSON`。
8. 缺少 timestamp/signature、timestamp 格式非法、timestamp 超出 300 秒、signature 不匹配分别返回稳定错误码。
9. 签名按 HMAC-SHA256 over `<timestamp>.<rawBody>` 校验，使用每 receiver secret。
10. receiver `allowedIPs` 非空且来源 IP 不匹配时返回 `WEBHOOK_SOURCE_FORBIDDEN`。
11. 合法外部 JSON Webhook payload 能按配置映射为完整标准事件。
12. 映射只支持 `path` / `const` 和显式 attributes 白名单；不支持脚本、表达式、数组通配或隐式全量透传。
13. 映射路径不存在、必填字段为空、类型不符时返回 `WEBHOOK_MAPPING_FAILED`，不写 event inbox。
14. 映射后的 tenantId/sourceSystem 不在 receiver allowlist 时返回 `WEBHOOK_SOURCE_FORBIDDEN`。
15. 映射后的标准事件只包含允许字段，且幂等键固定为 `tenantId:sourceSystem:eventType:bizType:bizId`。
16. 合法 Webhook 首次推送进入既有事件处理链路，并可生成与标准事件接口一致的 participation、grant_order、outbox 结果。
17. 相同 Webhook payload 重复推送返回 `duplicate=true`，且不会重复创建 participation、grant_order、outbox 或重复扣 quota。
18. 同幂等键不同 payload 返回 `INVALID_IDEMPOTENCY_KEY`，不进入发奖链路。
19. 请求携带 `Idempotency-Key` 且与映射结果不一致时返回 `INVALID_IDEMPOTENCY_KEY`。
20. 成功、重复、receiver 拒绝、签名失败、来源失败、映射失败、标准事件校验失败、幂等冲突和 quota 失败均可通过审计排查。
21. 审计 payload 只保存 receiverCode、eventId、idempotencyKey、rawPayloadSha256、signatureSha256、errorCode 等脱敏字段，不保存完整 secret、完整签名或完整敏感 payload。
22. 现有 `/openapi/v1/events` 行为保持不变。
23. 文档说明 receiver 配置、签名规则、来源校验、映射规则、错误码、审计字段和示例请求 / 响应。

## 待确认问题

本轮无阻塞研发的待确认问题。以下问题均列为后续生产化或扩展，不影响本期默认最小口径进入 Review / Dev：

1. 是否需要 receiver 数据库表、Admin API 或 Admin UI。
2. 是否需要密钥轮换、双 secret 灰度、secret 过期时间和密钥加密存储。
3. 是否需要完整 IP allowlist 代理可信链、WAF、mTLS、OAuth2 或 JWT。
4. 是否需要复杂 JSONPath、数组映射、表达式、脚本、provider adapter 或模板库。
5. 是否需要 receiver 级 body size、限流、失败次数告警和指标。
6. 是否需要保存完整 raw payload 到安全审计存储或 DLQ。
7. 是否需要为多错误同时存在的请求定义全局响应优先级。

## Score

```text
需求清晰度        18/20
范围可控          18/20
风险可控          17/20
验收标准明确      19/20
架构一致性        19/20
合计              91/100
```

扣分说明：

- 本次已用默认最小协议关闭旧 BLOCK，路径、receiver、配置、映射、签名、来源、错误码、审计和验收均可测试。
- 仍扣分是因为 receiver 配置使用环境变量和受限映射 DSL，只适合本期最小接入；生产级密钥管理、复杂 provider 映射、代理 IP 可信链和管理后台需后续任务补齐。

## Decision

```text
PASS
```

理由：Review 文件无实际 P0/P1；上一轮 Analysis BLOCK 项已按默认最小可验收口径逐条关闭。需求边界、输入输出、业务规则、异常路径、非目标、风险、审计和验收标准已明确，Score >= 80，可交给 Review Agent 审核。
