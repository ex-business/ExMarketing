# Review

## Findings

~~~text
P0:
- 无。

P1:
- tasks/2026-05-24-fact-query/analysis.md:59-76,144-180,320-327: Provider 配置契约仍未收敛，不能进入 Dev。task.md 明确要求“标准 Fact Provider 配置”和“未配置 provider 时后台不允许配置依赖该事实源的规则”，analysis.md 也把“只有已配置、已校验、当前启用的 HTTP Fact Provider 才允许被活动规则引用”列为核心口径；但文档随后允许 provider 配置来自预置表、后台配置或本地开发配置，并把最终管理入口、tenantId 是否进入正式数据字典、/openapi/v1/facts/query 是否鉴权等放入“不阻塞”的待确认项。现有 data-model 又注明 marketing_fact_provider 当前无 tenantId、mapping、enabled 且运行时未从表加载 provider。这个缺口会让 Dev 无法判断应新增/修改哪个持久化契约、后台创建活动到底校验哪个 provider registry、/facts/query 是按 factCode 查已配置 provider 还是继续接受 inline provider。若按 inline provider 实现，会直接绕过“未配置 provider 不允许依赖”的验收；若各自选择配置来源，又会产生不可测试的行为差异。

P2:
- tasks/2026-05-24-fact-query/analysis.md:73,242,311,324: cacheTtlSeconds 上限只写“建议不超过 3600 秒”且最大值仍待确认。该项不会单独阻塞需求分析，但会降低测试计划的确定性；建议在 analysis 中固定本任务的行为为“超过 3600 拒绝”或“超过 3600 按 3600 截断”，并指定可观察响应。
~~~

## Open Questions

1. 本任务的 provider registry 是否以 `marketing_fact_provider` 为唯一来源？如果是，analysis 需要明确新增/调整字段：`tenant_id`、`fact_code` 唯一约束、`mapping_json`、`enabled`、配置版本或更新时间。
2. `/openapi/v1/facts/query` 的请求应按 `tenantId + factCode + event` 查询已配置 provider，还是允许 debug-only inline provider？如果允许 inline，需要明确它不能作为活动规则配置校验依据，并且安全/脱敏/错误码完全一致。
3. 后台活动创建校验 provider 的入口是现有 `POST /admin/v1/activities` 同步校验，还是新增 Fact Provider 管理 API 后再引用？

## Score

~~~text
需求清晰度        15 / 20
需求边界清楚      17 / 20
风险可控          15 / 20
验收标准明确      17 / 20
约束一致性        14 / 20
总分              78 / 100
~~~

## Decision

~~~text
BLOCK
~~~

理由：存在 P1，且 Score < 80。analysis.md 已覆盖 Fact Query 的大部分安全、mapping、缓存和失败语义，但 provider 配置来源与运行时/后台校验契约没有固定，正好落在本需求的核心验收点上。需要先把 provider registry、tenant 隔离、mapping/enabled 持久化字段和 `/facts/query` 输入契约收敛后再进入 Dev。

需求：2026-05-24-fact-query
