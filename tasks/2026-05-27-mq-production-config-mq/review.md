# Review

## Agent 指令

你是 Review Agent。请审核 analysis.md。

只写 tasks/2026-05-27-mq-production-config-mq/review.md，不改代码。

存在 P0/P1 或 Score < 80 必须 BLOCK。

## Findings

```text
P0:
无。

P1:
无。

P2:
1. 运行时 MQ 配置的具体环境变量名、secret manager 形态、证书挂载路径和多集群路由策略仍未固化。analysis.md 已明确默认最小验收口径为每类 provider 至少一个运行时连接配置入口，且凭据不得进入 activity config、outbox、audit、log；因此该缺口不阻塞 Dev/Test，但后续实现和运维文档必须把实际配置 schema 写清楚。
2. 真实 Kafka/RabbitMQ/Pulsar broker 联调未作为本轮硬性验收。analysis.md 已要求用可控替身覆盖 ack/confirm/send ack、错误分类、retry/DLQ 和脱敏，真实 broker 冒烟作为生产化增强；在正式上线前仍建议补充真实 broker 冒烟。
```

## Review Notes

- analysis.md 已从模板态补齐需求目标、参与方、输入输出、配置隔离、安全约束、provider 确认语义、错误分类、审计脱敏、边界异常、非目标范围、风险点和验收方式。
- 核心任务点覆盖充分：`targetRef` 只描述 destination；broker endpoint、账号、密码、token、证书私钥和完整连接串只能来自运行时安全配置；activity config、outbox payload、audit/log/DLQ 查询不得泄露凭据。
- Kafka ack、RabbitMQ publisher confirm、Pulsar send ack 才能标记 `SENT` 的语义已明确，且 `SENT` 不等于真实发奖成功，grant_order 仍等待 reward callback。
- 失败路径已细分为可重试、不可重试、配置/认证失败、目标不存在或被拒绝，并要求保留 outbox payload、稳定 commandId/idempotencyKey、进入 retry/DLQ/replay/close 既有路径。
- 边界和验收覆盖合法/非法 MQ `targetRef`、provider 未配置、确认未知、认证失败、授权不足、payload 序列化失败、HTTP/WEBHOOK/MQ 通道隔离、模拟运行无副作用和基础验证命令，足以支撑后续 Dev/Test。

## Open Questions

无阻塞问题。

非阻塞待实现/运维阶段落地：

- 运行时配置 schema 的最终载体和字段名。
- 生产环境真实 broker 冒烟环境与凭据轮换流程。
- 是否在后续版本引入租户级 destination allowlist 或同 provider 多集群路由。

## Score

```text
需求清晰度        19/20
业务规则完整      19/20
边界异常清楚      19/20
验收标准明确      18/20
约束一致性        19/20
Score             94/100
```

扣分原因：

- 运行时 secret/config 的具体部署载体未在 analysis 阶段完全定型。
- 真实 broker 联调未作为本轮强制 PASS 条件。

## Decision

```text
PASS
```

规则：

```text
存在 P0/P1 -> BLOCK
Score < 80 -> BLOCK
无 P0/P1 且 Score >= 80 -> PASS
```

需求：2026-05-27-mq-production-config-mq
