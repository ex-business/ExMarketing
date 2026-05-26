# Analysis

## 需求 ID

2026-05-24-fact-query-2

## 标题

Fact Query 查询与缓存

## Review 反馈处理

- P1：缓存 key 口径不足。已修正为：缓存 key 必须包含 `tenantId`、`factCode`、规范化后的完整 provider 输入上下文，以及 provider 配置版本。provider 配置版本优先使用 `marketing_fact_provider.updated_at`；若运行时无法取得可靠 `updated_at`，必须使用 `provider_type + endpoint + method + auth_ref + mapping_json + cache_ttl_seconds + enabled + timeout_ms` 的规范化配置指纹。这样 provider endpoint、mapping、authRef、timeout、TTL、enabled 等任一配置变化都会自然生成新 key，旧 TTL 缓存不会被复用。
- P1：内部 Fact Query 是否允许携带 public API 之外的标准事件字段。已明确允许且仅允许标准事件模型字段：`eventId`、`eventType`、`sourceSystem`、`tenantId`、`subject`、`biz`、`occurredAt`、`attributes`，外加查询定位字段 `factCode` 和追踪字段 `traceId`。其中实际传给 provider 的标准事件上下文必须全部进入规范化 provider 输入并参与缓存 key；`traceId` 不传 provider、不参与 key。
- P2：错误码粒度偏粗。保留对外稳定错误码 `FACT_MAPPING_FAILED`，但失败响应、规则拒绝原因、日志摘要和后续 Test Plan 必须稳定区分 `MAPPING_REQUIRED_MISSING`、`MAPPING_TYPE_MISMATCH`、`MAPPING_KEY_INVALID`、`MAPPING_SOURCE_INVALID`、`MAPPING_CONFIG_INVALID` 等 reason，避免只剩泛化错误码。
- Open Question：provider 配置变更后是否立即绕过旧缓存。已明确要求配置变更后立即自然绕过旧缓存，方式是将 `marketing_fact_provider.updated_at` 或等价配置指纹纳入缓存 key；不要求额外全量清理旧缓存，旧条目自然过期即可。

## Analysis 自检修正

- 上一轮 `analysis.md` 已给出 `PASS`，但被 Review 判定 `BLOCK`，主要错误假设是“缓存 key 只需要覆盖 tenant/subject/biz/attributes”以及“完整事件上下文可传 provider 但未进入 key”。本轮已改为以规范化 provider 输入为准，凡是会影响 provider 请求语义的标准事件字段都参与 key。
- 上一轮风险分偏高但遗漏配置变更缓存污染风险。本轮补充 provider 配置版本、TTL 变更、mapping 变更、authRef 变更后的缓存隔离要求，并更新风险与验收标准。
- 上一轮待确认问题中“跨实例共享缓存”仍不阻塞，但最小可验收口径已明确：无论单进程缓存还是共享缓存，都必须满足本分析定义的 key 组成；本需求不要求跨实例一致性或主动广播失效。

## 背景与目标

ExMarketing 是独立活动营销中台，只通过标准事件、可选 Fact Query、奖励指令和回调与外部系统交互。来源设计要求系统不查外部业务库、不引用外部业务代码包、不内置具体业务模型；能通过事件 `attributes` 携带的数据，不应实时查询外部系统。

本需求目标是细化 `POST /openapi/v1/facts/query` 的查询适配、字段 mapping、缓存 TTL 和失败降级规则，使活动规则可以读取 `facts.<factCode>.*`，同时保证缓存不会跨租户、跨主体、跨业务对象、跨事件上下文或跨 provider 配置版本复用错误事实。

## 参与方

- 外部事件上报方：优先在标准事件 `attributes` 中提供活动判断所需业务字段。
- ExMarketing OpenAPI：提供 `POST /openapi/v1/facts/query`，按 `tenantId + factCode` 查询已启用 Fact Provider。
- Fact Provider：外部画像、风控、会员、订单事实等 HTTP 服务，只通过 registry 配置接入。
- 活动规则引擎：通过标准字段路径读取 `attributes.*` 和 `facts.<factCode>.*`，并输出可解释命中或拒绝原因。
- 运营/研发配置方：负责维护 `marketing_fact_provider` 中 provider、mapping、TTL、timeout、enabled 状态和更新时间。

## 输入输出

### Fact Query 请求输入

