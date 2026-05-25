# Analysis

## 需求 ID

2026-05-24-http-event-api

## Review 反馈处理

本轮已先读取 `tasks/2026-05-24-http-event-api/review.md`。Review 结论为 `PASS`，Score `90 / 100`，无 P0/P1，无阻塞性 Open Questions；但有 2 条 P2 需要在 Analysis 文档中收口，避免 Dev 阶段扩大范围或误测组合错误优先级。

| Review 反馈 | 本轮处理 |
| --- | --- |
| P2：analysis.md 将标准事件“只允许 8 个顶层字段”、`subject` / `biz` 未知字段拒绝、`occurredAt` 必须带时区、`Idempotency-Key` 精确格式、重复 payload 处理等既有模型约束写入本期验收，存在被理解为本期新增业务语义的风险。 | 已明确这些规则属于既有标准事件模型和幂等收件箱的回归验收，本期不得修改其业务语义；Dev 只需确保新增鉴权、大小限制、Content-Type、错误响应不会破坏既有规则。 |
| P2：analysis.md 的校验顺序把 Content-Type 放在 Authorization 之前，但 task.md 未明确“同时缺少/错误 Content-Type 与缺少 Authorization”时的优先级。 | 已改为“边界校验集合”而非强制全局响应优先级；验收与测试应固定单一失败原因。多个错误同时存在的组合场景不作为本期阻塞验收，除非 Owner 后续补充明确优先级。 |
| Open Questions | Review 明确无阻塞性 Open Questions；本轮不新增 Decision BLOCK。 |

## Analysis 自检修正

本轮已读取上一版 `analysis.md`。上一版文件 Score `95 / 100`、Decision `PASS`，但其“Review 反馈处理”仍描述旧模板占位，与最新 `review.md` 不一致；同时 Review P2 指出本期范围存在轻微膨胀风险。

本轮修正如下：

| 自检项 | 修正 |
| --- | --- |
| 旧 Analysis 曾因 Bearer 凭证来源、tenant/sourceSystem 授权、生产强制鉴权、body size、Content-Type、错误响应格式缺 Owner 决策而 BLOCK。 | 已重新读取 `task.md`，确认 Owner 补充决策已覆盖上述问题；按 Owner 决策关闭历史 BLOCK。 |
| 上一版误称 `review.md` 仍是模板占位。 | 已更新为最新 Review 结论：PASS、无 P0/P1、2 条 P2。 |
| 既有事件模型和幂等规则写得像本期新增需求。 | 已标注为“既有规则回归验收”，本期非目标明确包含“不修改标准事件模型、幂等键格式或重复处理语义”。 |
| 校验顺序可能被理解为组合错误响应优先级。 | 已改为本期需要覆盖的边界校验集合，并注明单一失败原因用于验收，组合失败优先级不阻塞。 |

本轮读取范围：`task.md`、`review.md`、上一版 `analysis.md`、`requirements/backlog.json`、`README.md`、`CLAUDE.md`、`docs/api.md`、`docs/architecture.md`、`docs/data-model.md`、`docs/operations.md`、`docs/roadmap.md`、`docs/workflow.md`、`/Users/tong/Documents/GitHub/OracleBetX-Projects/act-analysis/22-独立活动营销中台设计.md`。本轮未读取实现代码、未写代码、未提交、未设计具体代码改法。

## 需求说明

本需求为 `POST /openapi/v1/events` 补齐 HTTP 接入安全边界。该接口是外部业务系统向 ExMarketing 上报标准事件的入口；事件通过接入后，会进入 event inbox、幂等处理、活动匹配、规则判断、奖励计算、Quota、participation、grant_order 和 outbox 链路。

本期新增和明确的需求范围是：

1. Bearer Token 鉴权。
2. token 对 `tenantId` 的授权校验。
3. token 对 `sourceSystem` 的授权校验。
4. 请求 body 最大大小限制。
5. JSON `Content-Type` 要求。
6. 401 / 403 / 413 / 415 / 400 失败响应的稳定 JSON 格式。
7. 生产强制鉴权与本地开发无鉴权开关。
8. 文档和测试同步。

标准事件字段、幂等键格式、重复事件处理、活动匹配、规则执行、奖励计算、Quota、grant_order、outbox、reward callback、Fact Query、MQ Event In 和 Webhook Receiver 的业务语义不在本期修改范围内。涉及这些既有能力的验收只作为回归保护。

## 背景目标

