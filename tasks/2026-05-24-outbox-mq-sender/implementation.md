# Implementation

## Agent 指令

你是 Dev Agent。只有 review.md 为 PASS 且 test-plan.md 为 PASS 时才能开发。

必须按 task.md、analysis.md、review.md、test-plan.md 执行，不得扩大范围。

完成开发后必须先做基础验证：

~~~bash
go test ./...
go build ./...
~~~

## Summary

已完成 Outbox MQ Sender 的最小适配边界：

- 补齐 `test-plan.md`，明确 PASS，并覆盖 Review P2：RabbitMQ 默认 exchange 示例、三类 MQ ack/confirm/send ack 的 sender contract、MQ key/headers 验收。
- 新增 MQ `targetRef` 解析与校验，支持：
  - `kafka://<topic>`
  - `rabbitmq://<exchange>/<routingKey>`
  - `rabbitmq:///<routingKey>`，表示默认 exchange
  - `pulsar://<tenant>/<namespace>/<topic>`
- 活动配置允许 `grantMode=AUTO` + `grantChannel=MQ`；非法 MQ targetRef、HTTP URL、凭据、query、fragment、缺少目的地会在创建或上线前校验失败。
- Outbox worker claim 范围扩展到 `MQ`，MQ 发送走可注入 `MQSender` contract。
- MQ sender 成功返回后才将 outbox 标记为 `SENT`；发送失败、sender 未配置或 broker 不可达时保留原 outbox payload，并复用既有 retry / backoff / DLQ 语义。
- MQ message contract 使用 `commandId` 优先作为 key，headers 带 `tenantId`、`messageType`、`targetChannel`、`commandId`、`idempotencyKey`，用于外部消费幂等。
- 文档同步说明 MQ targetRef 格式、敏感配置边界、`SENT` 与 callback 成功的区别、失败排查和 DLQ 操作。

## Changed Files

- `internal/mqtarget/target.go`
- `internal/marketing/model.go`
- `internal/marketing/memory_repository.go`
- `internal/postgres/repository.go`
- `internal/worker/outbox_worker.go`
- `internal/marketing/prelaunch_validation_test.go`
- `internal/worker/outbox_worker_test.go`
- `README.md`
- `docs/api.md`
- `docs/architecture.md`
- `docs/data-model.md`
- `docs/operations.md`
- `tasks/2026-05-24-outbox-mq-sender/test-plan.md`
- `tasks/2026-05-24-outbox-mq-sender/implementation.md`

## Behavior Changes

- `MQ` 不再在活动创建阶段被一概拒绝，改为按 MQ URI destination 校验。
- `ListPendingOutbox` 会 claim `HTTP`、`WEBHOOK`、`MQ` 三类待发送 outbox。
- `MQ` outbox 不再跳过；worker 会构造 MQ sender message 并调用 adapter。
- 当前代码只定义 sender adapter contract，不绑定具体 Kafka / RabbitMQ / Pulsar 客户端；真实 broker 连接和凭据仍应由运行环境 sender 实现提供。
- MQ `SENT` 只表示 broker adapter 确认接收，不会把 grant_order 自动置为 `SUCCESS`；真实发奖结果仍以 reward callback 为准。

## Verification

~~~bash
go test ./internal/mqtarget ./internal/marketing ./internal/worker
go test ./...
go build ./...
~~~

结果：

~~~text
PASS
~~~

## Notes

- `test.md` 和 `commit.md` 当前仍为模板态，未发现 Result=FAIL 或 BLOCK / FAILED 的具体失败原因可逐条修复。
- 当前工作树存在大量其它任务留下的未提交修改；本轮未回滚、整理或提交这些无关改动。

需求：2026-05-24-outbox-mq-sender
