# Task

## 标题

Fact Query 可选事实查询

## 需求 ID

2026-05-24-fact-query

## 背景

来源：22-独立活动营销中台设计。实现标准 Fact Provider 配置、HTTP 查询、字段 mapping、cacheTtlSeconds、facts.* 规则字段读取。Fact Query 必须是可选能力，未配置 provider 时后台不允许配置依赖该事实源的规则。验收：事件 attributes 可独立完成判断；facts 查询失败有降级和错误原因。

## Owner 补充决策

### Provider Registry 契约

本期采用持久化 provider registry，不允许活动规则或 `/openapi/v1/facts/query` 直接传 inline provider 配置绕过 registry。

Fact Provider 以 `tenantId + factCode` 唯一定位。

Provider 字段：

- `tenantId`
- `factCode`
- `providerType`，本期只支持 `HTTP`
- `endpoint`
- `method`，本期支持 `GET` / `POST`
- `authRef`
- `mapping`
- `cacheTtlSeconds`
- `enabled`
- `timeoutMs`
- `createdAt`
- `updatedAt`

只有 `enabled = true` 且 schema 校验通过的 provider 才能被规则引用和 Fact Query 使用。

### 规则配置校验

后台配置规则时，如果规则引用 `facts.<factCode>.*`，必须校验当前 tenant 下存在已启用的 `factCode` provider。

未配置 provider 时：

- 规则保存或上线校验必须失败。
- 错误信息必须指出缺失的 `factCode`。
- 不能在运行时临时接受 inline provider 配置。

事件 `attributes` 足够完成判断时，不强制配置 Fact Provider。

### Fact Query API 输入契约

`POST /openapi/v1/facts/query` 本期只接受：

- `tenantId`
- `factCode`
- `subject`
- `biz`
- `attributes`
- `traceId` 或 request id

不接受 inline endpoint、inline auth、inline mapping 或 inline provider。

服务端按 `tenantId + factCode` 查找 enabled provider，完成 HTTP 查询、mapping、缓存和错误降级。

### 租户隔离

provider registry、规则校验、Fact Query 查询都必须按 `tenantId` 隔离。

跨 tenant 不能读取、引用或执行 provider。

### Mapping 与缓存

`mapping` 负责把外部 HTTP 响应映射为标准 `facts.<factCode>.*` 字段。

`cacheTtlSeconds` 为 0 表示不缓存，大于 0 表示按 tenant、factCode、subject、biz 和必要 attributes 组成缓存 key。

### 失败降级

Fact Query 是可选能力。provider 不存在、未启用、HTTP 失败、超时、mapping 失败或缓存失败时，必须返回可解释错误原因。

规则执行时是否阻断由规则配置决定，但失败原因必须进入 rule result / simulation explain。

## 目标

- 实现标准 Fact Provider 配置、HTTP 查询、字段 mapping、缓存 TTL 和 facts.* 规则字段读取。
- 保证 Fact Query 是可选能力，attributes 足够时不依赖 provider。
- 保证未配置 provider 时，后台不允许配置依赖该事实源的规则。

## 范围

- Provider registry 数据契约。
- Provider 创建/校验/启用状态。
- `facts.*` 规则引用校验。
- `POST /openapi/v1/facts/query` 按 registry 查询 provider。
- HTTP Fact Provider 查询。
- 响应 mapping。
- cacheTtlSeconds 缓存语义。
- 失败降级和错误原因。

## 非目标

- MQ / gRPC / SQL provider。
- inline provider 查询。
- 跨租户共享 provider。
- 完整生产密钥管理；`authRef` 只引用已有或后续接入的密钥配置。
- 前端规则编辑器完整交互。

## 风险等级

中

## 验收标准

- 当前 tenant 未配置 enabled provider 时，后台不允许保存或上线引用 `facts.<factCode>.*` 的规则。
- 已配置 enabled HTTP provider 后，规则可以引用对应 `facts.<factCode>.*` 字段。
- `/openapi/v1/facts/query` 不接受 inline provider，只按 `tenantId + factCode` 查 registry。
- 跨 tenant 不能引用或查询其他 tenant 的 provider。
- HTTP 查询成功后，mapping 输出标准 facts 字段。
- `cacheTtlSeconds = 0` 时不缓存；大于 0 时按 tenant、factCode、subject、biz 和必要 attributes 命中缓存。
- provider 不存在、未启用、HTTP 失败、超时、mapping 失败时返回明确错误原因。
- 事件 attributes 足够完成判断时，不强制配置 Fact Provider。