上游设计文档要求 ExMarketing 作为独立活动营销中台，不依赖具体业务系统，不查外部业务库，不写外部业务库，只接收标准事件并输出标准奖励指令。HTTP Event In 示例已包含 `Authorization: Bearer xxx`，路线图也将 OpenAPI 鉴权列为生产化前必须补齐的安全能力。

本期目标：

1. 未授权请求必须被拒绝，不得写入 event inbox 或进入活动引擎。
2. 合法 token 只能上报授权范围内的 `tenantId` 和 `sourceSystem`。
3. 请求 body 默认最大 `1 MiB`，可通过环境变量覆盖。
4. Event API 必须要求 JSON Content-Type。
5. 所有失败响应必须稳定、可机器识别，且不得泄漏凭证、内部配置或敏感 payload。
6. 本地开发可显式关闭鉴权，生产部署必须强制鉴权。
7. API 文档和运维文档必须同步说明鉴权、授权、大小限制和错误响应。

## 参与方

| 参与方 | 职责 |
| --- | --- |
| 外部业务系统 / 接入方 | 调用 `POST /openapi/v1/events`，提供 `Content-Type`、`Authorization`、`Idempotency-Key` 和标准事件 JSON。 |
| ExMarketing OpenAPI Event In | 在事件入库和业务处理前完成请求大小、Content-Type、Bearer token、tenant/sourceSystem 授权、JSON、事件模型和幂等相关边界校验。 |
| ExMarketing Engine | 只处理已通过 OpenAPI 接入边界的事件，沿用既有活动匹配、规则、奖励、Quota、grant_order 和 outbox 流程。 |
| 运维 / 部署方 | 配置 `EX_MARKETING_OPENAPI_TOKENS`、`EX_MARKETING_REQUIRE_OPENAPI_AUTH` 和可选 `EX_MARKETING_EVENT_MAX_BODY_BYTES`。 |
| Review / Test Agent | 审核鉴权、租户隔离、日志脱敏、失败路径和回归测试覆盖。 |

## 输入输出

### 配置输入

`EX_MARKETING_OPENAPI_TOKENS`：

```text
token:tenantId[,tenantId...]:sourceSystem[,sourceSystem...];token2:tenantId2:sourceSystem2
```

规则：

- token 不能为空。
- tenantId 不能为空。
- sourceSystem 不能为空。
- 多个 token 可并存。
- 一个 token 可授权多个 tenant 和多个 sourceSystem。
- 每个 token 的授权独立生效，不得把不同 token 的 tenant/sourceSystem 授权合并。
- 本期不支持 token 过期、轮换 API、禁用 API、数据库持久化或凭证管理后台。
- 日志、错误响应和审计不得输出完整 token 或完整 Authorization header。

`EX_MARKETING_REQUIRE_OPENAPI_AUTH`：

- 为 `true` 时，必须配置至少一个有效 token，否则服务启动失败。
- 为 `true` 时，所有 `/openapi/v1/events` 请求必须通过 Bearer 鉴权和 tenant/sourceSystem 授权。
- 未设置或为 `false` 时，允许本地开发无鉴权，但必须在日志中提示当前为开发模式。
- 生产部署文档必须要求设置为 `true`。

`EX_MARKETING_EVENT_MAX_BODY_BYTES`：

- 可选，覆盖默认请求 body 最大大小。
- 未设置时默认 `1 MiB`。
- 超过限制返回 `413 REQUEST_TOO_LARGE`。

### HTTP 请求

```http
POST /openapi/v1/events
Content-Type: application/json
Authorization: Bearer <access-token>
Idempotency-Key: <tenantId>:<sourceSystem>:<eventType>:<bizType>:<bizId>
```

JSON body 沿用既有标准事件模型：

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

字段边界：

| 字段 | 本期要求 |
| --- | --- |
| `sourceSystem` | 必填 non-empty string，必须在 token 授权范围内。 |
| `tenantId` | 必填 non-empty string，必须在 token 授权范围内。 |
| 其他标准事件字段 | 沿用既有标准事件模型校验；本期只做回归保护，不改变字段语义。 |

`Idempotency-Key` 沿用既有规则，必填且等于：

```text
tenantId + ":" + sourceSystem + ":" + eventType + ":" + biz.bizType + ":" + biz.bizId
```

### 成功输出

合法且非重复事件沿用现有事件处理响应语义，返回是否重复、participations、grantOrders 和 outboxMessages。

完全重复事件仍沿用既有幂等语义，返回 `200 OK` 且 `duplicate=true`，不得重复生成 participation、grant_order 或 outbox。此项为回归验收，不是本期新增重复处理规则。

