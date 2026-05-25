# Analysis

## 需求 ID

2026-05-24-webhook-receiver

## Review 反馈处理

本轮已先读取 `tasks/2026-05-24-webhook-receiver/review.md`。

| Review 项 | 处理结果 |
| --- | --- |
| `P0` | `review.md` 仍为模板，占位为空，未列出实际 P0。无可逐条修正项。 |
| `P1` | `review.md` 仍为模板，占位为空，未列出实际 P1。无可逐条修正项。 |
| `P2` | `review.md` 仍为模板，占位为空，未列出实际 P2。无可逐条修正项。 |
| `Open Questions` | `review.md` 为“待填写”，未列出实际问题。无可逐条修正项。 |
| `Decision` | `review.md` 仍是 `PASS / BLOCK` 模板，不是有效审核结论；本轮不沿用该占位结论。 |

## Analysis 自检修正

本轮已读取上一版 `analysis.md`。上一版为 `Decision BLOCK`、`Score 78/100`，并列出 Webhook 专属协议缺口。本轮逐条处理如下。

| 上轮 BLOCK / 扣分项 / 待确认项 | 本轮处理 |
| --- | --- |
| Webhook Receiver 对外路径未定。 | 已重新读取 `task.md`、`requirements/backlog.json`、`docs/api.md`、`docs/architecture.md`、`docs/data-model.md`、`docs/operations.md`、`docs/roadmap.md` 和原始设计文档。现有材料只确认需要 Webhook Receiver 接入模型，未定义 URL。继续 BLOCK。 |
| receiver 标识来源未定。 | 未发现 path、query、header、域名或 token 绑定 receiver 的 Owner 决策。receiver 无法唯一定位配置。继续 BLOCK。 |
| receiver 配置来源未定。 | `docs/data-model.md` 没有 receiver 表；`docs/api.md` / `docs/operations.md` 只定义 OpenAPI Event token 配置，没有 Webhook receiver 配置、Admin API 或环境变量。继续 BLOCK。 |
| 是否新增 receiver 配置模型和管理 API 未定。 | 需求可以确认“必须存在配置来源”，但不能替 Owner 指定数据库表、静态文件、环境变量或 Admin API。继续 BLOCK。 |
| 映射 DSL / 映射表达方式未定。 | 原始设计只说明外部字段可通过 `attributes/facts/mapping/adapter` 进入规则引擎，并给出 Fact Provider `mapping` 示例；没有定义 Webhook payload 到标准事件的 JSONPath、模板、固定映射表或 adapter DSL。继续 BLOCK。 |
| 映射能力边界未定。 | 常量、默认值、类型转换、字段重命名、attributes 白名单、attributes 透传、缺字段处理均无 Owner 决策。本轮只能要求映射结果满足标准事件 Schema。继续 BLOCK。 |
| 签名算法和签名串未定。 | `task.md` 验收要求“签名/来源校验完整”，但未定义 HMAC 算法、签名串、摘要编码、raw body 参与方式。继续 BLOCK。 |
| 签名 header 未定。 | 未发现 timestamp header、signature header、provider event id header 的名称和格式。继续 BLOCK。 |
| receiver secret 粒度未定。 | 未定义每 receiver、每 tenant/source、每 provider 或全局 secret。继续 BLOCK。 |
| 重放窗口未定。 | 未定义 timestamp 允许偏差、时钟漂移处理、超窗错误码。继续 BLOCK。 |
| 来源校验依据未定。 | 未定义 IP allowlist、Host、header、Bearer token、receiver secret 或组合策略。继续 BLOCK。 |
| Webhook 专属安全 / 映射错误码未定。 | 现有 Event API 错误码可继承通用 JSON、事件校验、幂等、quota 语义，但签名失败、来源失败、receiver 不存在、映射失败缺少明确错误码。继续 BLOCK。 |
| 失败响应字段名不一致。 | `docs/api.md` 正文要求 `errorCode/message`，示例仍使用 `code/error`。需要 Owner 统一 Webhook 与标准事件接口响应字段名。继续作为 P2 风险。 |
| 审计 action / payload 未定。 | 可复用 `marketing_audit` 字段，但 Webhook 成功、重复、签名失败、来源失败、映射失败、receiver 禁用等 action 名和脱敏 payload 未定义。继续 BLOCK。 |
| `Idempotency-Key` header 是否允许未定。 | 标准事件 HTTP API 必填该 header；Webhook 是否也暴露该 header未决策。本轮保留“若允许，必须等于映射后标准事件派生值”的规则。仍需 Owner 决策。 |
| `occurredAt` 缺失是否可兜底未定。 | 标准事件要求 `occurredAt` 必填且带时区。缺少 Owner 新决策前，Webhook 映射后也必须必填，不允许用接收时间兜底。该项已按现有标准事件规则收口。 |
| 本期是否只支持 JSON 未定。 | 现有 JSON API 默认 `application/json`，任务只说 Webhook payload 映射。为最小口径建议只支持 JSON，但是否拒绝 XML、form-urlencoded、CloudEvents 仍需 Owner 确认。继续 BLOCK。 |
| receiver 级 body size 是否沿用 `1 MiB` 未定。 | 可从标准 Event API 推导默认建议为 `1 MiB`；若 receiver 需要独立限制或覆盖方式，仍需 Owner 决策。 |
| 当前只能明确能力和边界，不能形成唯一可验收接口契约。 | 本轮进一步把可继承规则、最小边界和不可推导项拆开。由于关键协议仍缺 Owner 决策，本轮继续 `Decision BLOCK`，不交给 Dev Agent。 |

