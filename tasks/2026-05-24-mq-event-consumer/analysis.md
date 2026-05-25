# Analysis

## 需求

`2026-05-24-mq-event-consumer`：为 ExMarketing 增加 `marketing.events` 的 MQ 事件消费入口，使外部业务系统除 HTTP `/openapi/v1/events` 外，也可以通过 MQ 投递标准营销事件。MQ 入口必须复用现有标准事件校验、幂等、活动匹配、规则执行、奖励计算、quota、participation、grant_order、outbox、审计与失败处理语义，不允许在 Consumer 内编写活动业务逻辑。

## Review 反馈处理

本轮读取 `tasks/2026-05-24-mq-event-consumer/review.md` 后，文件仍为模板：

- `P0/P1/P2`：未填写具体问题。
- `Open Questions`：未填写具体问题。
- `Decision`：未给出实际 PASS/BLOCK 结论。

因此本轮没有 Review Agent 的实质打回项需要逐条修正；本节保留用于满足返工流程要求。

## Analysis 自检修正

上一轮 `analysis.md` 为模板状态，存在以下自检问题：

- `现状/目标理解/方案/改动范围/风险点/验收方式` 均为“待填写”：本轮已补充为完整需求说明、业务规则、边界条件、异常场景、非目标和验收标准。
- `Score = 0`：上一轮未完成评分，本轮按评分规则重新评估。
- `Decision = PASS / BLOCK` 模板未定稿：本轮给出明确 Decision。
- 未回应任务背景中的三条验收方向：本轮明确要求“消费者不直接写业务逻辑”“重复消息不重复发奖”“失败消息进入可重试路径”。

当前无需要继续 BLOCK 的历史自检问题。

## 背景目标

ExMarketing 是独立活动营销中台，只通过标准事件、可选 Fact Query 和标准奖励指令与外部系统集成。当前事件接入已有 HTTP OpenAPI 入口，文档设计目标要求事件接入方式覆盖 HTTP API、Kafka/RabbitMQ/Pulsar MQ 和 Webhook Receiver。本任务聚焦 MQ Event In。

目标：

- 提供 `marketing.events` 的 MQ 消费入口。
- 让 MQ 消息进入与 HTTP 事件一致的标准事件处理链路。
- 确保 MQ 重投、重复消费、消费者重启不会导致重复 participation、grant_order、outbox 或重复发奖。
- 失败消息必须进入可重试路径，最终不可处理消息必须可观测、可排查、可人工处理或进入 DLQ 语义。
- 为 Kafka、RabbitMQ、Pulsar 明确适配边界，避免把 broker 差异泄漏到营销引擎。

## 参与方

- 外部业务系统：生产标准事件并投递到 MQ。
- MQ Broker：Kafka、RabbitMQ 或 Pulsar，承载 `marketing.events` 事件流。
- MQ Event Consumer：从 broker 读取消息，完成消息级校验、幂等键提取、ack/nack/retry/DLQ 协调。
- Event Intake / Engine：现有标准事件处理链路，负责业务校验、活动匹配、规则、奖励、quota、发奖单和 outbox。
- Repository：保存 `marketing_event_inbox`、participation、grant_order、quota、outbox、audit 等状态。
- 运维/后台人员：观察失败消息、重试状态、DLQ 和审计结果。

## 输入

MQ Topic/Queue：

```text
marketing.events
```

消息体必须是标准事件 JSON，仅接受以下业务字段：

```text
eventId
eventType
sourceSystem
tenantId
subject.type
subject.id
biz.bizType
biz.bizId
occurredAt
attributes
```

幂等键要求：

```text
tenantId:sourceSystem:eventType:bizType:bizId
```

MQ 入口必须支持从消息 header/properties 中读取 `Idempotency-Key`。如果 broker 或生产方不能提供 header，则允许按标准事件字段派生同一格式的幂等键，但派生结果必须与标准格式完全一致。

可选消息元信息：

```text
messageId
partition / offset 或 deliveryTag / messageId
traceId / correlationId
producer
attempt / redelivery count
publishedAt
```

这些元信息只能用于追踪、审计、重试和 DLQ，不得进入活动规则判断。

## 输出

成功处理后，与 HTTP 事件入口保持一致：

- 写入或复用 `marketing_event_inbox`。
- 生成活动参与记录，规则拒绝时记录 `REJECTED`。
- 命中奖励时按现有原子语义创建 participation、扣减 quota、创建 grant_order 和 outbox。
- 重复事件返回重复处理结果，不重复生成 participation、grant_order、outbox，不重复扣 quota。
- 记录必要 audit，便于区分 HTTP 与 MQ 来源。

失败处理后：