### 错误输出

所有失败响应必须是 JSON object：

```json
{
  "errorCode": "UNAUTHORIZED",
  "message": "unauthorized"
}
```

错误码要求：

| HTTP | errorCode | 场景 |
| --- | --- | --- |
| 400 | `INVALID_JSON` | JSON 不可解析。 |
| 400 | `INVALID_EVENT` | 标准事件字段缺失或类型错误；其他标准事件模型错误沿用既有语义。 |
| 400 | `INVALID_IDEMPOTENCY_KEY` | 缺少 `Idempotency-Key`、Header 与 payload 不匹配、同 key payload 不一致；属于既有幂等规则回归。 |
| 401 | `UNAUTHORIZED` | 缺少 `Authorization`、不是 Bearer 格式、Bearer token 为空、token 不存在或不可用。 |
| 403 | `TENANT_FORBIDDEN` | token 存在，但无权上报 body 中的 `tenantId`。 |
| 403 | `SOURCE_FORBIDDEN` | token 存在，但无权上报 body 中的 `sourceSystem`。 |
| 413 | `REQUEST_TOO_LARGE` | 请求 body 超过 `1 MiB` 或 `EX_MARKETING_EVENT_MAX_BODY_BYTES` 配置值。 |
| 415 | `UNSUPPORTED_MEDIA_TYPE` | 缺少 `Content-Type` 或不是 `application/json`。 |
| 409 | `QUOTA_EXCEEDED` | 事件处理阶段预算、库存或次数不足，沿用现有 quota 响应语义。 |
| 500 | `INTERNAL_ERROR` | 非预期内部错误，不得泄露内部敏感细节。 |

错误响应不得包含完整 token、完整 Authorization header、内部配置、token 授权列表或敏感 payload。

## 业务规则

### 接入边界校验

请求进入 event inbox 或活动引擎之前，必须覆盖以下边界校验：

1. 请求 body 大小限制。
2. JSON `Content-Type` 校验。
3. 强制鉴权模式下的 `Authorization: Bearer <token>` 存在性、格式和 token 可用性校验。
4. JSON decode；语法错误返回 `INVALID_JSON`。
5. 标准事件模型校验；缺字段或类型错误返回 `INVALID_EVENT`。
6. token 对 body `tenantId` 的授权校验。
7. token 对 body `sourceSystem` 的授权校验。
8. `Idempotency-Key` 与 payload 一致性校验。
9. 事件幂等处理和现有业务流程。

说明：

- 上述列表定义本期必须覆盖的校验集合，不定义所有组合错误场景的响应优先级。
- 测试与验收应固定单一失败原因，例如“只缺 Authorization”“只缺 Content-Type”“只传非法 JSON”。
- 同时缺少或错误 `Content-Type` 与 `Authorization` 等组合场景，不作为本期阻塞验收；若后续需要稳定优先级，需由 Owner 另行补充决策。
- `401`、`403`、`413`、`415`、`INVALID_JSON`、`INVALID_EVENT`、`INVALID_IDEMPOTENCY_KEY` 均不得创建成功类业务数据。

### 鉴权与授权

1. `EX_MARKETING_REQUIRE_OPENAPI_AUTH=true` 时，`POST /openapi/v1/events` 必须要求 `Authorization: Bearer <token>`。
2. 缺少 Authorization、非 Bearer、空 token、未知 token 或不可用 token 返回 `401 UNAUTHORIZED`。
3. `401` 不得区分 token 不存在、格式正确但未知、不可用等细节。
4. token 存在但不授权 body `tenantId` 返回 `403 TENANT_FORBIDDEN`。
5. token 存在但不授权 body `sourceSystem` 返回 `403 SOURCE_FORBIDDEN`。
6. `403` 不得返回该 token 实际可访问的 tenant 或 sourceSystem 列表。
7. 鉴权或授权失败不得写入 `marketing_event_inbox`，不得进入活动引擎，不得创建 `marketing_participation`、`marketing_grant_order`、`marketing_outbox`。
8. 本地开发模式允许无鉴权，但必须通过 `EX_MARKETING_REQUIRE_OPENAPI_AUTH` 未设置或为 `false` 显式进入，并输出开发模式日志提示；生产部署文档必须要求强制鉴权。

### 请求大小与 Content-Type

