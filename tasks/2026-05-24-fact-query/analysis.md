# Analysis

## 需求说明

本需求为 ExMarketing 补齐可选 Fact Query 能力，使活动规则在标准事件 `attributes.*` 和系统 `state.*` 之外，可以读取外部事实 `facts.<factCode>.*`。

Fact Query 的产品定位如下：

- 它是标准事件之外的补充能力，不是事件处理的必选依赖。
- 能由业务系统在事件中稳定提供的数据，应优先放入 `attributes`，不应强制实时查询外部系统。
- 只有已配置、已校验、当前启用的 HTTP Fact Provider，才允许被活动规则引用。
- 查询失败、映射失败、字段缺失或类型不匹配时，依赖该 fact 的规则不得通过，必须给出机器可识别原因。
- Fact Query 不改变 participation、grant_order、outbox、quota 的既有生命周期。

本轮分析根据现有 `task.md`、`docs/architecture.md`、`docs/api.md`、`docs/data-model.md`、`docs/workflow.md` 和 `docs/roadmap.md` 收敛需求口径，不读取代码，不设计具体代码改法。

## 背景目标

ExMarketing 是独立活动营销中台，外部系统只通过标准事件、Fact Query、Reward Command 和 Callback In 对接。会员等级、用户画像、风控结果、历史统计等事实不应以内置业务查询、业务专用 controller 或直连业务库的形式进入营销平台。

本需求目标是提供一个最小但可验收的 HTTP Fact Provider 能力：

1. 后台或预置配置可以登记事实源。
2. 活动规则可以引用 `facts.<factCode>.<path>`。
3. 正式事件处理和模拟运行在确实需要 facts 时查询外部 HTTP provider。
4. provider 响应经过 mapping 转换为规则可读 facts。
5. 查询过程有安全边界、缓存边界、失败降级和规则解释。

## 参与方

- 外部事实系统：提供 HTTP endpoint，接收标准事件上下文，返回 JSON 事实数据。
- 活动配置方 / 运营后台：维护或选择 Fact Provider，并在活动规则中引用 `facts.*` 字段。
- ExMarketing 管理 API：创建活动时校验规则引用的 provider 是否存在、启用、可引用。
- Fact Query API：用于调试、模拟和 provider 接入验收。
- 规则引擎：读取 `facts` 上下文并输出规则级解释。
- 事件处理流程：根据活动规则依赖决定是否查询 facts，并保持原有发奖事务语义。
- 安全 / 运维方：维护 `EX_MARKETING_FACT_ALLOWLIST`，排查超时、SSRF 拦截、provider 错误。
- 审计 / 排障使用方：查看 facts 查询摘要、规则拒绝原因和脱敏错误。

## 输入输出

### 标准事件输入

Fact Query 必须使用现有标准事件模型作为上下文：

- `eventId`
- `eventType`
- `sourceSystem`
- `tenantId`
- `subject.type`
- `subject.id`
- `biz.bizType`
- `biz.bizId`
- `occurredAt`
- `attributes`

事件顶层、`subject`、`biz` 的未知字段不得通过 Fact Query 绕过 Event API 约束；业务扩展字段仍只能放入 `attributes`。

### Fact Provider 配置输入

本任务内采用最小可验收配置模型：

| 字段 | 要求 |
|---|---|
| `tenantId` | 必填。provider 按租户隔离；如当前表结构未显式列出，验收仍必须保证不会跨 tenant 引用。 |
| `factCode` | 必填。规则路径命名空间，建议格式 `^[A-Z][A-Z0-9_]{1,63}$`，同一 tenant 内唯一。 |
| `provider` | 必填。本任务只允许 `HTTP`。 |
| `endpoint` | 必填。仅允许 `https://` 或本地开发明确允许的 `http://`；host 必须命中 `EX_MARKETING_FACT_ALLOWLIST`。 |
| `method` | 必填。本任务只接受 `POST`；请求体为标准事件上下文。 |
| `headers` / `headers_json` | 可选。只允许必要的固定请求头；敏感头展示和错误输出必须脱敏。 |
| `authRef` | 本任务不作为必填能力。若出现，仅作为外部密钥引用标识，不允许在规则解释或错误中明文输出。 |
| `mapping` | 必填。把 provider JSON 响应映射到 `facts.<factCode>` 下。 |
| `cacheTtlSeconds` | 可选。缺失或 `0` 表示不跨事件缓存；正数启用缓存；负数非法；最大值建议不超过 `3600` 秒。 |
| `enabled` | 需要有等价状态语义。未启用 provider 不允许新规则引用；历史配置执行时失败。 |

