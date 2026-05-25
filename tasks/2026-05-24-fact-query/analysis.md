# Analysis

## Review 反馈处理

上一轮 Review 结论为 `BLOCK`，原因是 Provider 配置契约未收敛，且 `cacheTtlSeconds` 上限不可验收。本轮按 `task.md` 中 Owner 补充决策重新收敛如下：

1. P1：Provider registry 是否唯一来源。
   - 已处理。本任务固定以持久化 provider registry 作为唯一正式来源，活动规则和 `/openapi/v1/facts/query` 都不得传 inline provider 配置绕过 registry。
   - Provider 以 `tenantId + factCode` 唯一定位。`marketing_fact_provider` 是本任务的数据字典承载对象；现有字段不足，需求上明确需要补齐或等价表达 `tenant_id`、`fact_code`、`provider_type`、`endpoint`、`method`、`auth_ref`、`mapping_json`、`cache_ttl_seconds`、`enabled`、`timeout_ms`、`created_at`、`updated_at`。
   - 同一 tenant 内 `factCode` 必须唯一，不同 tenant 的同名 `factCode` 必须隔离，不能互相引用、查询或复用缓存。

2. P1：`/openapi/v1/facts/query` 是否允许 inline provider。
   - 已处理。本期不允许 inline endpoint、inline auth、inline mapping 或 inline provider。
   - `/openapi/v1/facts/query` 只接受 `tenantId`、`factCode`、`subject`、`biz`、`attributes`、`traceId` 或 request id。服务端按 `tenantId + factCode` 查找 enabled provider，再执行 HTTP 查询、mapping、缓存和错误降级。
   - `docs/api.md` 中带 `provider` 的旧示例视为待更新文档，本任务以 `task.md` Owner 决策和本分析为准。

3. P1：后台活动创建校验 provider 的入口。
   - 已处理。本任务要求现有 `POST /admin/v1/activities` 在保存或上线校验时同步校验规则引用的 provider。
   - 若规则引用 `facts.<factCode>.*`，必须校验当前 `tenantId` 下存在 enabled 且 schema 合法的 HTTP provider，并且 mapping 覆盖规则引用字段。未通过则拒绝保存或上线。
   - 本任务不要求新增完整 Fact Provider 管理后台页面；provider 可通过数据预置、最小后台 API 或其他受控配置方式进入 registry，但进入规则校验和运行时查询的唯一来源必须是 registry。

4. P2：`cacheTtlSeconds` 上限。
   - 已处理。本任务固定 `cacheTtlSeconds` 最大值为 `3600` 秒。
   - 缺失或 `0` 表示不跨事件缓存；负数拒绝 provider 配置；大于 `3600` 拒绝 provider 配置，返回可识别配置错误 `FACT_CACHE_TTL_INVALID`，不得静默截断。

5. Open Questions 回应。
   - Q1：是，provider registry 以 `marketing_fact_provider` 为正式数据字典承载对象；本任务要求补齐或等价表达 `tenant_id`、`fact_code` 唯一约束、`mapping_json`、`enabled`、`updated_at`。配置版本可用显式版本或 `updated_at` 表达，缓存 key 必须包含其一。
   - Q2：`/openapi/v1/facts/query` 按 `tenantId + factCode + event context` 查询已配置 provider，不允许 debug-only inline provider。
   - Q3：后台活动创建校验入口是现有 `POST /admin/v1/activities` 的同步保存/上线校验；是否另建 Provider 管理 API 不阻塞本任务，但不能替代活动创建校验。

## 需求说明

本需求为 ExMarketing 补齐可选 Fact Query 能力，使活动规则在标准事件 `attributes.*` 和系统 `state.*` 之外，可以读取外部事实 `facts.<factCode>.*`。

本期核心口径：

- Fact Query 是可选能力，不依赖 facts 的活动不受 provider 配置影响。
- Provider registry 是唯一正式配置来源，不允许活动规则或 Fact Query API 使用 inline provider。
- Provider 按 `tenantId + factCode` 唯一定位并强制租户隔离。
- 只有 `enabled = true` 且 schema 校验通过的 HTTP provider 才允许被规则引用和查询。
- Provider 响应必须经过显式 mapping 后进入 `facts.<factCode>.*`，不得把原始响应整体透传给规则。
- 查询失败、映射失败、字段缺失或类型不匹配时，必须输出机器可识别原因；依赖该 fact 的规则不得默认通过。
- Fact Query 不改变 Event API 幂等、participation、grant_order、outbox、quota 的既有生命周期。

