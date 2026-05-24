# Analysis

## Agent 指令

你是 Analysis Agent。请读取本目录 task.md、项目 README.md、CLAUDE.md、docs/architecture.md 和相关代码。

只做分析，不改代码，不提交。

## 评分规则

满分 100，低于 80 不允许进入研发：

~~~text
需求清晰度        20
范围可控          20
风险可控          20
验收标准明确      20
架构一致性        20
~~~

## 现状

当前代码已经具备标准事件接入的第一版能力：

- `POST /openapi/v1/events` 已存在，HTTP 层使用 `json.Decoder.UseNumber()` 和 `DisallowUnknownFields()` 解码 `marketing.ActivityEvent`。
- `ActivityEvent` 当前标准字段为 `eventId/eventType/sourceSystem/tenantId/subject/biz/occurredAt/attributes`，与需求定义基本一致。
- `ActivityEvent.IdempotencyKey()` 固定为 `tenantId:sourceSystem:eventType:bizType:bizId`，符合 `CLAUDE.md` 的外部事件幂等键要求。
- `ProcessEvent` 先调用 `CreateInbox` 写入 `marketing_event_inbox`，再匹配活动、写 participation、grant_order 和 outbox。
- PostgreSQL schema 中 `marketing_event_inbox.idempotency_key` 已有 `UNIQUE` 约束；PostgreSQL 仓储用 `ON CONFLICT (idempotency_key) DO NOTHING` 去重。
- 内存仓储用 `inboxByIdempotency` map 去重。
- 重复事件当前返回 `200 OK`，响应体 `duplicate=true`，不会再次进入活动匹配和发奖链路。
- 规则引擎字段读取只暴露 `eventId/eventType/sourceSystem/tenantId/subject/biz/attributes`，没有读取外部业务库。
- 奖励计算当前只支持从 `attributes.*` 取 `baseField`。

主要缺口：

- `Idempotency-Key` 目前是可选校验：只有请求头存在且不匹配 payload 时才返回 400；缺少请求头仍会处理事件。需求写明需要 `Idempotency-Key 校验`，OpenAPI 边界应要求该 Header 必填。
- 重复幂等键没有检测 payload 冲突。同一 `Idempotency-Key` 如果携带不同 `eventId`、`subject`、`occurredAt` 或 `attributes`，当前会直接按 duplicate 返回，调用方无法发现幂等键被错误复用。
- `ActivityEvent.Validate()` 对 `attributes == nil` 的赋值发生在值接收者上，不会把事件规范化为 `{}`；如果目标是标准事件模型，建议明确 `attributes` 缺失是否允许，允许则归一化后入库，禁止则返回 400。
- 非法事件错误目前统一为 `"invalid json body"` 或 `ErrInvalidEvent` 拼接消息，缺少稳定错误码；对接方排障不够明确。
- 规则字段边界缺少测试。`DisallowUnknownFields` 能拒绝顶层和嵌套结构未知字段，但没有测试覆盖“事件模型之外字段被拒绝”“规则不能读取非标准顶层字段”。
- `occurredAt` 是标准字段，但规则引擎 `fieldValue` 当前不支持 `occurredAt` 路径。需求强调字段边界是该集合，建议支持该字段或在文档明确规则暂不支持时间字段判断。
- API 文档已有事件示例和幂等键说明，但未明确 Header 必填、幂等键冲突语义、重复响应语义和标准错误码。

## 目标理解

本任务要把 HTTP Event API 收敛成可长期对外开放的标准事件入口：

- 只接收标准事件模型字段：`eventId/eventType/sourceSystem/tenantId/subject/biz/occurredAt/attributes`。
- OpenAPI 请求必须携带 `Idempotency-Key`，且必须等于 payload 计算值。
- 首次合法事件写入 `marketing_event_inbox`，再进入活动参与和发奖流程。
- 完全重复上报必须被识别为 duplicate，且不能重复创建 participation、grant_order、outbox，不能重复扣 quota 或重复发奖。
- 幂等键相同但 payload 不一致应返回明确错误，不应静默当作成功重复。
- 非法 JSON、未知字段、缺少必填字段、幂等键缺失/不匹配/冲突都要返回明确 400 错误。
- 规则、奖励和后续处理只能依赖标准事件字段；业务特定字段只能放在 `attributes` 内。
- 补齐单元测试、必要 PostgreSQL smoke 和 API 文档。