`POST /openapi/v1/facts/query` 只接受 registry key 和标准事件上下文，禁止在请求中传 inline endpoint、auth、mapping、timeout、TTL 或 provider 配置。

最小直接查询请求：

```json
{
  "tenantId": "site_001",
  "factCode": "USER_PROFILE",
  "subject": {"type": "USER", "id": "10001"},
  "biz": {"bizType": "DEPOSIT", "bizId": "pay_123"},
  "attributes": {},
  "traceId": "trace_1"
}
```

事件处理链内部触发时，可使用完整标准事件上下文：

```json
{
  "eventId": "evt_20260524_000001",
  "eventType": "DEPOSIT_SUCCESS",
  "sourceSystem": "payment",
  "tenantId": "site_001",
  "factCode": "USER_PROFILE",
  "subject": {"type": "USER", "id": "10001"},
  "biz": {"bizType": "DEPOSIT", "bizId": "pay_123"},
  "occurredAt": "2026-05-24T12:00:00Z",
  "attributes": {"amount": "299.00"},
  "traceId": "trace_1"
}
```

默认口径：

- `tenantId`、`factCode`、`subject.type`、`subject.id`、`biz.bizType`、`biz.bizId` 必填。
- `attributes` 必填但可为空对象；JSON number 必须保留 `UseNumber` 精度语义。
- `eventId`、`eventType`、`sourceSystem`、`occurredAt` 对直接查询可选；对事件处理链内部触发应来自原标准事件。
- 除标准事件字段、`factCode`、`traceId` 外，请求中出现未知顶层字段应被拒绝为 `INVALID_FACT_QUERY`，不能被透传到 provider。
- `traceId` 可选，只用于链路追踪和解释，不参与 provider 输入、不参与缓存 key。

### Provider registry 输入

`marketing_fact_provider` 是事实源 registry，按 `tenant_id + fact_code` 唯一定位：

- `provider_type`：当前只支持 `HTTP`。
- `endpoint`：HTTP endpoint。
- `method`：当前支持 `GET` / `POST`。
- `auth_ref`：鉴权配置引用，不保存明文密钥。
- `mapping_json`：外部 JSON 到 `facts.<factCode>.*` 的显式映射。
- `cache_ttl_seconds`：`0` 不缓存，`1..3600` 启用缓存。
- `enabled`：只有 enabled provider 可查询、可被规则引用。
- `timeout_ms`：provider 请求超时时间。
- `updated_at`：provider 配置版本，必须进入缓存 key；不可用时使用等价配置指纹。

### 成功输出

```json
{
  "tenantId": "site_001",
  "factCode": "USER_PROFILE",
  "success": true,
  "cached": false,
  "facts": {
    "USER_PROFILE": {
      "vipLevel": "VIP3"
    }
  },
  "providerStatus": "200 OK"
}
```

### 失败输出

失败响应必须包含稳定 `errorCode`、可解释 `message` 和可选稳定 `reason`，不得泄露 secret、完整 token、完整 Authorization header、内部 provider 鉴权细节或敏感 payload。

最小错误码集合：

```text
INVALID_FACT_QUERY
FACT_PROVIDER_NOT_CONFIGURED
FACT_PROVIDER_DISABLED
FACT_PROVIDER_HOST_NOT_ALLOWLISTED
FACT_PROVIDER_PRIVATE_IP_BLOCKED
FACT_PROVIDER_HTTP_FAILED
FACT_PROVIDER_TIMEOUT
FACT_MAPPING_FAILED
FACT_CACHE_UNAVAILABLE
```

mapping 失败统一使用 `FACT_MAPPING_FAILED`，但 `reason` 必须稳定区分：

```text
MAPPING_REQUIRED_MISSING
MAPPING_TYPE_MISMATCH
MAPPING_KEY_INVALID
MAPPING_SOURCE_INVALID
MAPPING_CONFIG_INVALID
```

## 业务规则