本分析依据 `task.md`、`docs/architecture.md`、`docs/api.md`、`docs/data-model.md`、`docs/workflow.md` 和 `docs/roadmap.md`。未读取代码，未设计具体代码改法。

## 背景目标

ExMarketing 是独立活动营销中台，外部系统通过标准事件、Fact Query、Reward Command 和 Callback In 对接。会员等级、用户画像、风控结果、历史统计等事实不应以内置业务查询、业务专用 controller 或直连业务库方式进入营销平台。

本需求目标是提供一个最小但可验收的 HTTP Fact Provider 能力：

1. 持久化登记并启用事实源。
2. 创建或上线活动时校验 `facts.*` 规则引用。
3. 正式事件处理和模拟运行在确实需要 facts 时查询外部 HTTP provider。
4. provider 响应经过 mapping 转换为规则可读 facts。
5. 查询过程具备租户隔离、安全边界、缓存边界、失败降级和规则解释。

## 参与方

- 外部事实系统：提供 HTTP endpoint，接收标准事件上下文，返回 JSON 事实数据。
- 活动配置方 / 运营后台：配置活动规则并引用 `facts.*` 字段。
- Provider 配置维护方：通过受控方式维护 `marketing_fact_provider` registry。
- ExMarketing 管理 API：在活动保存或上线时校验 provider 引用。
- Fact Query API：按 registry 查询 provider，用于调试、模拟和接入验收。
- 规则引擎：读取 mapped facts 上下文并输出规则级解释。
- 事件处理流程：按活动依赖决定是否查询 facts，并保持原有发奖事务语义。
- 安全 / 运维方：维护 `EX_MARKETING_FACT_ALLOWLIST`，排查超时、SSRF 拦截和 provider 错误。
- 审计 / 排障使用方：查看 facts 查询摘要、规则拒绝原因和脱敏错误。

## 输入输出

### Provider Registry 数据契约

本任务固定 `marketing_fact_provider` 为正式 provider registry 承载对象。若现有数据字典字段不足，本任务需求要求补齐或提供等价持久化字段；不得用仅运行时 inline 配置替代。

| 字段 | 要求 |
|---|---|
| `tenant_id` | 必填。provider、规则校验、Fact Query、缓存都按 tenant 隔离。 |
| `fact_code` | 必填。规则路径命名空间，建议格式 `^[A-Z][A-Z0-9_]{1,63}$`。同一 tenant 内唯一。 |
| `provider_type` | 必填。本期只支持 `HTTP`。 |
| `endpoint` | 必填。仅允许 HTTP provider endpoint；host 必须命中 `EX_MARKETING_FACT_ALLOWLIST`，并通过 SSRF 防护。 |
| `method` | 必填。本期支持 `GET` / `POST`。 |
| `auth_ref` | 可选。只表示密钥或鉴权配置引用；不得在错误、日志、规则解释中明文输出。 |
| `mapping_json` | 必填。声明 provider JSON 响应到 `facts.<factCode>.*` 的字段映射和类型。 |
| `cache_ttl_seconds` | 必填或有默认值。缺失/`0` 不跨事件缓存；`1..3600` 启用缓存；负数或大于 `3600` 拒绝。 |
| `enabled` | 必填。只有 `true` 且 schema 合法的 provider 可被引用和查询。 |
| `timeout_ms` | 必填或有安全默认值。超时必须输出稳定错误码。 |
| `created_at` | 必填。用于审计和排障。 |
| `updated_at` | 必填。用于审计、缓存隔离或配置版本判断。 |

唯一性要求：`tenant_id + fact_code` 唯一。缓存 key 必须包含 provider 配置版本或 `updated_at`，避免配置变更后复用旧事实。

### 标准事件上下文约束

正式事件处理链路中，Fact Query 使用标准事件模型作为上下文；但公开调试接口 `/openapi/v1/facts/query` 本期只接受 Owner 决策列出的固定字段，不额外接受 `eventType`、`sourceSystem`、`occurredAt`。