结论：现有材料仍不能关闭上一轮 BLOCK。继续 `Decision BLOCK` 是必要结果，否则 Dev Agent 会被迫用实现假设替代需求决策。

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
- `/Users/tong/Documents/GitHub/OracleBetX-Projects/act-analysis/22-独立活动营销中台设计.md`

本轮未读取业务代码，未写代码，未改代码，未提交。

## 可推导与不可推导结论

| 需求点 | 现有材料 | 本轮口径 |
| --- | --- | --- |
| 接入目标 | `task.md` 和 backlog 明确“实现 SaaS 化 Webhook Receiver 的接入模型，支持外部系统将事件推给活动系统，统一转换为标准事件”。 | Webhook Receiver 是标准事件接入前置层，不是新活动引擎。 |
| 对外路径 | 原始设计只列出 `Webhook Receiver` 作为接入方式，未给 URL。 | 不可推导，需 Owner 决策。 |
| receiver 定位 | 无 path/query/header/domain 说明。 | 不可推导，需 Owner 决策。 |
| receiver 配置来源 | 现有数据模型无 receiver 表；现有配置只覆盖 OpenAPI Event token。 | 不可推导，需 Owner 决策。 |
| 标准事件模型 | 设计文档和 `docs/api.md` 均明确 8 个顶层字段、`subject`、`biz`、`occurredAt`、`attributes`。 | 映射后的标准事件必须严格满足现有 Schema。 |
| 幂等键 | 设计文档和 `docs/api.md` 明确 `tenantId:sourceSystem:eventType:bizType:bizId`。 | Webhook 映射后必须使用同一派生规则。 |
| 重复事件 | `docs/api.md` 明确完全重复返回 `200 OK duplicate=true`，同 key 不同 payload 返回 `INVALID_IDEMPOTENCY_KEY`。 | Webhook 复用相同语义。 |
| Content-Type | 标准 Event API 要求 `application/json`，接受 charset 参数。 | 最小建议 Webhook 只支持 JSON，但仍需 Owner 明确是否为本期范围。 |
| 请求体大小 | 标准 Event API 默认 `1 MiB`，可用 `EX_MARKETING_EVENT_MAX_BODY_BYTES` 覆盖。 | 最小建议沿用 `1 MiB`，receiver 独立限制需 Owner 决策。 |
| 签名 / 来源 | `task.md` 验收要求完整，但未定义算法、header、secret、来源策略。 | 不可推导，需 Owner 决策。 |
| 错误响应 | `docs/api.md` 正文写 `errorCode/message`，示例用 `code/error`。 | 必须稳定 JSON，字段名需 Owner 统一。 |
| 审计 | `marketing_audit` 有通用字段和部分 action。 | 可复用审计表，但 Webhook action 与脱敏 payload 不可推导。 |

## 背景目标

