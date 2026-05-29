# Test Plan

## Agent 指令

你是 Test Plan Agent。请在 Review PASS 后根据 task.md、analysis.md、review.md 设计测试用例。

只允许写 test-plan.md。
禁止修改业务代码、测试代码、文档代码、配置文件或任何其他文件。
禁止执行测试命令、构建命令、格式化命令或 git 命令。
你的职责只是写清楚后续 Test Agent 应执行的测试用例和命令清单。

## Test Strategy

覆盖 MQ 生产连接配置的最小上线闭环：

- `targetRef` 只表达 destination，不允许 broker endpoint、userinfo、query、fragment 或疑似凭据。
- MQ provider 运行时配置从 `EX_MARKETING_MQ_SENDERS_JSON` 加载，活动配置和 outbox payload 不保存 endpoint 或凭据。
- Kafka/RabbitMQ/Pulsar sender 仍走 adapter 边界；只有底层 transport 返回成功才代表 ack/confirm/send ack 成功并允许 outbox `SENT`。
- provider 未配置、禁用、destination allowlist 拒绝、transport 缺失、发送超时或 broker 错误时，保持 outbox payload，进入 retry/DLQ，并写脱敏错误摘要和审计。
- HTTP / WEBHOOK / MQ 通道隔离，MQ 配置错误不影响 HTTP / WEBHOOK 既有发送测试。
- 文档说明运行时配置 schema、凭据隔离、`SENT` 含义、错误分类和 DLQ 操作。

## Test Cases

1. MQ target parser accepts:
   - `kafka://reward.commands`
   - `rabbitmq://reward-exchange/reward.commands`
   - `rabbitmq:///reward.commands`
   - `pulsar://tenant-a/rewards/commands`

2. MQ target parser rejects:
   - unknown scheme
   - empty destination
   - missing RabbitMQ routingKey
   - incomplete Pulsar path
   - userinfo credentials
   - query or fragment
   - broker-like port endpoints such as `kafka://broker.example.com:9092`
   - suspicious credential-like destinations such as `kafka://token=secret`

3. Activity validation:
   - `grantChannel=MQ` accepts only legal MQ target refs.
   - `grantChannel=MQ` rejects HTTP target refs and credential-bearing refs.
   - HTTP / WEBHOOK target validation remains unchanged.

4. Runtime config:
   - `EX_MARKETING_MQ_SENDERS_JSON` loads provider endpoint, `authRef`, TLS file refs, timeout and allowlist.
   - Enabled provider without endpoints fails startup config validation.
   - Endpoint containing userinfo or `password=`, `token=`, `secret=` fails validation.
   - Unknown provider key fails validation.

5. Runtime sender:
   - Missing provider returns retryable failure and does not ack.
   - Disabled provider returns retryable failure and does not ack.
   - Destination allowlist mismatch returns retryable failure and does not ack.
   - Missing transport adapter returns retryable failure and does not ack.
   - Registered transport success is treated as provider ack and allows `SENT`.
   - Send timeout propagates as retryable failure.

6. Outbox worker:
   - MQ sender success marks outbox `SENT` and preserves command key/header idempotency.
   - MQ sender failure keeps outbox retryable, increments retry count, redacts sensitive error material, and records audit.
   - Invalid MQ target moves to DLQ without sending.
   - HTTP/Webhook success, retry, Retry-After and DLQ tests still pass.

7. Regression:
   - Existing event engine, callback, simulation, DLQ replay/close and workflow tests pass.
   - Date-sensitive MQ consumer tests must not expire based on the current date.

## Required Commands

~~~bash
go test ./...
go build ./...
~~~

## Data / Smoke

No real broker is required for this minimum PASS. Use unit tests and fake transports for ack/confirm/send ack behavior. If a real broker environment is available later, add non-blocking smoke checks for Kafka/RabbitMQ/Pulsar producer credentials and publish confirms.

## Decision

~~~text
PASS
~~~

需求：2026-05-27-mq-production-config-mq