如果 provider 请求需要事件类型、来源系统或发生时间，本期必须通过 provider 配置、`subject`、`biz` 或 `attributes` 中已允许字段表达；不能扩展 `/facts/query` 顶层输入白名单。

### Fact Query API 输入

`POST /openapi/v1/facts/query` 本期只接受：

- `tenantId`
- `factCode`
- `subject`
- `biz`
- `attributes`
- `traceId` 或 request id

不得携带 `eventType`、`sourceSystem`、`occurredAt` 作为 `/facts/query` 顶层输入。不得接受：

- inline endpoint
- inline method
- inline headers
- inline auth
- inline mapping
- inline provider

服务端必须按 `tenantId + factCode` 查找 enabled provider。查不到、未启用、schema 非法或 tenant 不匹配时，请求失败并返回稳定错误码。

### Mapping 输入

本任务采用显式字段映射，不允许把 provider 原始响应整体无约束透传为规则事实。

最小 mapping 语义：

- `mapping_json` 是对象，key 为输出字段相对路径，value 描述来源路径和类型。
- 输出字段如 `vipLevel`、`risk.score`，最终进入 `facts.<factCode>.<outputPath>`。
- 来源路径只从 provider JSON 响应读取，如 `$.data.vipLevel`。
- 支持基础类型：`string`、`number`、`boolean`、`array`、`object`。
- mapping 字段缺失、类型不匹配、来源路径非法时，依赖该字段的规则失败，不得静默通过。

示例：

```json
{
  "vipLevel": {"source": "$.data.vipLevel", "type": "string"},
  "depositCount30d": {"source": "$.data.stats.depositCount30d", "type": "number"}
}
```

Provider 原始响应：

```json
{
  "data": {
    "vipLevel": "VIP3",
    "stats": {"depositCount30d": 5}
  }
}
```

映射后的规则上下文：

```json
{
  "facts": {
    "USER_PROFILE": {
      "vipLevel": "VIP3",
      "depositCount30d": 5
    }
  }
}
```

### 规则配置输入

活动规则字段路径支持现有文档定义：

- `attributes.<path>`
- `state.activity.<field>`
- `state.subject.<field>`
- `state.quota.<quotaType>.<field>`
- `state.exclusive.<field>`
- `facts.<factCode>.<path>`

后台创建或更新活动时，凡规则字段引用 `facts.<factCode>.<path>`，`POST /admin/v1/activities` 必须同步校验：

1. 当前 `tenantId` 下存在 `factCode` provider。
2. provider `enabled = true`。
3. provider 类型为 `HTTP`。
4. provider schema 合法，包括 endpoint、method、mapping、cache TTL、timeout。
5. mapping 中存在规则引用的 `<path>`。
6. mapping 声明类型与规则操作符可比较。

校验失败时，后台不得保存或上线该活动，并返回可识别配置错误，错误信息必须指出缺失或非法的 `factCode` / 字段路径。

### Fact Query API 输出

`POST /openapi/v1/facts/query` 响应必须机器可识别，至少包含：

- `tenantId`
- `factCode`
- `success`
- `cached`
- `facts`：成功时为映射后的 `facts.<factCode>` 数据。
- `errorCode`：失败时必填。
- `error`：脱敏后的可读原因。
- `providerStatus`：HTTP 状态码或安全拦截摘要，不包含敏感 header 或原始敏感响应。

成功示例：

```json
{
  "tenantId": "site_001",
  "factCode": "USER_PROFILE",
  "success": true,
  "cached": false,
  "facts": {
    "USER_PROFILE": {
      "vipLevel": "VIP3",
      "depositCount30d": 5
    }
  }
}
```

失败示例：

```json
{
  "tenantId": "site_001",
  "factCode": "USER_PROFILE",
  "success": false,
  "cached": false,
  "errorCode": "FACT_PROVIDER_TIMEOUT",
  "error": "fact provider request timeout"
}
```

### 规则执行输出

依赖 `facts.*` 的规则执行后必须输出规则级解释：