1. Event API 请求 body 默认最大 `1 MiB`。
2. `EX_MARKETING_EVENT_MAX_BODY_BYTES` 可覆盖默认值。
3. 超过限制返回 `413 REQUEST_TOO_LARGE`。
4. 超大 body 不得进入完整 JSON decode、事件校验、inbox 写入或业务处理。
5. 缺少 `Content-Type` 或不是 `application/json` 返回 `415 UNSUPPORTED_MEDIA_TYPE`。
6. `application/json; charset=utf-8` 属于 JSON media type，应作为合法 JSON Content-Type 接受。

### 标准事件与幂等回归

以下内容属于既有标准事件模型和幂等规则的回归验收，本期不得改变其业务语义：

1. 标准事件字段集合和字段类型。
2. 业务扩展字段通过 `attributes` 承载。
3. `occurredAt` 时间格式要求。
4. `eventId` 用于追踪，不作为唯一幂等主键。
5. `Idempotency-Key` 必填且必须与 payload 计算值一致。
6. 完全重复事件返回 `200 OK duplicate=true`。
7. 相同 `Idempotency-Key` 但 payload 不一致返回 `400 INVALID_IDEMPOTENCY_KEY`。

Dev 不应将上述条目理解为本期新增模型设计或重做幂等处理；本期只要求新增接入安全边界后，这些既有能力不回退。

### 审计与日志

1. 成功接收事件沿用现有 `EVENT_IN` 审计语义。
2. `QUOTA_EXCEEDED` 沿用现有审计语义。
3. 本期不要求新增完整安全审计模型或凭证管理审计。
4. 所有日志、错误响应和审计都不得输出完整 token、完整 Authorization header、secret、内部配置值或敏感 payload。
5. 开发无鉴权模式必须有明确日志提示，便于生产前检查发现错误配置。

## 边界条件

| 场景 | 预期 |
| --- | --- |
| `EX_MARKETING_REQUIRE_OPENAPI_AUTH=true` 且没有有效 token 配置 | 服务启动失败。 |
| `EX_MARKETING_REQUIRE_OPENAPI_AUTH` 未设置或为 `false` | 本地开发允许无鉴权，日志提示开发模式。 |
| 无 `Authorization`，且其他输入合法 | 强制鉴权模式下返回 `401 UNAUTHORIZED`，无业务写入。 |
| `Authorization: Basic xxx`，且其他输入合法 | `401 UNAUTHORIZED`，无业务写入。 |
| `Authorization: Bearer` 但 token 为空，且其他输入合法 | `401 UNAUTHORIZED`，无业务写入。 |
| token 未配置或不可用，且其他输入合法 | `401 UNAUTHORIZED`，不区分细节，无业务写入。 |
| token 有效但 body `tenantId` 不在授权范围，且其他输入合法 | `403 TENANT_FORBIDDEN`，无业务写入。 |
| token 有效但 body `sourceSystem` 不在授权范围，且其他输入合法 | `403 SOURCE_FORBIDDEN`，无业务写入。 |
| body 缺少 `tenantId`，且鉴权和 Content-Type 合法 | `400 INVALID_EVENT`。 |
| body 缺少 `sourceSystem`，且鉴权和 Content-Type 合法 | `400 INVALID_EVENT`。 |
| body `tenantId` 或 `sourceSystem` 与 `Idempotency-Key` 不一致，且鉴权和 Content-Type 合法 | `400 INVALID_IDEMPOTENCY_KEY`。 |
| 缺少 `Idempotency-Key`，且鉴权和 Content-Type 合法 | `400 INVALID_IDEMPOTENCY_KEY`。 |
| 相同 `Idempotency-Key`、相同 payload 重复请求 | `200 OK duplicate=true`，不重复发奖。 |
| 相同 `Idempotency-Key`、不同 payload | `400 INVALID_IDEMPOTENCY_KEY`。 |
| body 超过 `1 MiB` 或配置上限 | `413 REQUEST_TOO_LARGE`，不进入完整 JSON decode 或业务处理。 |
| 缺少 Content-Type，且其他输入合法 | `415 UNSUPPORTED_MEDIA_TYPE`。 |
| Content-Type 不是 JSON，且其他输入合法 | `415 UNSUPPORTED_MEDIA_TYPE`。 |
| JSON 语法错误，且鉴权和 Content-Type 合法 | `400 INVALID_JSON`，不泄露内部栈。 |
| 标准事件缺字段或类型错误，且鉴权和 Content-Type 合法 | `400 INVALID_EVENT`。 |
| 合法 token + 合法 tenant + 合法 sourceSystem + 合法事件 | 正常接收并沿用现有处理流程。 |
| 合法事件但 quota 不足 | `409 QUOTA_EXCEEDED`，沿用现有 quota 响应和审计。 |
| 同一请求同时存在多个错误 | 本期不规定响应优先级，不作为阻塞验收；测试应拆成单一失败原因。 |