Provider 配置来源在本任务内收敛为“可被后台活动创建校验和运行时读取的 provider 配置”。可以来自现有 `marketing_fact_provider` 预置数据、管理后台配置或本地开发配置，但不得只依赖 `/openapi/v1/facts/query` 的 inline provider，否则无法满足“后台不允许配置依赖未配置事实源的规则”。

### Mapping 输入

本任务要求采用显式字段映射，不允许把 provider 原始响应整体无约束透传为规则事实。

最小 mapping 语义：

- `mapping` 是对象，key 为输出字段名，value 描述 provider 响应中的来源路径和类型。
- 输出字段名只允许相对路径，如 `vipLevel`、`risk.score`，最终进入 `facts.<factCode>.<outputPath>`。
- 来源路径只从 provider JSON 响应读取，如 `$.data.vipLevel`。
- 支持基础类型：`string`、`number`、`boolean`、`array`、`object`。
- mapping 字段缺失、类型不匹配、来源路径非法时，依赖该字段的规则失败，不得静默通过。

示例口径：

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

后台创建或更新活动时，凡规则字段引用 `facts.<factCode>.<path>`，必须校验：

1. `<factCode>` 对应 provider 在同一 tenant 内存在。
2. provider 类型为 `HTTP` 且启用。
3. provider endpoint 通过基本安全配置校验。
4. mapping 中存在规则引用的 `<path>`。
5. mapping 声明类型与规则操作符可比较。

无法完成上述校验时，后台不得保存该活动配置。

### Fact Query API 输出

`POST /openapi/v1/facts/query` 用于调试、模拟和接入验收。响应必须机器可识别，至少包含：

- `factCode`
- `success`
- `cached`
- `facts`：映射后的 `facts.<factCode>` 数据。
- `errorCode`：失败时必填。
- `error`：脱敏后的可读原因。
- `providerStatus`：HTTP 状态码或安全拦截摘要。

成功示例口径：