## 范围

建议纳入本任务：

- `POST /openapi/v1/events` 的 Header 必填校验和错误响应收敛。
- 标准事件模型校验和必要归一化。
- inbox 创建接口的重复/冲突语义增强。
- 内存仓储与 PostgreSQL 仓储幂等行为一致性。
- 规则字段边界测试，必要时补齐 `occurredAt` 字段读取。
- HTTP handler、engine、repository 的单元测试。
- API 文档更新，包括正常、重复、非法、冲突示例。
- PostgreSQL smoke 验证 `marketing_event_inbox.idempotency_key` 唯一约束和重复事件不重复发奖。

不建议纳入本任务：

- 不新增活动、规则、奖励或 quota 的业务能力。
- 不新增外部业务库连接。
- 不直接发奖、不修改用户余额。
- 不把事件接入改成 MQ/Kafka。
- 不引入跨活动全局事务；保持当前 activity 级发奖事务边界。
- 不设计生产鉴权，本任务只处理事件模型与幂等语义。

## 方案

1. 收紧 HTTP Event API 校验

- `createEvent` 必须读取 `Idempotency-Key`。
- Header 缺失返回 `400`，错误信息建议为 `Idempotency-Key is required`。
- Header 与 `event.IdempotencyKey()` 不一致返回 `400`，保留当前不匹配语义。
- JSON decode 继续使用 `UseNumber` 和 `DisallowUnknownFields`。
- 对解码错误、字段校验错误、幂等错误返回稳定结构，例如：

~~~json
{
  "code": "INVALID_EVENT",
  "error": "eventId, eventType, sourceSystem and tenantId are required"
}
~~~

2. 明确标准事件模型

- 保留 `ActivityEvent` 当前字段集合，不新增业务字段。
- `attributes` 建议允许为空对象；如果请求未提供或为 `null`，由 HTTP 或 engine 层归一化为 `{}` 后入库和返回，避免 payload 中出现 `null`。
- `subject` 只允许 `type/id`，`biz` 只允许 `bizType/bizId`；继续依赖 `DisallowUnknownFields` 拒绝未知字段。
- 若业务字段需要参与规则，必须放入 `attributes`，文档中明确禁止在顶层扩展。

3. 增强 inbox 幂等语义

当前 `CreateInbox(ctx, inbox) (EventInbox, bool, error)` 只能表达“新建/重复/错误”，不能表达“幂等键冲突”。建议扩展为结构化结果或新增错误：

~~~text
created              首次写入
duplicate            幂等键重复且标准 payload 一致
conflict             幂等键重复但 payload 不一致
~~~

实现方式：

- PostgreSQL：`ON CONFLICT DO NOTHING` 后查出已有 inbox，比较已有标准事件 payload 与本次标准事件 payload。
- Memory：命中 map 后同样比较已有事件与本次事件。
- 比较建议基于标准化后的 `ActivityEvent`，不要用原始 JSON 字符串顺序比较。
- 冲突返回 `ErrIdempotencyConflict`，HTTP 映射为 `400` 或 `409`。本任务建议用 `400 INVALID_IDEMPOTENCY_KEY`，因为是客户端错误复用幂等键。

4. 保持重复事件短路

- 完全重复事件仍在 `ProcessEvent` 的 inbox 阶段短路，返回 `duplicate=true`。
- 短路后不得调用 `FindMatchingActivities`，不得写 participation、grant_order、outbox，不得扣 quota。
- 保留 participation 和 grant_order 的唯一键作为异常路径兜底。

5. 收紧规则和奖励字段边界

- `internal/rule/evaluator.go` 当前只允许标准顶层字段和 `attributes`，建议补齐 `occurredAt`，并增加单元测试锁定允许字段集合。
- `internal/reward/calculator.go` 当前只允许 `attributes.*` 作为 `baseField`，符合边界要求；补充测试保证非 `attributes.*` 的 reward baseField 返回明确错误。
- 不引入 `facts.*` 或外部 adapter 到本任务，避免扩大范围。