- 可重试失败不得丢消息。
- 不可重试失败必须进入可观测的失败路径，可定位 tenant、sourceSystem、eventType、biz、幂等键和失败原因。
- broker 层 ack/nack 行为必须与处理结果一致，避免成功后重复消费或失败后静默丢弃。

## 业务规则

1. MQ Consumer 只负责消息接入、格式转换、消息级确认、失败分类和调用标准事件处理链路，不允许直接写活动匹配、奖励计算、发奖、quota 扣减等业务逻辑。
2. MQ 事件与 HTTP `/openapi/v1/events` 必须共享同一标准事件模型和幂等语义。
3. 幂等键必须稳定等于 `tenantId:sourceSystem:eventType:bizType:bizId`。同一幂等键同一 payload 重复消费必须视为成功的重复消息。
4. 同一幂等键不同 payload 必须视为幂等冲突，不允许进入发奖链路。
5. 消息中出现标准事件字段以外的顶层业务字段时，应按现有标准事件严格校验策略拒绝或标记不可重试失败；业务扩展字段必须放入 `attributes`。
6. MQ 消息不能绕过 OpenAPI 事件鉴权/租户来源边界的等价约束。即使 MQ 部署在内网，也必须能限制允许的 `tenantId + sourceSystem` 组合。
7. 事件处理完成且结果已持久化后才能确认 broker 消息。
8. 对于已被幂等识别的重复消息，Consumer 应确认 broker 消息，避免无限重投。
9. 对于可重试的系统故障，应保留重试机会，不得提前 ack 后丢失。
10. 对于不可重试的数据错误，应进入 DLQ 或等价失败队列/状态，并记录明确原因。
11. Kafka、RabbitMQ、Pulsar 的差异应被限制在 MQ 适配层：订阅、确认、重投、DLQ、offset/delivery 标识和配置映射不得污染 Event Intake / Engine。
12. `marketing.events` 的消息顺序不能作为业务正确性的唯一前提；重复、乱序、至少一次投递都必须由幂等和持久化状态兜底。

## Broker 适配边界

Kafka 适配需覆盖：

- topic、consumer group、partition/offset、手动提交 offset。
- 处理成功或重复消息后提交 offset。
- 可重试失败不提交或进入受控重试 topic/机制。
- 不可重试失败进入 DLQ topic 或等价失败路径。

RabbitMQ 适配需覆盖：

- exchange/queue/routing key、manual ack/nack、redelivery、dead-letter exchange/queue。
- 处理成功或重复消息后 ack。
- 可重试失败按策略 nack/requeue 或进入延迟重试队列。
- 不可重试失败 reject/nack 到 DLQ。

Pulsar 适配需覆盖：

- topic、subscription、ack/nack、redelivery、dead letter policy。
- 处理成功或重复消息后 ack。
- 可重试失败 negative ack 或进入 broker 支持的重试机制。
- 不可重试失败进入 DLQ 或等价失败路径。

本任务要求边界清晰、行为一致；具体启用哪一种 broker 应由运行配置选择，同一进程不要求同时连接多种 broker。

## 边界条件

- MQ 消息体为空、非 JSON、超过事件大小限制：不可重试失败。
- 缺少必填标准字段：不可重试失败。
- `occurredAt` 无法解析：不可重试失败。
- 缺失 `Idempotency-Key` 但可由标准字段完整派生：允许继续处理。
- 缺失 `Idempotency-Key` 且标准字段不足以派生：不可重试失败。
- 幂等键与字段派生值不一致：不可重试失败或幂等冲突，不允许进入引擎。
- 同 key 同 payload 重投：按 duplicate 成功处理并 ack。
- 同 key 不同 payload：按 `INVALID_IDEMPOTENCY_KEY` 等价语义拒绝，进入不可重试失败路径。
- 没有匹配活动：应按现有引擎语义处理并确认消息，不应重试。
- 规则拒绝：记录拒绝参与结果并确认消息，不应重试。
- quota 不足：按现有业务语义处理，不能因 MQ 重试造成后续重复扣减。
- 数据库、仓储、引擎依赖临时不可用：可重试失败。
- Consumer 进程在处理后 ack 前崩溃：允许 broker 重投，但重投必须被幂等处理为 duplicate 或恢复处理，不得重复发奖。

## 异常场景

- Broker 连接失败：Consumer 应启动失败或持续重连，并暴露可观测状态；不得假装消费正常。
- 消息反序列化失败：直接进入不可重试失败路径。
- 标准事件校验失败：进入不可重试失败路径，并记录原因。
- 幂等冲突：进入不可重试失败路径，并记录原始幂等键、派生幂等键和冲突原因。
- 引擎处理出现临时错误：进入可重试路径。
- 可重试次数耗尽：进入 DLQ 或等价终态，保留重放依据。
- DLQ 重放：重放仍必须走标准事件校验和幂等逻辑，不得绕过业务安全规则。