- 查询成功且字段存在：输出 `field`、`expected`、`actual`、`passed`、`reasonCode`、`reason`。
- provider 未配置或未启用：输出 `FACT_PROVIDER_NOT_CONFIGURED` 或 `FACT_PROVIDER_DISABLED`。
- 查询失败、超时、安全拦截：依赖该 fact 的规则失败，输出对应 `FACT_*` 错误码。
- mapping 字段缺失：输出 `FACT_FIELD_MISSING`。
- 类型不匹配：输出 `FACT_TYPE_MISMATCH`。

### 事件处理输出

正式事件处理保持现有语义：

- 不依赖 `facts.*` 的活动不触发 Fact Query。
- 规则拒绝时写 `REJECTED` participation，不生成 grant_order / outbox，不扣 quota。
- 规则通过后仍进入现有 activity 级事务处理 quota、grant_order 和 outbox。
- Fact Query 失败导致规则拒绝时，拒绝原因必须能在 participation `snapshot_json` 的规则结果摘要或 `reject_reason` 中追踪。

模拟运行保持现有语义：

- 不写 `marketing_event_inbox`、`marketing_participation`、`marketing_grant_order`、`marketing_outbox`、`marketing_quota`。
- 应展示 facts 查询成功结果、缓存命中状态或失败原因。

## 业务规则

1. Fact Query 是可选能力，未被规则引用时不得强制调用 provider。
2. 已有只依赖 `attributes.*` / `state.*` 的活动，不受 Fact Provider 缺失、不可用、allowlist 未配置影响。
3. 本任务只开放 HTTP Fact Provider，不开放 DB、MQ、脚本、内置业务查询等 provider。
4. Provider registry 是后台规则校验、模拟运行、正式事件处理和 `/openapi/v1/facts/query` 的唯一 provider 来源。
5. 活动规则和 Fact Query API 不得接受 inline provider 配置。
6. Provider 请求必须以标准事件上下文为输入，外部系统按事件中的 tenant、subject、biz、attributes 返回事实。
7. 活动规则引用 `facts.<factCode>.*` 时，后台必须校验 provider 存在、启用、同 tenant、mapping 覆盖字段。
8. 未配置 provider 时不得保存或上线新规则；历史脏配置执行时不得默认通过。
9. `factCode` 是 facts 命名空间，同一 tenant 内唯一；不同 tenant 的同名 `factCode` 不得互相引用或复用缓存。
10. HTTP endpoint host 必须在 `EX_MARKETING_FACT_ALLOWLIST` 内。
11. Fact Query 必须拒绝 private / local IP，禁止 redirect，禁用 proxy。
12. Provider 响应必须是可解析 JSON；mapping 只能从 JSON 响应读取字段。
13. Provider 响应必须经过 mapping 后才能进入规则上下文，不允许规则直接依赖未声明的原始响应字段。
14. `cacheTtlSeconds` 缺失或 `0` 表示不跨事件缓存；`1..3600` 启用缓存；负数或大于 `3600` 拒绝 provider 配置。
15. 缓存 key 至少隔离 `tenantId`、`factCode`、provider 配置版本或 `updatedAt`、`subject.type`、`subject.id`、`biz.bizType`、`biz.bizId`，以及会影响 provider 响应的必要 attributes。
16. 同一事件处理上下文中，同一 tenant、同一 factCode、同一 subject/biz 的重复查询应复用本次结果，避免同一活动多条规则重复请求。
17. Provider 查询失败不得让依赖该 fact 的规则默认通过；本任务的“降级”定义为记录失败原因并使依赖该 fact 的规则失败。
18. 一个 factCode 查询失败只影响依赖该 factCode 的规则和活动，不影响不依赖该 factCode 的活动。
19. 同一活动内若必须满足的规则依赖失败 fact，则该活动拒绝；不应跳过该规则后继续发奖。
20. 错误、规则解释、模拟结果、审计摘要不得泄漏 Authorization、token、完整 headers、内部网络地址、provider 原始敏感响应。
21. Provider 非 2xx、超时、非 JSON、mapping 失败、字段缺失、类型不匹配、SSRF 拦截、allowlist 不匹配必须有稳定错误码。
22. Fact Query 不改变外部事件幂等键、participation 幂等键、grant_order 幂等键和 quota 扣减规则。

## 边界条件

