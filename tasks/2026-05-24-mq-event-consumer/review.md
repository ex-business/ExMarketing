# Review

## Agent 指令

你是 Review Agent。请审核 analysis.md。

不改代码，不提交。

## Findings

~~~text
P0:
无。

P1:
无。

P2:
1. analysis.md:78、154 允许 MQ 消息缺失 Idempotency-Key 时从标准事件字段派生幂等键。该口径与 HTTP Event API 的“Header 必填”不同，但 MQ 场景可接受。Dev/Test 需要明确实现优先级：有 header 时必须校验 header 与 payload 派生值一致；无 header 时才允许派生，并在审计/日志中标记为 derived，避免生产方长期漏传 header 不可见。
2. analysis.md:147、229 将首个可运行 broker 留给实现/测试阶段选择。此项不阻塞研发，但 test-plan.md 必须固定本轮实际跑通的 broker 或 fake adapter 范围，不能只写 Kafka/RabbitMQ/Pulsar 边界文档而没有任何可执行消费验证。
3. analysis.md:120、172、208、222 对 DLQ 或“等价失败状态”给出了目标，但未指定数据模型/API 形态。此项不阻塞 analysis PASS；Dev/Test 必须至少提供可查询的失败记录或 broker DLQ 验证方式，覆盖非法 JSON、缺必填字段、幂等冲突和重试耗尽。
~~~

## Review Notes

analysis.md 已覆盖原任务三条硬性验收：

- Consumer 不写活动业务逻辑，只调用标准 Event Intake / Engine 链路：analysis.md:111、202-204。
- 重复消息不重复 participation、grant_order、outbox 或 quota 扣减：analysis.md:113-114、157-163、204-206。
- 失败消息进入可重试路径，不可重试失败可观测或进入 DLQ：analysis.md:103-107、119-120、167-173、207-208。

与现有 HTTP Event API 约束也基本一致，包括标准事件字段、未知字段拒绝、幂等冲突、tenant/sourceSystem 来源约束和敏感信息日志限制。

## Open Questions

- 本轮 Dev/Test 选择 Kafka、RabbitMQ、Pulsar 中哪一个作为实际可运行验证目标，还是先实现 broker-neutral fake adapter 加单元测试。
- MQ tenant/sourceSystem allowlist 是复用 `EX_MARKETING_OPENAPI_TOKENS` 的配置解析，还是新增 MQ 专用配置。
- Event DLQ 的查询/重放是否复用 outbox DLQ 管理接口，还是只在本任务提供 broker DLQ/失败状态记录。

## Score

~~~text
需求清晰度        18/20
范围可控          17/20
风险可控          17/20
验收标准明确      18/20
架构一致性        19/20
总分              89/100
~~~

## Decision

~~~text
PASS
~~~

规则：

~~~text
存在 P0/P1 -> BLOCK
Score < 80 -> BLOCK
无 P0/P1 且 Score >= 80 -> PASS
~~~

需求：2026-05-24-mq-event-consumer