ExMarketing 是独立活动营销中台。外部业务系统应通过标准事件、Fact Query、Reward Command / Callback 等边界与营销系统交互，营销系统不内置订单、支付、会员、游戏流水等外部业务模型。

现有标准事件入口 `POST /openapi/v1/events` 已定义事件模型、幂等键、鉴权、Content-Type、body size、错误码、重复事件和审计语义。Webhook Receiver 的目标是在标准事件入口之前增加 SaaS 化接入层：外部系统可以推送自身 JSON Webhook payload，ExMarketing 根据 receiver 配置完成安全校验和字段映射，再把结果作为标准事件交给既有事件处理链路。

Webhook Receiver 不应绕过 event inbox、活动匹配、规则执行、quota、participation、grant_order、outbox 和 audit。

## 参与方

| 参与方 | 职责 |
| --- | --- |
| 外部 Webhook Provider | 按自身协议向 ExMarketing 推送原始事件 payload。 |
| Webhook Receiver | 接收请求，定位 receiver，完成来源校验、签名校验、格式校验、payload 映射、标准事件校验、失败响应和审计。 |
| 标准 Event Engine | 处理映射后的标准事件，执行幂等、活动匹配、规则、奖励、quota、participation、grant_order、outbox。 |
| 运营 / 运维 | 管理 receiver 配置、密钥、来源约束，排查接入失败和审计记录。 |
| Review / Test Agent | 在需求可实现后审核并设计测试。 |

## 需求说明

Webhook Receiver 应提供外部 Webhook 接收能力，将外部 JSON payload 转换为标准事件。

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

`subject` 只允许：

```text
type
id
```

`biz` 只允许：

```text
bizType
bizId
```

幂等键固定为：

```text
tenantId:sourceSystem:eventType:bizType:bizId
```

业务扩展字段只能进入 `attributes`。外部 provider event id、trace id、签名 timestamp 可用于追踪、`eventId` 或审计 payload，但不得替代标准幂等键。

## 输入输出

### Webhook 请求输入

| 输入 | 需求 |
| --- | --- |
| URL / receiver 标识 | 必须能唯一定位 receiver 配置。具体路径和标识来源未定义，属于阻塞 Owner 决策。 |
| HTTP headers | 用于签名、时间戳、来源、外部 trace、可选幂等 header。header 名和格式未定义，属于阻塞 Owner 决策。 |
| Raw body | 外部原始 payload。签名如依赖 body，必须使用原始字节而非重序列化 JSON。 |
| Content-Type | 最小建议仅接受 `application/json` 和带参数的 `application/json; charset=utf-8`。是否支持其他格式需 Owner 决策。 |
| Query 参数 | 默认不参与业务映射，除非 Owner 明确允许。 |

### Receiver 配置输入

receiver 配置至少需要表达：

- receiver code / id。
- enabled 状态。
- 允许的 tenantId 范围。
- 允许的 sourceSystem 范围。
- eventId、eventType、tenantId、sourceSystem、subject、biz、occurredAt、attributes 映射规则。
- attributes 白名单、透传、重命名或拒绝策略。
- 签名校验算法、签名串、header 名、secret 粒度、重放窗口。
- 来源校验策略。
- 请求体大小限制。
- 是否允许 `Idempotency-Key` header，以及与映射结果不一致时的错误语义。
- 审计 action、resource、resourceId 和脱敏 payload 规则。

配置来源未由 Owner 决策，不能假设为数据库表、静态文件、环境变量、Admin API 或内置示例 receiver。

### 成功输出

- 首次有效事件：返回 `duplicate=false`，并返回与标准事件接口一致的 participation、grant_order、outbox 摘要。
- 完全重复事件：返回 `200 OK` 且 `duplicate=true`，不得重复创建 participation、grant_order、outbox，不得重复扣 quota。
- 标准事件处理阶段 quota 不足：沿用现有 `409 QUOTA_EXCEEDED` 语义。

### 失败输出

失败响应必须是稳定 JSON，至少包含机器可识别错误码和可读 message。字段名应在 Owner 统一 `errorCode/message` 与 `code/error` 后与标准事件接口一致。

失败响应、日志、审计不得暴露完整 token、完整 Authorization header、完整签名、receiver secret、内部配置或敏感 payload。