- 活动只依赖 `attributes.*`：不触发 Fact Query，未配置 provider 不影响活动创建、模拟或正式执行。
- 活动同时依赖 `attributes.*` 和 `facts.USER_PROFILE.*`：只查询 `USER_PROFILE`，其他 provider 不影响该活动。
- 多个规则依赖同一 `factCode`：同一事件处理上下文内只需要一次查询结果，后续规则复用 mapped facts 或同一失败结果。
- 多个活动依赖同一 `factCode`：可以复用符合缓存 key 的结果，但不得跨 tenant、subject、biz 或 provider 配置版本复用。
- 多个 provider 使用相同 endpoint：仍按 `tenantId + factCode` 分离命名空间、mapping 和缓存。
- Provider 返回空对象：mapping 字段缺失，依赖字段的规则失败。
- Provider 返回空数组或顶层数组：只有 mapping 明确支持的路径可读取；否则视为 mapping 失败。
- Provider 返回额外字段：未在 mapping 中声明的字段不进入规则上下文。
- Mapping 输出字段与规则操作符不兼容：活动配置阶段拒绝；历史配置执行时规则失败。
- `cacheTtlSeconds > 3600`：provider 配置拒绝，错误码 `FACT_CACHE_TTL_INVALID`。
- `EX_MARKETING_FACT_ALLOWLIST` 未配置：任何需要外部 HTTP provider 的查询都应失败；不依赖 facts 的活动不受影响。
- 模拟运行触发 facts：可以发起 provider 查询，但不得写业务表，不得扣 quota。
- 正式事件重复上报：沿用 Event API 幂等语义，不得因 Fact Query 再次发起发奖。
- Provider headers 或 authRef 含敏感值：允许发送必要鉴权信息，但展示、日志、错误和规则解释必须脱敏。
- Registry 中存在 disabled provider：不能被新规则引用，Fact Query API 查询该 factCode 返回 disabled 类错误。
- Registry 中存在跨 tenant 同名 factCode：只能查询当前请求 tenant 的 provider。

## 异常场景

| 场景 | 期望行为 | 错误码 |
|---|---|---|
| 规则引用不存在的 factCode | 后台拒绝保存；历史配置执行失败 | `FACT_PROVIDER_NOT_CONFIGURED` |
| provider 被禁用 | 后台拒绝保存；Fact Query 和历史配置执行失败 | `FACT_PROVIDER_DISABLED` |
| provider 类型不是 HTTP | 后台拒绝保存或查询拒绝 | `FACT_PROVIDER_UNSUPPORTED` |
| Fact Query 请求包含 inline provider | 请求拒绝 | `FACT_INLINE_PROVIDER_NOT_ALLOWED` |
| tenantId + factCode 不唯一或冲突 | 配置拒绝或查询拒绝 | `FACT_PROVIDER_CONFLICT` |
| cacheTtlSeconds 为负数或大于 3600 | 配置拒绝 | `FACT_CACHE_TTL_INVALID` |
| allowlist 未配置或未命中 | 查询拒绝，依赖规则失败 | `FACT_ENDPOINT_NOT_ALLOWED` |
| endpoint 解析为 private/local IP | 查询拒绝，依赖规则失败 | `FACT_ENDPOINT_BLOCKED` |
| provider redirect | 查询拒绝，依赖规则失败 | `FACT_REDIRECT_BLOCKED` |
| provider 请求超时 | 依赖规则失败，输出超时原因 | `FACT_PROVIDER_TIMEOUT` |
| provider 返回 401/403 | 依赖规则失败，错误脱敏 | `FACT_PROVIDER_UNAUTHORIZED` |
| provider 返回 404 | 依赖规则失败，错误脱敏 | `FACT_PROVIDER_NOT_FOUND` |
| provider 返回 429 | 依赖规则失败，错误脱敏 | `FACT_PROVIDER_RATE_LIMITED` |
| provider 返回 5xx | 依赖规则失败，错误脱敏 | `FACT_PROVIDER_ERROR` |
| provider 返回非 JSON | 依赖规则失败 | `FACT_RESPONSE_INVALID_JSON` |
| provider 响应体过大 | 查询拒绝或失败 | `FACT_RESPONSE_TOO_LARGE` |
| mapping 来源路径不存在 | 依赖字段规则失败 | `FACT_FIELD_MISSING` |
| mapping 类型不匹配 | 依赖字段规则失败 | `FACT_TYPE_MISMATCH` |
| mapping 语法非法 | provider 配置拒绝或查询失败 | `FACT_MAPPING_INVALID` |
| cache key 冲突风险 | 不得复用缓存，按 miss 查询或拒绝 | `FACT_CACHE_KEY_INVALID` |
| provider 返回敏感原文 | 对外只展示脱敏摘要 | `FACT_PROVIDER_ERROR` |

