# Analysis

## 需求 ID

2026-05-24-fact-provider

## Review 反馈处理

本轮为 Review 打回后的返工，已逐条处理 `review.md` 中的 P1/P2 和 Open Questions。

### P1-1：mapping 缺少最小 JSON 结构契约

处理结论：已关闭。

本轮明确 `mapping_json` 的最小结构固定为：

```json
{
  "<outputPath>": {
    "source": "$.<providerResponsePath>",
    "type": "string|number|boolean|array|object",
    "required": true
  }
}
```

约束如下：

- key 是输出字段相对路径，不包含 `facts.<factCode>.` 前缀。例如规则字段 `facts.USER_PROFILE.vipLevel` 对应 mapping key `vipLevel`。
- key 支持点号分隔的对象路径，例如 `risk.score`，最终输出到 `facts.<factCode>.risk.score`。
- value 必须包含 `source` 和 `type`；`required` 可省略，默认 `true`。
- `source` 只支持从 provider HTTP JSON 响应读取的简单 JSONPath 子集：`$` 起始、点号对象访问和数组数字下标，例如 `$.data.vipLevel`、`$.data.tags[0]`。不支持通配符、过滤表达式、脚本、函数、递归搜索或跨字段计算。
- `type` 只允许 `string`、`number`、`boolean`、`array`、`object`。
- 空 mapping、非对象 mapping、缺少 `source` / `type`、非法 `source`、非法 `type` 均属于 provider schema 非法，不能被规则引用或 Fact Query 使用。

### P1-2：规则保存/上线未要求 mapping 覆盖被引用字段

处理结论：已关闭。

本轮明确：后台创建、更新、保存或上线活动时，凡规则字段引用 `facts.<factCode>.<path>`，必须校验同租户 enabled provider 的 `mapping_json` 覆盖每个被引用的 `<path>`。若 provider 存在但 mapping 不包含该字段，必须配置期拒绝，稳定错误码固定为 `FACT_FIELD_MISSING`。

示例：Provider `USER_PROFILE` 的 mapping 只有 `depositCount30d`，规则引用 `facts.USER_PROFILE.vipLevel` 时，即使 provider 已启用，也必须拒绝保存或上线，错误指向 `factCode=USER_PROFILE`、`fieldPath=vipLevel`、`code=FACT_FIELD_MISSING`。

### P2-1：factCode 唯一性表述不一致

处理结论：已关闭。

本轮统一为：`tenant_id + fact_code` 是唯一业务键，同一租户不得存在重复 `factCode` 记录，不区分 enabled 或 disabled；不同租户可以复用相同 `factCode`。

### P2-2：Provider 配置入口验收路径偏松

处理结论：已关闭。

本轮给出默认最小可验收口径：Provider 配置必须通过受控配置入口写入正式 registry，并被活动规则校验和 Fact Query 使用同一份 registry。入口可以复用现有 Admin API/后台边界；若当前阶段未提供完整 CRUD 页面，至少必须有一条可审计、可复现的配置写入路径，例如 Admin API、受控 seed/fixture 或迁移初始化数据。不得只存在内存 inline provider、测试内硬编码 provider 或请求内 provider。

端到端验收必须能先写入/预置 `marketing_fact_provider`，再验证规则保存校验和 `/openapi/v1/facts/query` 都按 `tenant_id + fact_code` 读取同一份配置。

### Open Question-1：mapping 是否沿用 `{ "<outputPath>": { "source": "$.<path>", "type": "...", "required": true|false } }`

处理结论：采用该结构作为默认最小口径，并细化字段必填性和路径语法。`required` 可选，默认 `true`；当显式为 `false` 时，Fact Query 缺失该字段可不使整次 mapping 失败，但规则若引用该字段且运行时缺失，规则不得默认通过，应返回字段缺失类原因。

### Open Question-2：缺少 mapping 字段时错误码如何归类

处理结论：固定为 `FACT_FIELD_MISSING`。区分规则如下：