## 业务规则

1. Receiver 必须先根据明确的 receiver 标识定位配置；无法定位时不得使用默认密钥、默认映射或兜底 receiver。
2. receiver 不存在或未启用时必须拒绝请求，不得进入标准事件处理链路。
3. 来源校验和签名校验必须发生在标准事件写入、活动匹配、participation、grant_order、outbox 之前。
4. 若签名算法使用 raw body，签名校验必须基于原始字节，不得用重序列化 JSON 结果参与验签。
5. 映射结果必须是完整标准事件；缺少必填字段、字段类型不匹配或出现未知顶层字段时必须拒绝。
6. `occurredAt` 必须为带时区的 RFC3339 时间；缺失时不允许用接收时间兜底，除非 Owner 明确改变标准事件规则。
7. 幂等键必须由映射后的标准事件字段派生，固定为 `tenantId:sourceSystem:eventType:bizType:bizId`。
8. 外部 provider event id 只能用于追踪或 `eventId`，不得作为唯一幂等主键。
9. 如果允许请求携带 `Idempotency-Key` header，header 必须与映射后的标准事件派生值一致；不一致时返回 `INVALID_IDEMPOTENCY_KEY`。
10. 完全相同 payload、相同幂等键重复推送必须按 duplicate 成功处理。
11. 相同幂等键但标准事件 payload 不同必须返回 `INVALID_IDEMPOTENCY_KEY`，不得进入发奖链路。
12. 映射成功后必须复用既有标准事件链路，不得新增绕过 event inbox、quota、participation、grant_order 或 outbox 的发奖逻辑。
13. 成功、重复、安全失败、映射失败、标准事件校验失败和 quota 失败都必须可审计。
14. Webhook Receiver 不得连接外部业务数据库，不得引用外部业务系统 SDK，不得内置某个 provider 的业务模型。

## 边界条件

- 初期最小建议只支持 JSON Webhook；XML、form-urlencoded、CloudEvents、provider 特定二进制格式不纳入，除非 Owner 明确纳入。
- 请求体大小建议与 OpenAPI Event API 默认 `1 MiB` 对齐；receiver 级独立限制或覆盖方式需 Owner 决策。
- JSON number、金额和 Decimal 语义与现有标准事件处理保持一致；金额计算仍由奖励计算链路处理。
- 映射后的 `tenantId` 必须在 receiver 允许范围内。
- 映射后的 `sourceSystem` 必须在 receiver 允许范围内。
- `attributes` 可以为空对象 `{}`，但不能缺失。
- 高并发重复 Webhook 必须依赖既有幂等和持久化状态兜底，不得重复发奖、重复扣 quota 或重复发送 outbox。
- 审计写入失败不能把安全失败吞掉；调用方必须得到明确失败响应。

## 异常场景

| 场景 | 期望行为 |
| --- | --- |
| receiver 标识缺失、重复或不存在 | 拒绝请求，不处理事件，记录可排查审计。错误码需 Owner 定义。 |
| receiver 已禁用 | 拒绝请求，不处理事件，记录可排查审计。错误码需 Owner 定义。 |
| Content-Type 缺失或不支持 | 返回 `UNSUPPORTED_MEDIA_TYPE` 类错误。 |
| body 超限 | 返回 `REQUEST_TOO_LARGE` 类错误。 |
| JSON 不合法 | 返回 `INVALID_JSON` 类错误。 |
| 签名 header 缺失、格式错误、时间戳超窗、摘要不匹配 | 返回签名失败类错误，不写 event inbox、participation、grant_order 或 outbox。具体错误码待 Owner 决策。 |
| 来源 IP、host、header 或 token 不被允许 | 返回来源失败类错误，不写 event inbox、participation、grant_order 或 outbox。具体错误码待 Owner 决策。 |
| 映射路径不存在、类型不符、必填字段为空 | 返回映射失败类错误，不写标准事件。具体错误码待 Owner 决策。 |
| 映射后标准事件字段非法 | 返回 `INVALID_EVENT`。 |
| `Idempotency-Key` 与映射结果不一致 | 返回 `INVALID_IDEMPOTENCY_KEY`。 |
| 同幂等键不同 payload | 返回 `INVALID_IDEMPOTENCY_KEY`。 |
| 完全重复 payload | 返回 `200 OK duplicate=true`，不重复发奖。 |
| quota 不足 | 沿用现有 `409 QUOTA_EXCEEDED`。 |
| 审计记录失败 | 不得伪装成功；返回明确内部错误或审计失败类错误，且不得泄露敏感信息。 |