```json
{
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

失败示例口径：

```json
{
  "factCode": "USER_PROFILE",
  "success": false,
  "cached": false,
  "errorCode": "FACT_PROVIDER_TIMEOUT",
  "error": "fact provider request timeout"
}
```

### 规则执行输出

依赖 `facts.*` 的规则执行后必须输出规则级解释：

- 查询成功且字段存在：输出 field、expected、actual、passed、reasonCode、reason。
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
4. Provider 请求必须以标准事件为上下文，外部系统按事件中的 tenant、subject、biz、attributes 返回事实。
5. 活动规则引用 `facts.<factCode>.*` 时，后台必须校验 provider 存在、启用、同 tenant、mapping 覆盖字段。
6. 未配置 provider 时不得保存新规则；历史脏配置执行时不得默认通过。
7. `factCode` 是 facts 命名空间，同一 tenant 内唯一；不同 tenant 的同名 `factCode` 不得互相引用或复用缓存。
8. HTTP endpoint host 必须在 `EX_MARKETING_FACT_ALLOWLIST` 内。
9. Fact Query 必须拒绝 private / local IP，禁止 redirect，禁用 proxy。
10. Provider 响应必须是可解析 JSON；mapping 只能从 JSON 响应读取字段。
11. Provider 响应必须经过 mapping 后才能进入规则上下文，不允许规则直接依赖未声明的原始响应字段。
12. 缺失或 `0` 的 `cacheTtlSeconds` 表示不跨事件缓存；负数非法；正数启用缓存且应设置上限。
13. 缓存 key 至少隔离 `tenantId`、`factCode`、provider 配置版本或更新时间、`subject.type`、`subject.id`、`eventType`、`biz.bizType`、`biz.bizId`；不得跨 tenant、subject、biz 错误复用。
14. 同一事件处理上下文中，同一 tenant、同一 factCode、同一 subject/biz 的重复查询应复用本次结果，避免同一活动多条规则重复请求。
15. Provider 查询失败不得让依赖该 fact 的规则通过；本任务的“降级”定义为记录失败原因并拒绝依赖该 fact 的规则或活动。
16. 一个 factCode 查询失败只影响依赖该 factCode 的规则和活动，不影响不依赖该 factCode 的活动。
17. 同一活动内若必须满足的规则依赖失败 fact，则该活动拒绝；不应跳过该规则后继续发奖。
18. 错误、规则解释、模拟结果、审计摘要不得泄漏 Authorization、token、完整 headers、内部网络地址、provider 原始敏感响应。
19. Provider 非 2xx、超时、非 JSON、mapping 失败、字段缺失、类型不匹配、SSRF 拦截、allowlist 不匹配必须有稳定错误码。
20. Fact Query 不改变外部事件幂等键、participation 幂等键、grant_order 幂等键和 quota 扣减规则。

## 边界条件

- 活动只依赖 `attributes.*`：不触发 Fact Query，未配置 provider 不影响活动创建、模拟或正式执行。
- 活动同时依赖 `attributes.*` 和 `facts.USER_PROFILE.*`：只查询 `USER_PROFILE`，其他 provider 不影响该活动。
- 多个规则依赖同一 `factCode`：同一事件处理上下文内只需要一次查询结果，后续规则复用 mapped facts 或同一失败结果。
- 多个活动依赖同一 `factCode`：可以复用符合缓存 key 的结果，但不得跨 tenant、subject、biz 或 provider 配置版本复用。
- 多个 provider 使用相同 endpoint：仍按 `factCode` 分离命名空间、mapping 和缓存。
- Provider 返回空对象：mapping 字段缺失，依赖字段的规则失败。
- Provider 返回空数组或顶层数组：只有 mapping 明确支持的路径可读取；否则视为 mapping 失败。
- Provider 返回额外字段：未在 mapping 中声明的字段不进入规则上下文。
- Mapping 输出字段与规则操作符不兼容：活动配置阶段应尽量拒绝；历史配置执行时规则失败。
- `cacheTtlSeconds` 很大：超过上限应拒绝 provider 配置或按上限处理，但验收必须可观察到不会无限期使用过期事实。
- `EX_MARKETING_FACT_ALLOWLIST` 未配置：任何需要外部 HTTP provider 的查询都应失败；不依赖 facts 的活动不受影响。
- 模拟运行触发 facts：可以发起 provider 查询，但不得写业务表，不得扣 quota。
- 正式事件重复上报：沿用 Event API 幂等语义，不得因 Fact Query 再次发起发奖。
- Provider headers 含敏感值：允许发送必要 header，但展示、日志、错误和规则解释必须脱敏。

## 异常场景

| 场景 | 期望行为 | 建议错误码 |
|---|---|---|
| 规则引用不存在的 factCode | 后台拒绝保存；历史配置执行失败 | `FACT_PROVIDER_NOT_CONFIGURED` |
| provider 被禁用 | 后台拒绝保存；历史配置执行失败 | `FACT_PROVIDER_DISABLED` |
| provider 类型不是 HTTP | 后台拒绝保存或查询拒绝 | `FACT_PROVIDER_UNSUPPORTED` |
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
- 不实现完整生产级 Fact Provider 配置中心、审批流、密钥托管和权限体系。
- 不要求本任务完成 OpenAPI 鉴权、Admin 鉴权或 provider 签名机制；这些属于 roadmap Phase 5。
- 不放宽 SSRF 防护、安全 allowlist、redirect 和 proxy 限制。
- 不把 `/openapi/v1/facts/query` 作为外部业务系统批量查数 API；它只服务营销规则事实查询、调试和模拟。
- 不设计具体代码改法、包结构、函数签名或数据库 migration 细节。

## 风险

- P1：provider 配置来源需要在研发前固定。需求口径要求运行时和后台校验都能读取 provider 配置；如果只支持 inline query，将无法满足规则配置校验。
- P1：mapping 语法必须按本分析的最小字段映射收敛，否则规则字段是否存在、类型是否匹配、验收样例都不可判定。
- P1：缓存 key 若遗漏 tenant、subject、biz 或 provider 配置版本，可能导致跨用户、跨业务或跨配置误判，产生误发奖。
- P1：失败降级若被误解为忽略 facts 规则继续通过，会直接导致错误发奖。本需求明确降级为“失败可解释且依赖规则不通过”。
- P2：外部 provider 增加事件处理延迟和可用性风险，需要超时、响应体大小、错误码和局部失败隔离。
- P2：headers、Authorization、provider 原始响应和内部网络信息存在泄漏风险，必须全链路脱敏。
- P2：`cacheTtlSeconds` 上限过大可能使用过期会员或风控事实，建议上限不超过 3600 秒，敏感事实可配置为不缓存。
- P2：生产鉴权和密钥托管尚不在本任务范围内，因此生产开放前仍需单独安全评审。

## 验收标准

1. 不依赖 `facts.*` 的活动可以仅凭 `attributes.*` / `state.*` 完成后台创建、模拟运行和正式事件处理，未配置 Fact Provider 不影响结果。
2. 后台创建活动时，规则引用 `facts.USER_PROFILE.vipLevel` 且同 tenant 没有启用的 `USER_PROFILE` provider，应返回可识别配置错误，不保存活动。
3. 后台创建活动时，规则引用的 facts 字段不在 provider mapping 中，应返回可识别配置错误。
4. 后台创建活动时，规则操作符与 mapping 声明类型明显不兼容，应拒绝或给出可识别配置错误。
5. HTTP provider 只接受本任务允许的 provider 类型和 method；非 HTTP provider 不可执行。
6. Fact Query endpoint host 未命中 `EX_MARKETING_FACT_ALLOWLIST` 时，请求被拒绝，错误码为 allowlist 类错误。
7. endpoint 解析到 private/local IP、发生 redirect、使用 proxy 等安全违规场景被拒绝。
8. Provider 超时、非 2xx、非 JSON、响应体过大、mapping 失败、字段缺失、类型不匹配均有稳定错误码和脱敏原因。
9. Provider 成功返回并完成 mapping 后，规则可以读取 `facts.<factCode>.<path>`，并输出 field、expected、actual、passed、reasonCode、reason。
10. Provider 查询失败时，依赖该 fact 的规则不通过；活动因此拒绝时，不生成 grant_order / outbox，不扣 quota，并能在 participation 快照或拒绝原因中追踪。
11. Provider 查询失败只影响依赖该 factCode 的规则或活动，不影响不依赖该 factCode 的活动。
12. 同一事件处理上下文内多个规则依赖同一 factCode 时，不重复发起不必要查询，并复用同一成功结果或失败原因。
13. `cacheTtlSeconds` 缺失或 `0` 时不跨事件缓存；负数配置被拒绝；正数缓存不会跨 tenant、subject、biz、provider 配置版本错误复用。
14. 模拟运行展示 facts 查询成功结果、缓存命中状态或失败原因，且不写 inbox、participation、grant_order、outbox、quota。
15. `/openapi/v1/facts/query` 返回机器可识别响应，成功包含 mapped facts，失败包含 `errorCode` 和脱敏 `error`。
16. 错误响应、规则解释、模拟结果、日志摘要不得泄漏 Authorization、token、完整 headers、内部网络地址或 provider 原始敏感响应。
17. 正式事件处理继续遵守现有 Event API 幂等、规则阶段、quota 原子扣减、grant_order 和 outbox 语义。

## 待确认问题

以下问题不阻塞本任务按最小口径进入 Review / Dev，但需要在实现或后续生产化前确认：

1. Provider 配置最终管理入口是后台 API、数据库预置、配置文件，还是三者并存？
2. `tenantId` 是否需要补充进 `marketing_fact_provider` 的正式数据字典，还是通过现有环境隔离表达？
3. `authRef` 是否进入本任务，还是留到 Phase 5 的配置中心和密钥托管统一处理？
4. `cacheTtlSeconds` 最大值是否采用建议上限 `3600` 秒，还是按事实类型配置不同上限？
5. Provider 请求超时时间、最大响应体大小和允许 header 白名单的具体数值是多少？
6. `/openapi/v1/facts/query` 当前是否需要鉴权；若本地开发暂不鉴权，生产开放前必须补充哪种鉴权或网络隔离？
7. 是否需要把 Fact Query 的调用摘要写入 `marketing_audit`，还是仅进入 participation snapshot / 日志？

## Score

```text
需求清晰度        18 / 20
需求边界清楚      18 / 20
风险可控          17 / 20
验收标准明确      18 / 20
约束一致性        18 / 20
总分              89 / 100
```

## Decision

```text
PASS
```

理由：本轮已把上一轮阻塞项收敛为明确、可审核、可验收的最小需求口径：HTTP-only provider、显式 mapping、provider 必须可被后台校验和运行时读取、缓存 TTL 和 key 隔离规则、失败即拒绝依赖规则、安全错误码和脱敏要求。剩余问题主要是生产化管理入口、鉴权、密钥托管和具体数值参数，不影响按当前最小契约进入 Review / Dev。