## 异常场景

1. token 配置项中 token、tenantId 或 sourceSystem 为空时，应视为无效凭证配置；强制鉴权模式下不得静默开放接口。
2. 多 token 并存时，每个 token 的 tenant/sourceSystem 授权独立生效，不得跨 token 合并授权。
3. 多 tenant 或多 sourceSystem token 只允许上报授权集合内的组合；未授权的 tenant 或 sourceSystem 返回对应 `403`。
4. 超大 body 必须被拒绝，避免完整读取或完整 JSON decode 后才返回。
5. 安全错误枚举必须受控：`401` 不暴露 token 是否存在，`403` 不暴露授权列表。
6. 文档示例必须补充 Authorization、生产强制鉴权环境变量和 body size limit，避免接入方继续按无鉴权示例调用。
7. 组合错误场景未由 Owner 定义优先级，不得作为 Dev 或 Test 阻塞项。

## 非目标范围

本需求不包含：

- Kafka / RabbitMQ / Pulsar 等 MQ Event In。
- Webhook Receiver SaaS 化接入。
- Fact Query 鉴权改造。
- Reward callback 签名或 token 校验。
- Admin API 鉴权。
- 接入方管理后台、token 自助签发、凭证 UI。
- token 数据库存储、禁用 API、轮换 API、过期策略。
- OAuth2、JWT、mTLS、HMAC 签名等完整认证体系。
- 修改标准事件字段、字段未知规则、时间格式要求、幂等键格式、重复事件处理或事件处理业务语义。
- 修改活动匹配、规则执行、奖励计算、Quota、grant_order、outbox 的业务流程。
- 生产网关、WAF、全局限流、IP allowlist、指标告警的完整方案。
- 新增完整安全审计模型。
- 定义多个请求错误同时存在时的全局响应优先级。

## 风险

| 风险 | 等级 | 说明 | 缓解 |
| --- | --- | --- | --- |
| 环境变量 token 明文配置 | P2 | 本期不做凭证后台或散列存储，部署侧需要保护环境变量。 | 文档要求不得输出 token，并把完整凭证管理列为后续任务。 |
| 开发模式无鉴权误用于生产 | P1 | `EX_MARKETING_REQUIRE_OPENAPI_AUTH` 未设置或为 `false` 时允许无鉴权。 | 生产文档必须要求 `true`；开发模式必须日志提示；`true` 且无 token 启动失败。 |
| 错误格式从既有 `code/error` 变为 `errorCode/message` | P2 | 既有 API 文档示例使用 `code/error`，本任务要求所有失败响应使用 `errorCode/message`。 | Dev 需同步更新 `README.md`、`docs/api.md` 和测试，统一 Event API 失败格式。 |
| 多 tenant/sourceSystem 授权配置错误 | P2 | 运维配置错误可能导致合法接入方被拒绝或授权过宽。 | 配置格式必须可验证；日志不能泄漏 token；测试覆盖授权拒绝和通过路径。 |
| 既有事件模型/幂等规则被误解为本期新增改造 | P2 | Review 已指出范围膨胀风险。 | 本文已标注为回归验收和非目标，不要求 Dev 重做模型或幂等业务语义。 |
| 组合错误优先级不明确 | P2 | Owner 未定义同一请求同时缺 Content-Type 和 Authorization 等场景的响应优先级。 | 本期测试固定单一失败原因；组合失败不作为阻塞验收。 |
| body size 配置过大 | P2 | 过大配置会削弱资源保护。 | 默认 `1 MiB`，生产建议保守配置并纳入运维文档。 |

## 验收标准