## 非目标范围

- 不新增业务专用 controller、业务专用规则流程或业务数据库直连查询。
- 不把用户注册、会员等级计算、真实风控判定、订单统计等业务能力内置到 ExMarketing。
- 不要求所有活动必须配置 Fact Provider。
- 不改变标准事件模型；业务扩展字段仍只能进入 `attributes`。
- 不改变 participation、grant_order、outbox、quota 的核心生命周期。
- 不绕过现有 quota 原子扣减和发奖链路。
- 不开放 MQ provider、数据库 provider、脚本 provider 等 provider 类型。
- 不允许 inline provider 查询。
- 不允许跨租户共享 provider。
- 不实现完整生产级 Fact Provider 配置中心、审批流、密钥托管和权限体系。
- 不要求本任务完成 OpenAPI 鉴权、Admin 鉴权或 provider 签名机制；这些属于 roadmap Phase 5。但本地未鉴权不改变 provider registry、租户隔离和脱敏要求。
- 不放宽 SSRF 防护、安全 allowlist、redirect 和 proxy 限制。
- 不把 `/openapi/v1/facts/query` 作为外部业务系统批量查数 API；它只服务营销规则事实查询、调试和模拟。
- 不设计具体代码改法、包结构、函数签名或数据库 migration 细节。

## 风险

- P1：现有 `docs/data-model.md` 说明 `marketing_fact_provider` 当前字段不足且运行时尚未从表加载 provider。本任务已将补齐 registry 契约列为必做，否则不能满足“未配置 provider 不允许规则依赖”。
- P1：`docs/api.md` 仍保留 inline provider 示例，与 Owner 决策冲突。本任务明确废弃 inline 口径，需同步更新 API 文档，否则接入方可能按旧契约调用。
- P1：mapping 语法必须按本分析的最小字段映射收敛，否则规则字段是否存在、类型是否匹配、验收样例都不可判定。
- P1：缓存 key 若遗漏 tenant、subject、biz、必要 attributes 或 provider 配置版本，可能导致跨用户、跨业务或跨配置误判，产生误发奖。
- P1：失败降级若被误解为忽略 facts 规则继续通过，会直接导致错误发奖。本需求明确降级为“失败可解释且依赖规则不通过”。
- P2：外部 provider 增加事件处理延迟和可用性风险，需要超时、响应体大小、错误码和局部失败隔离。
- P2：headers、Authorization、provider 原始响应和内部网络信息存在泄漏风险，必须全链路脱敏。
- P2：生产鉴权和密钥托管尚不在本任务范围内，因此生产开放前仍需单独安全评审。

## 验收标准