## 非目标范围

- 不实现活动专用 Consumer、专用 topic 或专用发奖逻辑。
- 不修改活动规则、奖励计算或 quota 业务语义。
- 不新增外部业务数据库访问。
- 不直接修改用户余额、积分、优惠券或会员权益。
- 不实现 MQ Command Out；本任务只做 Event In。
- 不实现 Webhook Receiver。
- 不要求引入新的活动类型或插件。
- 不要求把所有 broker 同时部署上线；只要求适配边界和运行选择清晰。
- 不要求本任务完成生产级监控大盘，但必须留下可审计、可排障的信息。

## 风险

- 重复消费导致重复发奖：通过标准幂等键、inbox、participation、grant_order 幂等键和现有原子写入语义控制。
- ack 时机错误导致消息丢失：必须在处理结果持久化后确认 broker。
- broker 重试与业务重试混用导致无限循环：需区分可重试和不可重试失败，并定义重试耗尽后的 DLQ 终态。
- 多 broker 语义不一致：通过适配边界统一成功、重复、可重试失败、不可重试失败四类结果。
- MQ 内网接入绕过租户/来源约束：需要与 HTTP OpenAPI 等价的 tenant/sourceSystem allowlist 或运行配置约束。
- 消费者写入业务逻辑导致架构分层破坏：验收必须检查 Consumer 只调用标准事件处理链路。
- PostgreSQL 与内存仓储语义差异：涉及幂等、事务和重复消费的验收必须使用 PostgreSQL 验证。

## 验收标准

功能验收：

- 可从 `marketing.events` 消费标准事件 JSON。
- 消费到的事件进入与 HTTP `/openapi/v1/events` 一致的事件处理链路。
- Consumer 内不包含活动匹配、规则判断、奖励计算、quota 扣减、grant_order/outbox 创建等业务逻辑。
- 同一标准事件首次消费可生成正确 participation、grant_order 和 outbox。
- 同一消息或同一幂等键重复投递，不重复生成 participation、grant_order、outbox，不重复扣 quota。
- 同一幂等键不同 payload 被拒绝，不进入发奖链路。
- 临时系统错误进入可重试路径，不静默 ack 丢失。
- 非法消息进入不可重试失败路径或 DLQ，失败原因可查询。
- Kafka、RabbitMQ、Pulsar 的适配边界在文档和配置层清晰，至少覆盖订阅、ack/nack、retry、DLQ、消息元信息映射。

安全与架构验收：

- MQ 入口不连接外部业务库，不引用外部业务代码包。
- MQ 入口不能直接发奖，只能生成标准发奖单和 outbox 指令。
- MQ 入口对 `tenantId + sourceSystem` 有等价的来源约束。
- 消息日志不得记录 secret、token、完整 Authorization 等敏感信息。

测试验收：

- `go test ./...` 通过。
- `go build ./...` 通过。
- PostgreSQL 模式下验证：首次 MQ 消费、重复 MQ 消费、幂等冲突、非法 JSON、缺失必填字段、引擎临时失败重试路径、不可重试失败进入 DLQ 或等价失败状态。
- 至少覆盖一次 Consumer 崩溃/ack 前重投的幂等场景，可用测试或明确的本地复现步骤证明。

## 待确认问题

以下问题不阻塞本轮需求进入研发，但需要 Dev/Test 在实现和验证计划中显式选择：

- 首个可运行 broker 是否以 Kafka 为主，RabbitMQ/Pulsar 仅完成边界和可扩展适配，还是三者均需可运行。
- 生产环境 MQ 的具体连接参数、认证方式、topic/queue/DLQ 命名规范。
- MQ 入口的 tenant/sourceSystem allowlist 是否复用现有 OpenAPI token 配置，还是新增独立配置。
- DLQ 查询和重放是否复用现有 outbox DLQ 管理模型，还是需要后续任务单独建设 Event DLQ 管理后台。

## 评分

```text
需求清晰度        18/20
范围可控          18/20
风险可控          17/20
验收标准明确      18/20
架构一致性        19/20
总分              90/100
```

扣分原因：

- 具体生产 broker 和连接配置未在 Owner 决策中指定。
- Event DLQ 的后台操作形态尚未确定，只能要求具备等价失败路径和可观测信息。

## Decision

```text
PASS
```

理由：任务背景、设计文档和现有系统边界已经足以确定 MQ Event Consumer 的业务目标、输入输出、幂等要求、失败语义、非目标和验收标准。剩余问题属于实现配置和后续运维形态选择，不影响研发按标准事件入口和适配边界推进。

需求：2026-05-24-mq-event-consumer