1. Fact Query 是可选事实补充能力，不是事件处理强依赖。活动判断所需字段已经在 `attributes` 中时，不应为了同一事实再实时查询 provider。
2. 规则字段路径必须继续使用既有标准：`attributes.*` 读取事件业务字段，`facts.<factCode>.*` 读取 Fact Query 输出。禁止新增非标准顶层业务字段、业务 SQL、外部业务代码包或业务数据库连接。
3. `/openapi/v1/facts/query` 按 `tenantId + factCode` 查找 enabled provider；不存在、禁用或类型不支持时返回稳定错误，不允许请求方覆盖 endpoint/mapping/auth/timeout/TTL。
4. provider 请求语义只能来自 registry 配置和规范化后的标准事件上下文。规范化 provider 输入包括：`tenantId`、`factCode`、`subject`、`biz`、`attributes`，以及实际存在的 `eventId`、`eventType`、`sourceSystem`、`occurredAt`；不包括 `traceId`、请求时间、HTTP header 或未声明字段。
5. `mapping_json` 必须是非空对象。mapping key 是输出到 `facts.<factCode>.*` 下的相对路径，不得包含 `facts.` 前缀；`source` 只支持 `$` 起始的简单对象路径和数字数组下标；不支持通配、过滤、脚本、递归搜索或动态表达式。
6. mapping 类型只允许 `string`、`number`、`boolean`、`array`、`object`。`required` 省略默认 `true`；必填字段缺失或类型不匹配时整个查询失败并返回 `FACT_MAPPING_FAILED`，且 reason 分别为 `MAPPING_REQUIRED_MISSING` 或 `MAPPING_TYPE_MISMATCH`；非必填字段缺失时从输出中省略该字段。
7. mapping key/source 非法或 mapping 配置结构非法也返回 `FACT_MAPPING_FAILED`，reason 必须分别稳定为 `MAPPING_KEY_INVALID`、`MAPPING_SOURCE_INVALID` 或 `MAPPING_CONFIG_INVALID`。
8. 成功 mapping 后响应结构必须固定为 `facts[factCode][mappedPath]`，使规则统一使用 `facts.<factCode>.<mappedPath>`。
9. `cache_ttl_seconds = 0` 表示不缓存；`1..3600` 表示成功结果可缓存。超出范围的 provider 配置视为无效配置，不应执行查询。
10. 缓存默认只缓存成功 mapping 后的 facts 结果，不缓存 provider 失败、超时、安全拦截、非法请求或 mapping 失败结果。
11. 缓存 key 必须由以下两部分组成：规范化 provider 配置版本 + 规范化 provider 输入上下文。配置版本优先为 `marketing_fact_provider.updated_at`；若不可用，则为配置指纹。输入上下文必须覆盖所有实际传给 provider 且会影响查询语义的标准事件字段。
12. 缓存 key 的规范化必须稳定：对象字段按确定顺序序列化，数组顺序保留，JSON number 保留数值文本语义，缺失字段和空字符串不得混淆，空对象和缺失对象不得混淆。
13. `traceId`、请求时间、调用方 HTTP header、Authorization、随机数、运行实例 ID 不参与缓存 key。
14. provider 配置发生变更后，因 `updated_at` 或配置指纹变化，后续请求必须自然绕过旧缓存；不要求主动删除旧缓存条目，旧条目可按原 TTL 过期。
15. 命中缓存时响应 `cached=true`，且不访问 provider；未命中、TTL 过期或配置版本变化时访问 provider，并只在成功 mapping 后刷新缓存。
16. Provider HTTP 安全边界必须保持：endpoint host 必须在 `EX_MARKETING_FACT_ALLOWLIST` 中；拒绝 private/local IP；禁止 redirect；禁用 proxy。
17. Provider 超时、HTTP 非 2xx、网络错误、JSON 不可解析、mapping 失败都必须转换为可解释错误；错误信息应足够定位原因类别，但不能暴露敏感配置。
18. 事件处理链集成 Fact Query 时，只有当当前候选活动实际引用 `facts.<factCode>.*` 且需要该 fact 才触发查询。没有任何候选规则或奖励引用该 fact 时，不应查询 provider。
19. 如果事件 `attributes` 已满足活动配置中使用的字段，活动命中和发奖不应依赖 Fact Query 成功。
20. 如果活动规则显式依赖 `facts.<factCode>.*`，而对应查询失败或字段缺失，则该活动应按规则不可满足处理，并在规则结果或活动拒绝原因中输出可解释原因，例如 `FACT_QUERY_FAILED`、`FACT_MAPPING_FAILED`、`FACT_FIELD_MISSING`；不得静默通过。

## 边界条件