- 找不到同租户 provider：`FACT_PROVIDER_NOT_CONFIGURED`。
- provider 存在但 `enabled=false`：`FACT_PROVIDER_DISABLED`。
- provider schema 非法，包括 mapping 非对象、空 mapping、source/type 非法：`FACT_PROVIDER_SCHEMA_INVALID`。
- provider enabled 且 schema 合法，但 mapping 未覆盖规则引用字段：`FACT_FIELD_MISSING`。

## Analysis 自检修正

上一轮 `analysis.md` 已给出 `PASS`，但 Review 判定仍存在 P1 且 Score 78，需要返工。本轮自检修正如下：

- 扣分原因“mapping 结构不清”：已补充最小 JSON 结构、字段必填性、outputPath 语义、source path 语法、type 枚举、required 默认值和非法 mapping 拒绝规则。
- 扣分原因“验收未覆盖 mapping 字段覆盖”：已补充规则保存/上线前必须检查 mapping 覆盖每个 `facts.<factCode>.<path>` 引用，并固定 `FACT_FIELD_MISSING`。
- 扣分原因“factCode 唯一性表述歧义”：已统一为同租户 `tenant_id + fact_code` 唯一，不允许重复记录。
- 扣分原因“配置入口不明确”：已补充最小配置入口必须写入正式 registry，活动规则校验和 Fact Query 必须读取同一份配置。
- 错误假设修正：不再把“存在 enabled provider”视为规则字段可引用的充分条件；还必须满足 provider schema 合法且 mapping 覆盖具体字段。

## 背景目标

ExMarketing 是独立活动营销中台，通过标准事件、可选 Fact Query、Reward Command 和 Callback 与外部系统解耦。活动规则的数据来源优先是标准事件字段、`attributes.*` 和平台自身 `state.*`；当事件无法携带完整用户画像、历史统计、风控结果等外部事实时，才通过 Fact Provider 暴露为 `facts.<factCode>.<path>`。

本需求来自《22-独立活动营销中台设计》和现有 `docs/api.md`、`docs/data-model.md` 的 Fact Query / `marketing_fact_provider` 数据字典。目标是补齐租户级 Fact Provider 配置模型，使后台活动规则可以安全、可审计地引用 facts 字段，并在配置期拒绝不可解析、未授权或未映射的事实字段。

默认最小可验收口径：

- 只支持 HTTP Fact Provider。
- 正式 registry 承载对象为 `marketing_fact_provider` 或等价持久化配置，业务唯一键为 `tenant_id + fact_code`。
- 活动规则引用 `facts.<factCode>.<path>` 时，必须存在同租户 enabled provider，provider schema 合法，并且 mapping 覆盖 `<path>`。
- Fact Query 请求只接受 registry key 和事件上下文，不接受 inline endpoint、auth、mapping 或 provider 配置。
- 不依赖 `facts.*` 的活动不需要配置 Fact Provider，也不应被 Fact Provider 缺失影响。

## 参与方

- 活动运营/配置人员：配置活动规则，可能引用 `facts.<factCode>.<path>`。
- Provider 配置维护方：通过受控配置入口维护租户级 provider registry。
- 外部业务系统：提供 HTTP Provider，返回用户画像、历史统计、风控结果等 JSON 响应。
- ExMarketing Admin/API：保存 provider 配置和活动规则，执行配置期校验。
- ExMarketing Rule Engine：运行时读取标准事件、平台状态和已映射 facts。
- ExMarketing Fact Query API：按 registry 查询 HTTP Provider、执行 mapping、返回 facts 或错误原因。
- 安全/运维：维护 endpoint allowlist、密钥引用、超时、缓存和脱敏策略。

## 输入输出

### Provider 配置输入

Provider 配置最小字段：