## 非目标范围

- 不实现 MQ Event In。
- 不改变现有 `/openapi/v1/events` 标准事件接口语义。
- 不改变标准事件字段、幂等键格式、重复事件语义或 quota 语义。
- 不改变活动匹配、规则执行、奖励计算、participation、grant_order、outbox 或 DLQ 业务规则。
- 不实现外部业务数据库查询。
- 不接入外部业务系统 SDK。
- 不为单个 provider 写死业务字段、专用活动逻辑或专用发奖逻辑。
- 不实现完整 Admin 配置 UI，除非 Owner 明确纳入本期。
- 不实现 XML、form-urlencoded、CloudEvents 等多格式全量兼容，除非 Owner 明确纳入本期。
- 不实现生产级指标告警、密钥轮换、WAF、mTLS 或 OAuth2，除非 Owner 另开任务或明确纳入。

## 风险

| 风险 | 级别 | 说明 |
| --- | --- | --- |
| 映射层绕过标准事件处理 | P0 | 会导致重复发奖、跨 tenant 污染或 quota/outbox 不一致。需求已明确禁止。 |
| Webhook 对外路径未定 | P1 | Dev 无法确定 API 契约、测试 URL 和 receiver 定位方式。 |
| receiver 配置来源未定 | P1 | 无法确定是否需要数据模型、Admin API、静态配置、环境变量或内置示例。 |
| 映射规则未定 | P1 | 无法验收外部 payload 到标准事件的转换能力。 |
| 签名算法和 header 未定 | P1 | 安全边界不可审核，验签测试不可编写。 |
| 来源校验规则未定 | P1 | “来源校验完整”无法落地验收。 |
| 默认生成 bizId、eventId 或 occurredAt | P1 | 可能破坏幂等、活动时间判断和重放语义。需求已明确禁止 `occurredAt` 兜底。 |
| 失败响应字段名口径存在差异 | P2 | 文档正文与示例不一致，会导致测试和调用方契约不一致。 |
| 审计 action / payload 未定 | P2 | 失败排查和合规口径会不一致。 |

## 验收标准

以下验收标准中的 Webhook 专属协议项必须先由 Owner 决策补齐，才能进入 Dev。

1. 存在明确的 Webhook Receiver 对外路径和 receiver 定位方式。
2. 存在明确的 receiver 配置来源，且能配置至少一个启用 receiver。
3. 存在明确的映射规则表达方式和能力边界。
4. 一个合法外部 JSON Webhook payload 能按配置映射为完整标准事件。
5. 映射后的标准事件只包含允许字段，且幂等键固定为 `tenantId:sourceSystem:eventType:bizType:bizId`。
6. 合法 Webhook 首次推送进入既有事件处理链路，并可生成与标准事件接口一致的 participation、grant_order、outbox 结果。
7. 相同 Webhook payload 重复推送返回 `duplicate=true`，且不会重复创建 participation、grant_order、outbox 或重复扣 quota。
8. 同幂等键不同 payload 返回 `INVALID_IDEMPOTENCY_KEY`，不进入发奖链路。
9. 签名缺失、签名错误、时间戳超窗或来源不允许时请求失败，且不会写 event inbox、participation、grant_order 或 outbox。
10. receiver 不存在、receiver 禁用、映射缺失必填字段、类型不匹配、时间格式非法时请求失败，并返回稳定错误码。
11. 请求体过大和 Content-Type 不支持时返回明确错误。
12. 成功、重复、签名失败、来源失败、映射失败、标准事件校验失败、quota 失败均可通过审计排查。
13. 错误响应、日志和审计不得暴露完整 token、完整 Authorization header、完整签名、secret、内部配置或敏感 payload。
14. 现有 `/openapi/v1/events` 行为保持不变。
15. 文档说明 receiver 配置、签名规则、来源校验、映射规则、错误码、审计字段和示例请求 / 响应。

