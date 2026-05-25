# Test Plan

## Agent 指令

你是 Test Plan Agent。请在 Review PASS 后、Dev 开发前，根据 task.md、analysis.md、review.md 设计测试用例。

只写 test-plan.md，不改业务代码，不执行提交。

## Test Strategy

本轮固定采用 broker-neutral fake adapter 作为可执行消费验证范围，不直接连接 Kafka/RabbitMQ/Pulsar。代码层提供统一 Message/Action/FailureRecord 边界，测试覆盖 Kafka/RabbitMQ/Pulsar 需要映射的核心语义：成功 ACK、重复 ACK、不可重试 DLQ、可重试 NACK_RETRY。

实现优先级按 review.md P2 明确：

- 有 `Idempotency-Key` header 时，必须校验其与 payload 派生键完全一致。
- 无 `Idempotency-Key` header 时，允许从标准事件字段派生，并在处理结果中标记 `DerivedIdempotencyKey=true`。
- 非法 JSON、缺必填字段、未知业务字段、幂等冲突、来源不允许进入不可重试失败记录。
- 引擎/仓储系统错误进入可重试失败记录。
- Consumer 只调用标准事件处理链路，不写活动匹配、规则、奖励、quota 或 outbox 业务逻辑。

## Test Cases

1. 首次 MQ 标准事件消费成功：
   - fake broker 投递 `marketing.events` 标准 JSON。
   - header 包含正确 `Idempotency-Key`。
   - 期望 `Action=ACK`，生成 participation、grant_order、outbox。

2. 重复 MQ 消息幂等：
   - 同一幂等键、同一标准 payload 再次投递。
   - 期望 `Action=ACK` 且 `Duplicate=true`。
   - 不新增 participation、grant_order、outbox。

3. 无 header 派生幂等键：
   - MQ 消息不带 `Idempotency-Key`。
   - 期望从 `tenantId:sourceSystem:eventType:bizType:bizId` 派生。
   - 期望 `Action=ACK` 且 `DerivedIdempotencyKey=true`。
   - 审计 actor 为 `mq`，用于区分 HTTP 入口。

4. 非法消息不可重试失败：
   - 覆盖非法 JSON、缺必填字段、未知顶层业务字段、header 与 payload 幂等键不一致、tenant/sourceSystem 不在 allowlist。
   - 期望 `Action=DLQ`，保存 `FailureRecord`，`Retryable=false`，reasonCode 可定位。
   - 失败记录不得泄露 Authorization/token/secret header。

5. 引擎临时失败可重试：
   - 用 fake repository 模拟数据库不可用。
   - 期望 `Action=NACK_RETRY`，保存 `FailureRecord`，`Retryable=true`。

6. 架构边界检查：
   - `internal/mqconsumer` 不实现活动匹配、规则执行、奖励计算、quota 扣减、grant_order/outbox 创建。
   - 这些逻辑继续由 `engine.ProcessEventFrom` 和 repository 原子逻辑承担。

## Required Commands

~~~bash
go test ./...
go build ./...
~~~

## Data / Smoke

本轮 smoke 数据使用内存 repository 创建 ACTIVE `FIRST_DEPOSIT_BONUS` 活动：

- trigger event: `DEPOSIT_SUCCESS`
- tenantId: `site_001`
- sourceSystem: `payment`
- biz: `DEPOSIT/pay_*`
- reward: 20% ratio bonus through existing HTTP grant channel/outbox flow

可执行测试文件：

- `internal/mqconsumer/consumer_test.go`

## Decision

~~~text
PASS
~~~

需求：2026-05-24-mq-event-consumer