6. 文档同步

- `docs/api.md` 和 `README.md` 的事件示例保留。
- 增加 Header 必填说明。
- 增加重复响应示例：`200 OK` + `duplicate=true`。
- 增加幂等键冲突和未知字段错误示例。
- 明确幂等键公式：`tenantId + ":" + sourceSystem + ":" + eventType + ":" + bizType + ":" + bizId`。

## 改动范围

预计修改：

- `internal/httpapi/handler.go`：事件 Header 必填校验、结构化错误响应映射。
- `internal/httpapi/handler_test.go`：补充 HTTP Event API 正常、重复、缺 Header、不匹配 Header、未知字段、缺必填字段、冲突 payload 测试。
- `internal/engine/service.go`：处理仓储返回的幂等冲突；确保 duplicate 继续短路。
- `internal/marketing/model.go`：增加幂等冲突错误、事件标准化/比较 helper；必要时补充 `occurredAt` 支持相关类型逻辑。
- `internal/marketing/repository.go`：如需要，调整 `CreateInbox` 返回值以表达 conflict。
- `internal/marketing/memory_repository.go`：实现重复 payload 一致性检查。
- `internal/postgres/repository.go`：实现重复 payload 一致性检查。
- `internal/rule/evaluator.go`：补齐 `occurredAt` 字段路径或明确不支持；推荐补齐。
- `internal/rule` / `internal/reward` 测试：锁定字段边界。
- `docs/api.md`、`README.md`：同步标准事件、幂等和错误响应说明。

不预计修改：

- `db/migration/001_marketing_core.sql`：现有 `marketing_event_inbox` 字段和唯一约束足够支撑本任务。
- `internal/worker`：重复发奖通过 inbox 短路和 outbox 不新增实现，不需要改 worker。
- `internal/fact`：本任务不扩展 Fact Query。

## 数据模型影响

不需要新增表或字段。

需要确认并保留：

- `marketing_event_inbox.idempotency_key VARCHAR(256) NOT NULL UNIQUE`。
- `marketing_event_inbox.payload_json JSON NOT NULL` 保存标准化事件 payload。
- `marketing_participation.idempotency_key UNIQUE` 和 `marketing_grant_order.idempotency_key UNIQUE` 作为二级兜底。

可选但不建议本任务扩 schema：

- 为 `event_id` 加唯一约束。当前设计以业务幂等键为准，同一外部 `eventId` 可能无法覆盖业务唯一性；贸然加唯一约束可能破坏跨 source/tenant 数据。
- 新增 payload hash 字段。当前可直接比较 `payload_json` 反序列化后的标准事件，没必要扩大 migration 风险。

## API 影响

`POST /openapi/v1/events` 行为会更严格：

- 缺少 `Idempotency-Key`：从当前“可处理”变为 `400`。
- Header 与 payload 计算值不一致：继续 `400`。
- 幂等键重复且 payload 一致：`200 OK`，`duplicate=true`。
- 幂等键重复但 payload 不一致：新增明确错误，建议 `400 INVALID_IDEMPOTENCY_KEY`。
- 未知事件字段：`400 INVALID_EVENT`。
- 缺少必填字段：`400 INVALID_EVENT`。
- 首次接收成功：保持 `202 Accepted`。
- Quota 不足：保持 `409 QUOTA_EXCEEDED`，不属于本任务主要改动。

这是对 OpenAPI 事件入口的收紧，属于需求要求内的兼容性变化；文档必须同步。

## 安全和账务影响

- 金额：不改变金额计算逻辑。
- Quota：重复事件短路必须保证不重复扣 `BUDGET/STOCK/USER_LIMIT`。
- 发奖：重复事件不得新增 `grant_order` 和 `outbox`，不会触发重复发奖指令。
- Outbox：不修改 worker；通过不新增 outbox 保证重复事件不重复投递。
- Fact Query：不改。
- 鉴权：不新增生产鉴权；但 Header 必填校验提升事件入口安全性和可追踪性。
- 审计：首次事件当前会写 `EVENT_IN`；duplicate 当前不写审计。是否审计 duplicate 可作为后续运维增强，本任务不强制，避免扩大写放大。