## 待确认问题

### 阻塞研发的 Owner 决策

1. Webhook Receiver 的对外路径是什么？例如 `POST /openapi/v1/webhooks/{receiverCode}`，还是按 tenant/provider 分路径。
2. receiver 标识来自 path、query、header 还是域名？缺失、重复或冲突时如何处理？
3. receiver 配置来源是什么？数据库、静态配置文件、环境变量、Admin API，还是本期只做内置示例 receiver？
4. 本期是否需要新增 receiver 配置数据模型和管理 API？
5. 映射规则采用什么表达方式？JSONPath、固定字段映射表、模板表达式，还是 provider adapter 配置？
6. 映射是否支持常量、默认值、类型转换、字段重命名、attributes 白名单和 attributes 透传？
7. 签名算法和签名串是什么？例如 HMAC-SHA256 over `timestamp + rawBody`。
8. 签名相关 header 名称是什么？例如 timestamp header、signature header、provider event id header。
9. receiver secret 的粒度是什么？每 receiver 一个 secret，还是每 tenant/source/provider 一个 secret？
10. 重放窗口是多少？时间戳超窗返回什么错误码？
11. 来源校验依据是什么？IP allowlist、Host、特定 header、Bearer token、receiver secret，还是组合校验？
12. Webhook 专属安全失败错误码是什么？是否新增 `WEBHOOK_SIGNATURE_INVALID`、`WEBHOOK_SOURCE_FORBIDDEN`、`WEBHOOK_RECEIVER_NOT_FOUND`、`WEBHOOK_MAPPING_FAILED` 等错误码？
13. Webhook 失败响应字段名应使用 `errorCode/message` 还是沿用现有示例中的 `code/error`？
14. 审计 action、resource、resourceId 和 payload 字段如何定义？是否保存 raw payload 摘要、signature 摘要、receiverCode、providerEventId？
15. 是否允许请求携带 `Idempotency-Key` header？若允许，与映射结果不一致时是否直接返回 `INVALID_IDEMPOTENCY_KEY`？
16. 本期是否只支持 JSON Webhook？是否明确不支持 XML、form-urlencoded 和 CloudEvents？
17. receiver 级请求体大小限制是否沿用 OpenAPI Event API 默认 `1 MiB`？是否允许独立覆盖？

### 已按现有材料收口的问题

1. 映射后的 `occurredAt` 必须必填且带时区；无 Owner 新决策前不允许接收时间兜底。
2. Webhook 映射后的幂等键必须使用标准事件字段派生，不允许用 provider event id 替代。
3. Webhook 映射成功后必须复用既有标准事件链路，不允许直接发奖。

### 可后续优化的问题

1. 是否需要 receiver 级限流、失败次数告警和指标。
2. 是否需要密钥轮换、双 secret 过渡和 secret 过期时间。
3. 是否需要 Admin UI 管理 receiver 配置。
4. 是否需要 provider 模板库或示例配置。
5. 是否需要 Webhook DLQ 或失败重放后台。

## Score

```text
需求清晰度        13/20
范围可控          17/20
风险可控          13/20
验收标准明确      16/20
架构一致性        19/20
合计              78/100
```

扣分原因：

- Owner 仍未补齐 Webhook 对外路径、receiver 定位方式、receiver 配置来源、映射规则、签名算法/header、来源校验、错误码和审计字段。
- 既有标准事件、幂等、错误格式和审计表可以继承，但 Webhook 专属接入协议仍不可唯一实现和验收。
- 当前需求可以明确架构边界和安全原则，但不能交给 Dev Agent 直接开发，否则会迫使 Dev 用实现假设替代 Owner 决策。

## Decision

```text
BLOCK
```

BLOCK 原因：`task.md`、backlog、PRD / 设计文档和现有 API / 数据字典仍没有 Owner 补充决策，无法关闭上一轮 BLOCK。Webhook Receiver 的关键协议和配置决策缺失，至少需要补齐对外路径、receiver 定位方式、receiver 配置来源、映射规则、签名/来源校验、错误码和审计字段后，Analysis Agent 才能给出 `Score >= 80` 且 `Decision PASS` 的可研发版本。
