# Review

## 审核对象

`tasks/2026-05-24-outbox-mq-sender/analysis.md`

## Findings

```text
P0: 无
P1: 无
P2:
  1. analysis.md:62-66 对三类 MQ 的 targetRef 给出了默认 URI 口径，但 RabbitMQ 默认 exchange 的“空 exchange”表达仍偏抽象。实现前建议补一个明确示例，例如 rabbitmq:///routing-key 或 rabbitmq://default/routing-key，并说明解析规则，避免 Dev/Test Agent 各自理解。
  2. analysis.md:73、152 使用“broker 确认接受消息”作为 SENT 条件，方向正确；但三类 MQ 的确认语义不同，后续测试计划应把 Kafka ack、RabbitMQ publisher confirm、Pulsar send ack 分别落到可替身验证的 sender contract，避免只测到函数返回成功。
  3. analysis.md:87-99 已定义 Reward Command 必备字段，但未要求 MQ message key / headers 使用 commandId、tenantId 或 idempotencyKey。该点不阻塞需求分析，但实现前建议在 test-plan 中补充消息属性验收，降低外部消费者幂等和分区顺序的不确定性。
```

## Open Questions

- MQ 连接配置的环境变量或配置项命名仍未在 analysis 中固定；analysis 已明确凭据不得进入活动配置或 outbox payload，因此该问题可留给实现阶段按现有配置风格落地。
- 是否接真实 Kafka / RabbitMQ / Pulsar 做集成测试仍未强制；analysis 已给出可控替身验证与真实 broker 联调分层，当前不阻塞。

## Score

```text
需求清晰度        17/20
范围可控          18/20
风险可控          17/20
验收标准明确      18/20
架构一致性        19/20
总分              89/100
```

扣分原因：

- targetRef 和不同 MQ 的确认语义还有少量实现解释空间。
- 真实 broker 集成验收未固化为本轮强制项，但 analysis 已把状态流转、幂等和 DLQ 作为必须验收口径。

## Decision

```text
PASS
```

无 P0/P1，Score >= 80。可以进入 Test Plan / Dev 阶段；P2 项建议在测试计划和实现说明中收敛。
