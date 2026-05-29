# Task

## 标题

activityplugin-context-state: 插件上下文与状态隔离

## 需求 ID

2026-05-29-activityplugin-context-state

## 背景

玩法插件通常需要维护场次、用户抽奖次数、红包池、翻卡进度等状态。如果状态边界不清晰，插件很容易绕过主链路，造成重复发奖、跨租户污染或与 quota 状态不一致。

## 目标

- 明确插件可读上下文和可写状态边界。
- 插件状态必须按 tenantId、activityId、activityVersion、pluginCode、subject、biz 维度隔离。
- 插件状态写入必须具备幂等键，重复事件不重复消费状态。
- 插件状态不能替代主引擎 participation、grant_order、quota、outbox。

## 范围

- PluginContext：标准事件、活动配置快照、规则结果、已解析 facts、当前时间、traceId。
- PluginStateStore：Get、PutIfAbsent、CompareAndSwap、AppendEvent、ListByRound 等最小接口。
- 状态幂等：eventId/idempotencyKey + pluginCode + stateKey 唯一。
- 状态隔离：tenant 必填，禁止跨 tenant 查询或修改。
- 状态审计：关键状态变更保留 before/after 摘要。

## 交付物

- 插件上下文结构和状态存储接口。
- Memory / PostgreSQL 或等价持久化实现边界。
- 幂等和并发状态写测试。
- 文档说明哪些状态属于插件，哪些必须走主引擎。

## 非目标

- 不做具体红包池或抽奖池算法。
- 不做排行榜大状态存储。
- 不把插件状态作为最终发奖事实来源。

## 风险等级

高

## 验收标准

- 重复事件不会重复写入插件状态。
- 并发同一用户/同一场次状态更新不会覆盖或丢失。
- 插件不能修改 quota/grant_order/outbox。
- 跨 tenant/stateKey 访问被拒绝或无结果。
- `go test ./...` 通过。