```text
tenantId / tenant_id
factCode / fact_code
providerType / provider_type
endpoint
method
authRef / auth_ref
mapping / mappingJson / mapping_json
cacheTtlSeconds / cache_ttl_seconds
enabled
timeoutMs / timeout_ms
createdAt / created_at
updatedAt / updated_at
```

字段语义：

| 字段 | 要求 |
|---|---|
| `tenantId` / `tenant_id` | 必填。Provider、规则校验、Fact Query 都按 tenant 隔离。 |
| `factCode` / `fact_code` | 必填。用于规则路径 `facts.<factCode>.<path>`。同一 tenant 内唯一，建议格式 `^[A-Z][A-Z0-9_]{1,63}$`。 |
| `providerType` / `provider_type` | 必填。本期只允许 `HTTP`。 |
| `endpoint` | 必填。HTTP endpoint；配置期至少拒绝空值和明显非法 URL，运行时必须遵守 allowlist、private/local IP、redirect、proxy 安全约束。 |
| `method` | 必填。只允许 `GET` / `POST`。 |
| `authRef` / `auth_ref` | 可选。只保存鉴权配置引用；不得保存明文 secret、token 或完整 Authorization header。 |
| `mappingJson` / `mapping_json` | 必填。显式声明 provider JSON 响应到 `facts.<factCode>.*` 的字段映射。 |
| `cacheTtlSeconds` / `cache_ttl_seconds` | 必填或有默认值。`0` 不缓存，`1..3600` 启用缓存；负数或大于 `3600` 拒绝。 |
| `enabled` | 必填。只有 `true` 且 schema 合法的 provider 可被规则引用和 Fact Query 使用。 |
| `timeoutMs` / `timeout_ms` | 必填或有安全默认值。超时必须返回稳定错误码。 |
| `createdAt` / `created_at` | 审计元数据。 |
| `updatedAt` / `updated_at` | 审计元数据，也可作为配置版本或缓存隔离依据。 |

### Mapping JSON 输入

`mapping_json` 是 provider 配置的核心字段，最小结构如下：

```json
{
  "vipLevel": {
    "source": "$.data.vipLevel",
    "type": "string",
    "required": true
  },
  "depositCount30d": {
    "source": "$.data.stats.depositCount30d",
    "type": "number"
  },
  "risk.score": {
    "source": "$.risk.score",
    "type": "number",
    "required": false
  }
}
```

Mapping 规则：

- `mapping_json` 必须是非空对象。
- key 是输出字段相对路径，不包含 `facts.<factCode>.` 前缀。
- key 只能表示输出到 `facts.<factCode>.<outputPath>` 的字段路径，例如 `vipLevel`、`depositCount30d`、`risk.score`。
- key 不允许为空，不允许以 `.` 开头或结尾，不允许包含连续点号，不允许包含 `facts.` 前缀。
- key 不支持数组通配输出；如需数组整体输出，key 指向字段，`type=array`。
- value 必须是对象，至少包含 `source` 和 `type`。
- `source` 必须是从 provider JSON 响应读取的简单 JSONPath 子集：`$` 起始，支持点号对象路径和数字数组下标，例如 `$.data.vipLevel`、`$.data.tags[0]`。
- `source` 不支持 `*`、`..`、`?()`、脚本、函数、表达式计算、外部请求或读取事件上下文。
- `type` 只允许 `string`、`number`、`boolean`、`array`、`object`。
- `required` 可选，默认 `true`；只允许 boolean。
- 空 mapping、非法 source、非法 type、value 非对象、缺少 `source` 或 `type`，均使 provider schema 非法。

Mapping 输出示例：

Provider 原始响应：

```json
{
  "data": {
    "vipLevel": "VIP3",
    "stats": {
      "depositCount30d": 5
    }
  }
}
```

映射后的 facts：

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

活动规则字段可以引用：

```text
facts.<factCode>.<path>
```

合法示例：

```json
{
  "field": "facts.USER_PROFILE.vipLevel",
  "op": "==",
  "value": "VIP3"
}
```

语法非法示例：