1. 不依赖 `facts.*` 的活动可以仅凭 `attributes.*` / `state.*` 完成后台创建、模拟运行和正式事件处理，未配置 Fact Provider 不影响结果。
2. Provider registry 以 `tenantId + factCode` 唯一定位；同 tenant 重复 `factCode` 被拒绝或不可进入可用状态。
3. Provider registry 包含或等价表达 `tenant_id`、`fact_code`、`provider_type`、`endpoint`、`method`、`auth_ref`、`mapping_json`、`cache_ttl_seconds`、`enabled`、`timeout_ms`、`created_at`、`updated_at`。
4. 后台创建活动时，规则引用 `facts.USER_PROFILE.vipLevel` 且同 tenant 没有 enabled 的 `USER_PROFILE` provider，应返回可识别配置错误，不保存活动。
5. 后台创建活动时，规则引用其他 tenant 的同名 `factCode` 不得通过校验。
6. 后台创建活动时，规则引用的 facts 字段不在 provider mapping 中，应返回可识别配置错误。
7. 后台创建活动时，规则操作符与 mapping 声明类型明显不兼容，应拒绝或给出可识别配置错误。
8. `/openapi/v1/facts/query` 只接受 `tenantId`、`factCode`、`subject`、`biz`、`attributes`、`traceId` 或 request id；传入 inline provider、endpoint、auth、mapping 时返回 `FACT_INLINE_PROVIDER_NOT_ALLOWED` 或等价错误。
9. `/openapi/v1/facts/query` 按 `tenantId + factCode` 查 registry；provider 不存在、disabled、tenant 不匹配时返回稳定错误码。
10. HTTP provider 只接受本任务允许的 provider 类型和 method；非 HTTP provider 不可执行。
11. Fact Query endpoint host 未命中 `EX_MARKETING_FACT_ALLOWLIST` 时，请求被拒绝，错误码为 allowlist 类错误。
12. endpoint 解析到 private/local IP、发生 redirect、使用 proxy 等安全违规场景被拒绝。
13. `cacheTtlSeconds` 缺失或 `0` 时不跨事件缓存；`1..3600` 正常缓存；负数或大于 `3600` 的 provider 配置被拒绝并返回 `FACT_CACHE_TTL_INVALID` 或等价配置错误。
14. 正数缓存不会跨 tenant、subject、biz、必要 attributes、provider 配置版本或 `updatedAt` 错误复用。
15. Provider 超时、非 2xx、非 JSON、响应体过大、mapping 失败、字段缺失、类型不匹配均有稳定错误码和脱敏原因。
16. Provider 成功返回并完成 mapping 后，规则可以读取 `facts.<factCode>.<path>`，并输出 field、expected、actual、passed、reasonCode、reason。
17. Provider 查询失败时，依赖该 fact 的规则不通过；活动因此拒绝时，不生成 grant_order / outbox，不扣 quota，并能在 participation 快照或拒绝原因中追踪。
18. Provider 查询失败只影响依赖该 factCode 的规则或活动，不影响不依赖该 factCode 的活动。
19. 同一事件处理上下文内多个规则依赖同一 factCode 时，不重复发起不必要查询，并复用同一成功结果或失败原因。
20. 模拟运行展示 facts 查询成功结果、缓存命中状态或失败原因，且不写 inbox、participation、grant_order、outbox、quota。
21. `/openapi/v1/facts/query` 返回机器可识别响应，成功包含 mapped facts，失败包含 `errorCode` 和脱敏 `error`。
22. 错误响应、规则解释、模拟结果、日志摘要不得泄漏 Authorization、token、完整 headers、内部网络地址或 provider 原始敏感响应。
23. 正式事件处理继续遵守现有 Event API 幂等、规则阶段、quota 原子扣减、grant_order 和 outbox 语义。

## 待确认问题

以下问题不阻塞本任务按当前最小契约进入 Review / Dev；若需要生产开放，仍需在后续生产化任务确认：

1. Provider registry 的管理入口采用数据库预置、最小后台 API 还是完整后台页面。无论入口如何，活动校验和运行时查询都只能读取 registry。
2. `authRef` 对接哪套密钥管理系统，以及是否替代明文 headers。当前任务只要求引用和脱敏，不实现完整密钥托管。
3. Provider 请求最大响应体大小、允许 header 白名单、默认 `timeoutMs` 的具体数值。
4. `/openapi/v1/facts/query` 生产开放前采用哪种鉴权或网络隔离。当前任务不实现鉴权，但不允许因此放宽 tenant 隔离、registry 校验和脱敏。
5. 是否需要把 Fact Query 的调用摘要写入 `marketing_audit`，还是仅进入 participation snapshot / 日志。

## Score

```text
需求清晰度        19 / 20
需求边界清楚      19 / 20
风险可控          18 / 20
验收标准明确      19 / 20
约束一致性        19 / 20
总分              94 / 100
```

## Decision

```text
PASS
```

理由：本轮已逐条处理 Review 的 P1/P2 和 Open Questions，并按 Owner 决策固定 provider registry、租户隔离、mapping/enabled 持久化契约、`/openapi/v1/facts/query` 输入契约、后台活动同步校验入口和 `cacheTtlSeconds` 上限拒绝策略。剩余问题集中在生产化管理入口、密钥托管、鉴权和具体运维参数，不影响本任务按最小可验收契约进入 Review / Dev。
