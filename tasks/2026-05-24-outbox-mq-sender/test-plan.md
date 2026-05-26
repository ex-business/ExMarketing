# Test Plan

## Agent 指令

你是 Test Plan Agent。请在 Review PASS 后、Dev 开发前，根据 task.md、analysis.md、review.md 设计测试用例。

只写 test-plan.md，不改业务代码，不执行提交。

## Test Strategy

围绕 MQ Command Out 的最小可验收边界做分层验证：

- 配置校验层：`grantChannel=MQ` 允许合法 Kafka / RabbitMQ / Pulsar `targetRef`，拒绝 HTTP URL、凭据、缺少目的地、query / fragment 和无法解析的 URI。
- Outbox worker 层：MQ outbox 由既有 `ListPendingOutbox` claim 后进入 sender contract；只有 sender 返回成功才 `SENT`，失败保留原 payload 并进入 retry / DLQ 语义。
- Sender contract 层：用测试替身验证 Kafka ack、RabbitMQ publisher confirm、Pulsar send ack 等“broker 已接受”语义都收敛为 `Send(ctx, MQMessage) nil`；错误返回必须按 retry / non-retry 分类处理。
- 幂等属性层：MQ message key 使用 `commandId` 优先，其次 `idempotencyKey`；headers 至少包含 `tenantId`、`messageType`、`targetChannel`、`commandId`、`idempotencyKey`。
- 回归层：HTTP / WEBHOOK outbox 原发送、retry、DLQ、replay、close 不回归；模拟运行仍只预览，不写 outbox、不实际发送 MQ。
- 文档层：README、API、data model、architecture、operations 同步说明 MQ targetRef、SENT 与 callback 成功的区别、失败排查与 DLQ 操作。

## Test Cases

1. 创建活动时 `grantMode=AUTO`、`grantChannel=MQ`、`targetRef=kafka://reward.commands` 校验通过。
2. 创建活动时 `targetRef=rabbitmq://reward-exchange/reward.commands` 校验通过。
3. 创建活动时 `targetRef=rabbitmq:///reward.commands` 校验通过，表示 RabbitMQ 默认 exchange + routing key。
4. 创建活动时 `targetRef=pulsar://tenant-a/rewards/commands` 校验通过。
5. MQ `targetRef=https://wallet.example.com/reward`、`kafka://user:pass@reward.commands`、`kafka://`、`rabbitmq://reward-exchange`、`pulsar://tenant-a/rewards` 校验失败。
6. MQ outbox payload 非 JSON 时进入 `DLQ`，不发送空消息。
7. MQ outbox targetRef 非法时进入 `DLQ`，错误信息可诊断且不泄露凭据。
8. MQ sender 替身返回成功时，worker 标记 outbox 为 `SENT`；grant_order 不自动变为 `SUCCESS`。
9. MQ sender 替身返回 retryable 错误时，worker 标记 outbox 为 `FAILED`、增加 retry_count、设置 next_retry_at，payload 保持原值。
10. MQ sender 替身持续失败达到最大重试次数后，outbox 进入 `DLQ`，可被既有 DLQ 查询、replay、close 流程操作。
11. MQ message key / headers 包含 `commandId`、`tenantId`、`idempotencyKey`，重复 retry 使用同一 payload 和同一幂等标识。
12. HTTP / WEBHOOK worker 测试继续覆盖 2xx、4xx、5xx、429 Retry-After、超时、网络错误和 redirect 不跟随。

## Required Commands

~~~bash
go test ./...
go build ./...
~~~

## Data / Smoke

本轮不要求真实 Kafka / RabbitMQ / Pulsar broker 联调。使用可控 sender 替身分别模拟 broker ack / publisher confirm / send ack 成功、retryable 失败和 non-retryable 失败；真实 broker 连接配置作为生产化联调增强项，不进入活动配置或 outbox payload。

## Decision

~~~text
PASS
~~~

需求：2026-05-24-outbox-mq-sender