```text
facts
facts.USER_PROFILE
facts..vipLevel
facts.USER_PROFILE.
```

需要 mapping 覆盖的嵌套路径示例：

```text
facts.USER_PROFILE.vipLevel.extra
```

说明：`facts.USER_PROFILE.vipLevel.extra` 本身是语法合法的嵌套路径，但只有当 mapping key 明确存在 `vipLevel.extra` 时才可通过；若 mapping 只存在 `vipLevel`，则必须按 `FACT_FIELD_MISSING` 拒绝。

### Fact Query API 输入

`POST /openapi/v1/facts/query` 本期只接受：

```text
tenantId
factCode
subject
biz
attributes
traceId 或 request id
```

不得接受：

```text
inline endpoint
inline method
inline headers
inline auth
inline mapping
inline provider
```

服务端必须按 `tenantId + factCode` 查找 registry 中 enabled 且 schema 合法的 provider。

### 输出

配置期输出：

- Provider 配置保存成功后，进入正式 registry。
- 活动规则保存/上线前校验成功时，说明所引用 `facts.<factCode>.<path>` 均可由同租户 enabled provider 的 mapping 解析。
- 校验失败时返回稳定错误码、`factCode`、字段路径和脱敏错误信息。

Fact Query 成功输出：

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

Fact Query 失败输出至少包含：

```text
tenantId
factCode
success=false
cached
errorCode
error
providerStatus 或安全拦截摘要
```

错误信息不得输出明文鉴权、完整敏感响应或内部网络细节。

## 业务规则

1. Fact Provider 是可选能力；不依赖 `facts.*` 的活动不需要配置 provider。
2. 能通过事件 `attributes.*` 携带的数据，不应强制实时 Fact Query。
3. Provider registry 是唯一正式配置来源，活动规则和 Fact Query 请求不得携带 inline provider 配置。
4. `tenant_id + fact_code` 是唯一业务键；同一租户不得存在重复 `factCode`，不同租户可复用同名 `factCode`。
5. 当前 provider 类型只接受 `HTTP`。
6. 当前 method 只接受 `GET` / `POST`。
7. `authRef` 只能是引用，不存放明文密钥。
8. `mapping_json` 必须是显式、非空、结构合法的字段映射，不允许脚本、外部数据库读取、Provider SDK、通配数组、跨字段计算或原始响应整体透传。
9. `cacheTtlSeconds=0` 表示不缓存；`1..3600` 表示允许缓存。
10. `enabled=false` 的 provider 不允许规则引用，也不允许 Fact Query 执行。
11. 后台创建、更新、保存或上线活动时，只要规则字段引用 `facts.<factCode>.<path>`，必须校验同租户 provider 存在、enabled、schema 合法且 mapping 覆盖 `<path>`。
12. `enabled=false` 的规则也不能绕过字段合法性校验；非法或未配置的 `facts.*` 引用仍应拒绝保存。
13. Provider 配置不改变规则操作符语义；比较、IN/NOT_IN、字段缺失、类型不匹配按现有规则引擎口径处理，但不得默认通过。
14. Provider endpoint 运行时必须继续遵守 Fact Query 安全约束：host allowlist、拒绝 private/local IP、禁止 redirect、禁用 proxy。
15. Provider schema 非法时，即使 `enabled=true`，也不能被规则引用或 Fact Query 使用。
16. 规则引用字段与 mapping 类型存在明显不可比较关系时，应在配置期拒绝；例如 mapping 声明 `object` 却使用 `>=`。
17. 配置入口必须写入正式 registry，且活动规则校验与 Fact Query 必须读取同一份 registry。

## 边界条件

