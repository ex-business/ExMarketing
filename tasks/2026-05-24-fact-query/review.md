# Review

## Findings

~~~text
P0:
- 无。

P1:
- 无。

P2:
- 无。
~~~

## Review Notes

- `analysis.md:73-92` 已将 provider registry 固定为 `marketing_fact_provider` 或等价持久化字段，并明确 `tenant_id + fact_code` 唯一、`enabled`、`mapping_json`、`cache_ttl_seconds`、`updated_at` 等关键契约，覆盖 `task.md:17-38` 的 Owner 决策。
- `analysis.md:94-120` 已收敛 `/openapi/v1/facts/query` 输入白名单，只接受 `tenantId`、`factCode`、`subject`、`biz`、`attributes`、`traceId` 或 request id，并明确拒绝 `eventType`、`sourceSystem`、`occurredAt` 顶层输入及所有 inline provider 配置。
- `analysis.md:167-187` 已明确现有 `POST /admin/v1/activities` 在保存或上线时同步校验 `facts.<factCode>.<path>` 引用，包括同 tenant enabled HTTP provider、schema、mapping 字段覆盖和操作符类型兼容。
- `analysis.md:271-292` 与 `analysis.md:368-369` 已固定 `cacheTtlSeconds` 语义：缺失或 `0` 不跨事件缓存，`1..3600` 缓存，负数或大于 `3600` 拒绝并返回 `FACT_CACHE_TTL_INVALID` 或等价配置错误。
- `analysis.md:232-249`、`analysis.md:274-278`、`analysis.md:370-378` 已明确 provider 查询失败不会让依赖 fact 的规则默认通过，且不改变 participation、grant_order、outbox、quota 的既有生命周期。

## Open Questions

1. 无阻塞问题。`analysis.md:380-388` 中的 Provider 管理入口、密钥管理、响应体大小、生产鉴权和审计落点属于后续生产化细节，不影响当前最小可验收契约进入 Dev。

## Score

~~~text
需求清晰度        19 / 20
需求边界清楚      19 / 20
风险可控          18 / 20
验收标准明确      19 / 20
约束一致性        19 / 20
总分              94 / 100
~~~

## Decision

~~~text
PASS
~~~

理由：上一轮阻塞点已经在当前 `analysis.md` 中收敛。Provider registry 唯一来源、禁止 inline provider、后台活动同步校验入口、`/facts/query` 输入白名单、租户隔离、mapping 契约、缓存 TTL 上限和失败降级语义均具备可实现、可测试的验收口径。当前无 P0/P1，Score 94。

需求：2026-05-24-fact-query