## 风险点

- P1：如果只依赖当前可选 Header 校验，调用方漏传 `Idempotency-Key` 时仍可写入并发奖，不满足明确的 OpenAPI 幂等契约。
- P1：如果同一幂等键不同 payload 被静默判为 duplicate，会掩盖上游幂等键生成错误，可能导致合法事件被误吞。
- P1：PostgreSQL 和 Memory 仓储的重复/冲突语义必须一致，否则本地测试和生产行为会漂移。
- P1：payload 比较不能用原始 JSON 字符串，否则字段顺序、空 attributes 归一化、时间格式差异会造成误判。应比较标准化后的 `ActivityEvent`。
- P2：`attributes == nil` 是否允许需要实现和文档一致。建议允许并归一化为空对象，减少对接成本。
- P2：补齐 `occurredAt` 规则字段读取后，时间比较目前规则引擎只支持 decimal/string 相等。若需要时间范围判断，应另开需求，本任务只保证字段边界。
- P2：错误响应结构化可能影响已有测试断言，需要同步更新测试。

## 验收方式

最低验证：

~~~bash
go test ./...
go build ./...
~~~

必须新增或补齐的单元测试：

- 首次合法事件：返回 `202`，写入 inbox，生成 participation/grant_order/outbox。
- 完全重复事件：第二次返回 `200` 且 `duplicate=true`，participation/grant_order/outbox 数量不增加，quota 不重复扣减。
- 缺少 `Idempotency-Key`：返回 `400`，不写 inbox。
- `Idempotency-Key` 与 payload 不匹配：返回 `400`，不写 inbox。
- 同一 `Idempotency-Key` 但 payload 不一致：返回明确错误，不新增 participation/grant_order/outbox。
- 缺少 `eventId/eventType/sourceSystem/tenantId/subject/biz/occurredAt` 任一必填字段：返回 `400`。
- 顶层未知字段、`subject` 未知字段、`biz` 未知字段：返回 `400`。
- `attributes` 内任意业务字段允许通过。
- 规则字段路径只能读取标准字段或 `attributes.*`；非标准顶层字段返回 not found。
- reward `baseField` 只允许 `attributes.*`，非 `attributes.*` 返回明确错误。

PostgreSQL smoke：

~~~bash
docker compose up -d postgres
docker compose exec -T postgres psql -U exmarketing -d exmarketing -f /docker-entrypoint-initdb.d/001_marketing_core.sql
docker compose exec -T postgres psql -U exmarketing -d exmarketing -f /docker-entrypoint-initdb.d/002_audit_fact_outbox_indexes.sql
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./...
~~~

建议额外验证：

- 并发提交同一幂等键的相同事件，最终只保留一个 inbox，只有一个请求进入发奖链路。
- 并发提交同一幂等键但不同 payload，一个成功或已存在，另一个返回幂等冲突，不重复发奖。

## Score

~~~text
86
~~~

评分拆分：

~~~text
需求清晰度        18/20
范围可控          17/20
风险可控          16/20
验收标准明确      18/20
架构一致性        17/20
~~~

扣分原因：

- task.md 的目标、范围、非目标、验收标准仍是占位，需要 Analysis 补齐。
- 任务触及事件入口、幂等、inbox、发奖防重和 PostgreSQL 并发语义，风险等级高。
- 幂等冲突的 HTTP 状态码和 `attributes` 缺失语义需要在实现中固定并同步文档。

## Decision

~~~text
PASS
~~~

理由：Score >= 80，需求与现有 Event Intake -> Idempotency -> Campaign Match -> Grant Order -> Outbox 架构一致；改动集中在 HTTP 边界、inbox 幂等语义、仓储一致性、测试和文档，不需要新增 schema 或外部依赖。方案可控，允许进入 Review。

需求：2026-05-24-113400