- 多租户：`site_001` 配置的 `USER_PROFILE` 不授权 `site_002` 活动引用或查询。
- 重复 factCode：同一 tenant 中 `USER_PROFILE` 只能有一条 provider 记录，不允许通过 disabled 记录制造重复配置。
- Provider 禁用：历史活动若已引用该 factCode，再次保存或上线前校验应失败；本轮不要求自动下线已运行活动。
- Provider schema 非法：空 mapping、非法 endpoint、非法 method、非法 cache TTL、非法 timeout、非法 mapping path 均不能被引用。
- Mapping 字段覆盖：一个活动可引用多个 facts 字段，每个 `<factCode>.<path>` 都必须被对应 provider mapping 覆盖。
- 多 factCode：同一活动可以引用多个 factCode，但每个 factCode 都必须独立通过 provider 与 mapping 校验。
- 嵌套字段：`facts.USER_PROFILE.risk.score` 对应 mapping key `risk.score`；只配置 `risk` 不代表覆盖 `risk.score`。
- 可选 mapping 字段：`required=false` 只影响 Fact Query mapping 缺失时是否使整次查询失败，不放宽配置期字段覆盖要求；规则引用该 key 时仍必须存在 mapping key。
- endpoint 变更：不要求本轮定义灰度、版本化或历史活动快照重算；但缓存 key 应避免配置变更后复用旧 provider 结果，可使用 `updated_at` 或等价配置版本。
- 缓存：本任务定义 TTL 和隔离口径；不要求实现跨实例分布式缓存一致性。
- 配置入口：不要求完整可视化后台页面；但必须有受控、可审计、可复现的 registry 写入路径供验收。

## 异常场景

| 场景 | 期望 | 错误码 |
|---|---|---|
| 规则引用的 provider 不存在 | 拒绝保存/上线；Fact Query 失败 | `FACT_PROVIDER_NOT_CONFIGURED` |
| provider 存在但 disabled | 拒绝保存/上线；Fact Query 失败 | `FACT_PROVIDER_DISABLED` |
| provider 类型不是 HTTP | 拒绝配置或视为 schema 非法 | `FACT_PROVIDER_SCHEMA_INVALID` |
| method 非 GET/POST | 拒绝配置或视为 schema 非法 | `FACT_PROVIDER_SCHEMA_INVALID` |
| mapping 为空或非对象 | 拒绝配置或视为 schema 非法 | `FACT_PROVIDER_SCHEMA_INVALID` |
| mapping value 缺少 source/type | 拒绝配置或视为 schema 非法 | `FACT_PROVIDER_SCHEMA_INVALID` |
| source path 使用通配、过滤、脚本等高级 JSONPath | 拒绝配置或视为 schema 非法 | `FACT_PROVIDER_SCHEMA_INVALID` |
| 规则引用字段未被 mapping 覆盖 | 配置期拒绝，不保存/不上线 | `FACT_FIELD_MISSING` |
| provider 返回 JSON 中缺少 required=true 字段 | Fact Query 失败；依赖规则不得通过 | `FACT_FIELD_MISSING` |
| provider 返回字段类型与 mapping type 不一致 | Fact Query 失败；依赖规则不得通过 | `FACT_FIELD_TYPE_MISMATCH` |
| endpoint 为空或 URL 明显非法 | 拒绝配置或视为 schema 非法 | `FACT_PROVIDER_SCHEMA_INVALID` |
| endpoint host 不在 allowlist | 运行时拒绝 | `FACT_PROVIDER_HOST_NOT_ALLOWED` |
| endpoint 解析到 private/local IP | 运行时拒绝 | `FACT_PROVIDER_HOST_NOT_ALLOWED` |
| provider redirect | 运行时拒绝 | `FACT_PROVIDER_REDIRECT_NOT_ALLOWED` |
| provider HTTP 非 2xx 或网络失败 | Fact Query 失败 | `FACT_PROVIDER_HTTP_FAILED` |
| provider 超时 | Fact Query 失败 | `FACT_PROVIDER_TIMEOUT` |
| 请求携带 inline provider | Fact Query 请求拒绝 | `FACT_INLINE_PROVIDER_NOT_ALLOWED` |
| authRef 为空 | 允许无鉴权 provider | 无错误 |
| authRef 包含明文 token/secret/header | 拒绝配置或脱敏审计失败 | `FACT_PROVIDER_SCHEMA_INVALID` |