- 多租户隔离：provider、缓存、错误解释都必须按 `tenantId` 隔离，不允许跨租户复用 provider 或缓存结果。
- 同一租户同一 `factCode` 只能对应一个 enabled provider；配置冲突不是本需求要解决的问题。
- `attributes` 可为空对象；嵌套对象和数组允许作为 provider 输入和缓存 key 的一部分。
- 完整事件上下文可用于内部触发，但仅限标准事件字段；不能新增如 `user.vipLevel`、`orderAmount` 等非标准顶层业务字段。
- provider 返回空对象：若所有 required mapping 都满足可返回空 facts；若 required 字段缺失则失败。
- provider 返回额外字段：未在 mapping 中声明的字段不得透传到 facts。
- `cache_ttl_seconds` 到期后必须重新查询；缓存失效不能改变接口响应 schema。
- 缓存不可用时，最小口径为降级到直接查询 provider；如果 provider 查询成功，接口仍可成功但应便于观测缓存异常。只有缓存故障导致无法保证正确性时才返回 `FACT_CACHE_UNAVAILABLE`。
- GET provider 如何拼接查询参数、POST provider 请求体格式属于 adapter 细节；需求层只要求实际 provider 输入语义等于规范化 provider 输入上下文，且不得传 secret 明文。

## 异常场景

- 请求 JSON 非法、缺少必填字段、字段类型错误、未知顶层字段或 inline provider 配置：返回 `INVALID_FACT_QUERY`。
- 未配置 provider：返回 `FACT_PROVIDER_NOT_CONFIGURED`。
- provider disabled：返回 `FACT_PROVIDER_DISABLED`。
- allowlist 未配置或 endpoint host 不在 allowlist：返回 `FACT_PROVIDER_HOST_NOT_ALLOWLISTED`。
- endpoint 解析到 private/local IP：返回 `FACT_PROVIDER_PRIVATE_IP_BLOCKED`。
- provider 超时：返回 `FACT_PROVIDER_TIMEOUT`。
- provider 返回非 2xx、网络错误或响应体不可解析：返回 `FACT_PROVIDER_HTTP_FAILED`。
- required mapping source 不存在：返回 `FACT_MAPPING_FAILED`，reason 为 `MAPPING_REQUIRED_MISSING`。
- mapping source 值类型与声明类型不匹配：返回 `FACT_MAPPING_FAILED`，reason 为 `MAPPING_TYPE_MISMATCH`。
- mapping key 非法、source 非法或 mapping 配置结构非法：返回 `FACT_MAPPING_FAILED`，reason 为 `MAPPING_KEY_INVALID`、`MAPPING_SOURCE_INVALID` 或 `MAPPING_CONFIG_INVALID`。
- provider 配置变更但旧缓存仍在 TTL 内：新请求必须因配置版本 key 不同而访问 provider，不得命中旧配置缓存。
- 运行活动时 fact 查询失败：当前依赖该 fact 的活动不通过，并输出可解释原因；同一事件的其他不依赖该 fact 的活动不应被连带失败。

## 非目标范围

- 不新增外部业务数据库连接、外部业务代码依赖或业务 SQL 查询能力。
- 不把会员等级、订单状态、风险标签等具体业务字段内置成强模型。
- 不实现 Fact Provider 配置中心或 Admin CRUD；本需求默认使用既有 `marketing_fact_provider` registry。
- 不设计新的鉴权密钥管理系统；`auth_ref` 仅作为已有/后续运行时引用。
- 不要求 Fact Query 成为所有活动的强制前置步骤。
- 不改变标准事件模型、事件幂等键、奖励发放、quota、outbox 或 callback 语义。
- 不支持脚本 mapping、JSONPath 复杂表达式、通配、过滤、递归搜索、provider redirect、代理或 inline provider 配置。
- 不要求缓存失败结果。
- 不要求跨实例分布式缓存一致性、缓存主动广播失效或清理旧 key；最小可验收口径只要求所使用的缓存机制按本需求 key 规则隔离，TTL 内成功结果可复用且不串用。

## 风险

- SSRF 风险：Fact Provider endpoint 来自配置，必须严格执行 allowlist、private/local IP 拒绝、禁止 redirect、禁用 proxy。
- 强依赖风险：如果活动配置过度依赖实时 facts，会把外部 provider 稳定性传导到活动命中；验收必须覆盖 attributes 足够时不查询、不依赖 facts。
- 缓存串用风险：缓存 key 过粗会跨用户、跨业务对象、跨 attributes、跨事件上下文复用错误事实；默认 key 必须包含规范化 provider 输入上下文。
- 配置变更污染风险：endpoint、mapping、authRef、timeout、TTL 或 enabled 变化后若继续命中旧缓存，会导致规则读取错误 facts；必须将 `updated_at` 或配置指纹纳入 key。
- 数据新鲜度风险：TTL 内可能使用旧事实；默认 TTL 上限 3600 秒，并由 provider 配置显式决定，`0` 可关闭缓存。
- 可解释性风险：fact 查询失败若只表现为活动未命中，会导致排查困难；必须输出稳定 errorCode 和 reason。
- 配置风险：mapping required/type/source 配置错误会导致活动规则不可用；创建或启用依赖 facts 的活动时应以 enabled provider 和合法 mapping 为前置校验依据。