1. `EX_MARKETING_REQUIRE_OPENAPI_AUTH=true` 且未配置任何有效 token 时，服务启动失败。
2. 未设置或设置 `EX_MARKETING_REQUIRE_OPENAPI_AUTH=false` 时，本地开发允许无鉴权调用，并输出开发模式日志提示。
3. 在强制鉴权模式下，缺少 `Authorization` 且其他输入合法时，`POST /openapi/v1/events` 返回 `401 UNAUTHORIZED`，响应为 `errorCode/message` JSON，且不创建 inbox、participation、grant_order、outbox。
4. 非 Bearer Authorization 且其他输入合法时返回 `401 UNAUTHORIZED`。
5. Bearer token 为空或未知 token 且其他输入合法时返回 `401 UNAUTHORIZED`，错误信息不区分具体凭证状态。
6. token 有效但无权访问 body `tenantId`，且其他输入合法时返回 `403 TENANT_FORBIDDEN`，不进入 inbox 或活动引擎。
7. token 有效但无权访问 body `sourceSystem`，且其他输入合法时返回 `403 SOURCE_FORBIDDEN`，不进入 inbox 或活动引擎。
8. 合法 token、tenantId、sourceSystem、标准事件和 `Idempotency-Key` 可正常接收并沿用现有处理流程。
9. body 缺少 `tenantId` 或 `sourceSystem`，且鉴权和 Content-Type 合法时返回 `400 INVALID_EVENT`。
10. `Idempotency-Key` 缺失或与 payload 不匹配，且鉴权和 Content-Type 合法时返回 `400 INVALID_IDEMPOTENCY_KEY`；这是既有幂等规则回归。
11. 完全重复事件返回 `200 OK duplicate=true`，不重复创建 participation、grant_order 或 outbox；这是既有幂等规则回归。
12. 相同 key 不同 payload 返回 `400 INVALID_IDEMPOTENCY_KEY`；这是既有幂等规则回归。
13. 请求超过 `1 MiB` 或 `EX_MARKETING_EVENT_MAX_BODY_BYTES` 配置值返回 `413 REQUEST_TOO_LARGE`，且不进入完整 JSON decode 或业务处理。
14. 缺少 Content-Type 或非 JSON Content-Type，且其他输入合法时返回 `415 UNSUPPORTED_MEDIA_TYPE`。
15. JSON 不可解析，且鉴权和 Content-Type 合法时返回 `400 INVALID_JSON`。
16. 标准事件缺字段或类型错误，且鉴权和 Content-Type 合法时返回 `400 INVALID_EVENT`；其他既有标准事件模型规则只做回归保护，不作为本期新增语义。
17. 所有失败响应均为 JSON object，包含稳定 `errorCode` 和 `message`。
18. 错误响应、审计和日志不得暴露完整 token、完整 Authorization header、secret、内部配置或 token 授权列表。
19. `README.md`、`docs/api.md`、`docs/operations.md` 需要同步说明 Authorization、`EX_MARKETING_OPENAPI_TOKENS`、`EX_MARKETING_REQUIRE_OPENAPI_AUTH`、`EX_MARKETING_EVENT_MAX_BODY_BYTES` 和生产强制鉴权要求。
20. 测试覆盖成功路径、未授权、tenant forbidden、source forbidden、请求过大、Content-Type、非法 JSON、非法事件和幂等失败回归路径。
21. 测试用例应以单一失败原因构造；多个错误同时存在时的响应优先级不作为本期阻塞验收。

## 待确认问题

本轮无阻塞性待确认问题。

后续生产化可另开任务确认：凭证散列存储、后台签发、禁用、轮换、过期、安全审计、指标告警、网关/WAF/IP allowlist，以及多个请求错误同时存在时的全局响应优先级。上述内容已明确为本期非目标，不阻塞本任务进入 Review / Dev。

## 评分

| 项 | 分数 | 说明 |
| --- | ---: | --- |
| 需求清晰度 | 19 / 20 | Owner 已明确凭证来源、授权模型、生产策略、size limit、Content-Type 和错误格式；组合错误优先级明确为非阻塞。 |
| 范围可控 | 20 / 20 | 已按 Review P2 将标准事件和幂等规则收口为回归验收，不要求本期重做模型语义。 |
| 风险可控 | 18 / 20 | 主要风险集中在环境变量凭证、开发模式误用生产和组合错误优先级，已有缓解。 |
| 验收标准明确 | 20 / 20 | 成功、失败、重复、超大 body、Content-Type、JSON、tenant/source 授权均可按单一失败原因测试。 |
| 架构一致性 | 19 / 20 | 与独立营销中台、标准事件、幂等键、tenant 隔离和生产前检查一致。 |

总分：`96 / 100`

## Decision

```text
PASS
```

理由：Review 无 P0/P1，2 条 P2 已在本轮收口；历史 Analysis BLOCK 项已由 `task.md` Owner 决策补齐。本期需求边界、输入输出、业务规则、异常路径、非目标、风险和验收标准已明确，Score >= 80，可交给 Review Agent 审核。
