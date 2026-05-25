# Review

## Findings

~~~text
P0:
- 无。

P1:
- tasks/2026-05-24-fact-query/analysis.md:113-122,374: `/openapi/v1/facts/query` 输入契约仍未完全收敛，不能进入 Dev。task.md:54-63 的 Owner 决策明确写的是“本期只接受” `tenantId`、`factCode`、`subject`、`biz`、`attributes`、`traceId` 或 request id，并且 analysis.md:113-120 也重复了同一固定列表；但 analysis.md:122 又允许可选携带 `eventType`、`sourceSystem`、`occurredAt`。这会让 Dev 和 Test Plan 无法判断这些事件顶层字段到底应该被接受、透传给 provider、参与缓存 key，还是作为非白名单字段拒绝。该矛盾直接影响公开 API 契约、provider 请求上下文和缓存隔离验收，必须先统一：要么严格遵守 Owner 决策只接受固定字段；要么修改 task/analysis 的验收口径，明确这些字段是正式允许输入、校验规则和缓存 key 组成部分。

P2:
- 无。
~~~

## Open Questions

1. `/openapi/v1/facts/query` 是否正式允许 `eventType`、`sourceSystem`、`occurredAt`？如果允许，需要同步更新 Owner 输入契约和验收标准；如果不允许，需要从 analysis.md 的 API 输入章节移除该可选说明，并说明 provider 请求上下文如何在缺少这些字段时构造。

## Score

~~~text
需求清晰度        17 / 20
需求边界清楚      18 / 20
风险可控          17 / 20
验收标准明确      16 / 20
约束一致性        14 / 20
总分              82 / 100
~~~

## Decision

~~~text
BLOCK
~~~

理由：存在 P1。上一轮关于 provider registry、inline provider、后台校验入口和 `cacheTtlSeconds` 上限的阻塞点已经基本收敛，但 Fact Query API 入参白名单仍与 Owner 决策冲突。该点会影响实现和测试用例的确定性，需要先修正后再进入 Dev。

需求：2026-05-24-fact-query