## 非目标范围

- 不实现 SQL、gRPC、MQ、SDK、本地插件、外部业务库直连等 Provider 类型。
- 不允许通过读取外部业务数据库替代标准 Fact Provider。
- 不设计完整生产密钥管理系统；本轮只要求 `authRef` 引用和脱敏。
- 不要求新增完整运营后台页面、审批流、导入导出、版本回滚或权限体系。
- 不要求实现通用 JSONPath 高级能力、脚本 mapping、数组过滤、字段计算或原始响应透传。
- 不要求所有活动都配置 Fact Provider。
- 不要求 Fact Query 成为活动执行链路的强制步骤。
- 不要求解决 Provider SLA、熔断、降级策略、批量预取或分布式缓存一致性。
- 不改变标准事件模型、活动匹配、规则操作符、奖励发放、quota、outbox、callback 或幂等语义。
- 不要求 Provider 禁用后自动暂停或迁移历史 ACTIVE 活动。

## 风险

- 安全风险：endpoint 若未受 allowlist、private/local IP、redirect、proxy 约束，会形成 SSRF 风险。需求要求保持现有 Fact Query 安全边界。
- 配置漂移风险：规则引用 facts 但 provider 被删除、禁用或 mapping 被改动，会导致运行时不可解释失败。需求要求配置期校验，并建议缓存隔离包含配置版本。
- 字段漂移风险：外部 provider 响应结构变化会使 mapping 失败。需求要求 `FACT_FIELD_MISSING` / `FACT_FIELD_TYPE_MISMATCH` 可解释输出。
- 租户隔离风险：factCode 跨租户复用时容易误引用。需求要求按 `tenant_id + fact_code` 定位。
- 密钥泄露风险：若保存 token 或 Authorization header，会进入配置、日志或审计。需求要求只保存 `authRef` 并脱敏错误。
- 性能风险：实时 Fact Query 可能拖慢活动处理。需求保留可选能力，并通过 `cacheTtlSeconds` 约束缓存口径。
- 运营理解风险：事件 `attributes.*` 与 `facts.*` 都能供规则使用，需明确优先使用 attributes，facts 仅补充事件无法携带的数据。
- 验收偏差风险：若只测试 provider 存在性而不测试 mapping 覆盖，会放过运行时失败。需求已将 mapping 覆盖列为配置期强验收。

## 验收标准