## 验收标准

1. `POST /openapi/v1/facts/query` 使用 `tenantId + factCode` 查找 enabled provider；未配置、禁用、非法请求、安全拦截、超时、HTTP 失败和 mapping 失败均返回稳定可解释错误码。
2. 请求中传入 inline endpoint、auth、mapping、timeout、TTL 或 provider 配置不被接受，不会覆盖 registry。
3. 请求和内部触发只允许标准事件字段、`factCode`、`traceId`；未知顶层业务字段被拒绝，不会传给 provider。
4. provider mapping 成功后输出 `facts.<factCode>.*` 结构，规则可使用 `facts.USER_PROFILE.vipLevel` 等字段路径读取。
5. required mapping 字段缺失或类型不匹配时查询失败；非 required 字段缺失时不阻断成功。
6. mapping 失败响应在 `FACT_MAPPING_FAILED` 下稳定区分 `MAPPING_REQUIRED_MISSING`、`MAPPING_TYPE_MISMATCH`、`MAPPING_KEY_INVALID`、`MAPPING_SOURCE_INVALID`、`MAPPING_CONFIG_INVALID`。
7. `cache_ttl_seconds=0` 时连续相同请求不复用缓存；`cache_ttl_seconds` 在 `1..3600` 时，TTL 内相同 key 第二次查询返回 `cached=true` 且不访问 provider；TTL 过期后重新查询。
8. 缓存 key 按租户、factCode、完整规范化 provider 输入上下文和 provider 配置版本隔离；不同 tenant、subject、biz、attributes、eventType、sourceSystem、occurredAt 或 provider 配置版本不得命中同一缓存结果。
9. provider endpoint、method、authRef、mapping、timeout、TTL、enabled 或 `updated_at` 变化后，即使旧缓存仍在 TTL 内，下一次查询也不得命中旧配置缓存。
10. `traceId` 不影响缓存命中；仅 `traceId` 不同而其他 provider 输入和配置版本相同的请求可以命中同一缓存。
11. endpoint host 不在 `EX_MARKETING_FACT_ALLOWLIST`、解析到 private/local IP、发生 redirect 或尝试走 proxy 时被拒绝。
12. 当事件 `attributes` 已包含活动规则所需字段且活动未引用 `facts.*` 时，事件处理不依赖 Fact Query，provider 不可用也不影响该活动按 attributes 正常判断。
13. 当活动显式引用 `facts.<factCode>.*` 且查询失败时，该活动不静默通过，输出 `FACT_QUERY_FAILED` / `FACT_MAPPING_FAILED` / `FACT_FIELD_MISSING` 等可解释原因；同一事件中不依赖该 fact 的活动不被连带失败。
14. 失败响应、日志和审计不得包含 secret、完整 token、完整 Authorization header 或 provider 明文凭据。
15. 文档需同步说明 Fact Query 请求/响应、mapping、TTL、安全约束、错误码、reason、缓存 key 组成和“attributes 优先、facts 可选”的原则。

## 待确认问题

- 后续是否需要 Admin API 管理 `marketing_fact_provider` registry：不阻塞本需求，当前默认由已有数据表或运维初始化配置提供。
- `auth_ref` 的密钥解析和轮换机制：不阻塞本需求，当前只要求不在请求、活动配置、outbox、审计或日志中暴露明文密钥；`auth_ref` 值或其引用名变化必须通过配置版本进入缓存 key。
- 跨实例部署时是否需要共享缓存：不阻塞本需求，当前最小可验收口径允许单服务进程缓存；若使用共享缓存，也必须遵守同一 key 规则。生产化共享缓存一致性可另立需求。

## Score

```text
需求清晰度        19/20
范围可控          18/20
风险可控          19/20
验收标准明确      19/20
架构一致性        19/20
总分              94/100
```

## Decision

```text
PASS
```