1. 能表达并保存租户级 Fact Provider 配置，字段覆盖 `tenantId/factCode/providerType/endpoint/method/authRef/mappingJson/cacheTtlSeconds/enabled/timeoutMs/createdAt/updatedAt` 或等价数据字典字段。
2. Provider 配置必须进入正式 registry；活动规则校验和 `/openapi/v1/facts/query` 必须读取同一份 registry，不允许测试内硬编码或请求内 inline provider 替代。
3. 同一租户下 `factCode` 唯一；重复 `tenant_id + fact_code` 配置被拒绝；不同租户可复用同名 `factCode`。
4. 当前只接受 `HTTP` provider；非法 provider 类型被拒绝或不能通过 schema 校验。
5. 当前只接受 `GET` / `POST` method；非法 method 被拒绝或不能通过 schema 校验。
6. `cacheTtlSeconds` 只接受 `0..3600`。
7. `authRef` 不保存明文 secret/token/header；日志、错误响应和规则解释不得输出完整鉴权值。
8. `mapping_json` 必须是非空对象，结构满足 `{ "<outputPath>": { "source": "$.<path>", "type": "...", "required": true|false } }`。
9. mapping key 必须是输出字段相对路径；不能包含 `facts.<factCode>.` 前缀；非法空路径、连续点号、首尾点号必须被拒绝。
10. mapping `source` 只支持简单 JSONPath 子集；通配、过滤、脚本、函数、递归搜索等高级语法必须被拒绝。
11. mapping `type` 只接受 `string|number|boolean|array|object`；非法 type 必须被拒绝。
12. 后台创建或更新活动时，规则字段引用已配置、enabled、schema 合法且 mapping 覆盖的 `facts.USER_PROFILE.vipLevel` 可以通过字段合法性校验。
13. 后台创建或更新活动时，规则字段引用未配置的 `facts.USER_PROFILE.vipLevel` 必须被拒绝，错误码为 `FACT_PROVIDER_NOT_CONFIGURED` 或等价 provider 缺失配置错误。
14. 后台创建或更新活动时，规则字段引用 disabled provider 的 factCode 必须被拒绝，错误码为 `FACT_PROVIDER_DISABLED`。
15. 后台创建或更新活动时，provider 存在且 enabled，但 mapping 未覆盖 `vipLevel`，必须被拒绝，错误码固定为 `FACT_FIELD_MISSING`。
16. `enabled=false` 的规则引用未配置或未映射 facts 也必须被拒绝，不能以禁用规则绕过保存校验。
17. `facts`、`facts.USER_PROFILE`、`facts..vipLevel`、`facts.USER_PROFILE.` 等不完整或非法路径必须被拒绝。
18. `facts.USER_PROFILE.risk.score` 只有在 mapping key 存在 `risk.score` 时才允许；只存在 `risk` 不视为覆盖。
19. Fact Query 请求不得接受 inline endpoint、auth、mapping 或 provider 配置，只能按 `tenantId + factCode` 查 registry。
20. Fact Query 对未配置、disabled、schema 非法、HTTP 失败、超时、mapping 字段缺失、mapping 类型不匹配输出稳定错误码。
21. endpoint 运行时安全约束不被削弱：allowlist、拒绝 private/local IP、禁止 redirect、禁用 proxy。
22. 不依赖 `facts.*` 的活动可以仅凭标准事件、`attributes.*` 和 `state.*` 保存、模拟和执行，未配置 Fact Provider 不影响。
23. 相关 API/数据模型文档或任务交付说明能说明 Provider 配置字段、mapping 结构、规则引用校验、错误码和最小配置入口。

## 待确认问题

以下问题不阻塞本轮进入研发；均已给出默认最小可验收口径。

- 是否必须新增完整 Provider Admin CRUD API？默认口径：不强制完整页面或完整 CRUD；但必须存在至少一条受控、可审计、可复现的 registry 写入路径，优先复用现有 Admin API/后台边界，或使用受控 seed/fixture/迁移初始化数据完成验收。
- 是否必须支持 `timeoutMs` 管理能力？默认口径：字段语义必须存在或有安全默认值；本任务验收关注 provider schema、引用校验和错误口径，不把完整 timeout 配置页面作为强制范围。
- `required=false` 是否允许规则引用字段缺失时通过？默认口径：不允许默认通过。`required=false` 只表示 Fact Query 可在响应中缺失该字段而不使整个 provider mapping 失败；若规则实际读取缺失字段，该规则按字段缺失失败或不可评估处理。
- 是否支持完整 JSONPath？默认口径：不支持。只支持简单 `$` 起始对象路径和数字数组下标，避免脚本执行和不可控 mapping。
- Provider 禁用后历史 ACTIVE 活动如何处理？默认口径：本需求验收聚焦保存/上线前校验和后续更新拦截；不要求自动暂停历史活动。

## Score

```text
需求清晰度        19/20
范围可控          18/20
风险可控          18/20
验收标准明确      19/20
架构一致性        18/20
总分              92/100
```

扣分说明：不强制指定唯一的 Provider 配置入口形态，保留 Admin API、受控 seed/fixture 或迁移初始化数据等受控路径；但已要求必须写入正式 registry 并用于端到端验收，因此不阻塞研发和测试。

## Decision

```text
PASS
```

需求：2026-05-24-fact-provider
