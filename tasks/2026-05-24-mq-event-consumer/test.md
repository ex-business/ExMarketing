# Test

## Agent 指令

你是 Test Agent。请根据 task.md、analysis.md、review.md、test-plan.md 和 implementation.md 执行完整测试。

最低要求：

~~~bash
go test ./...
go build ./...
~~~

## Test Scope

按 `test-plan.md` 固定范围，本轮执行 broker-neutral fake adapter 验证，不连接真实 Kafka/RabbitMQ/Pulsar。

重点覆盖：

- 首次 MQ 标准事件消费成功并返回 `ACK`。
- 同一幂等键、同一 payload 重复消费返回 `ACK` + `Duplicate=true`，不新增 participation、grant_order、outbox。
- 缺少 `Idempotency-Key` header 时可从标准事件字段派生，结果标记 `DerivedIdempotencyKey=true`。
- MQ 入口审计 actor 为 `mq`，与 HTTP `openapi` 入口区分。
- 非法 JSON、缺必填字段、未知顶层字段、header 与 payload 幂等键不一致、tenant/sourceSystem 不在 allowlist 时进入 `DLQ`，保存不可重试 `FailureRecord`。
- 引擎/仓储系统错误进入 `NACK_RETRY`，保存可重试 `FailureRecord`。
- 失败记录过滤 `Authorization` / token / secret 类敏感 header。
- 架构边界检查：`internal/mqconsumer` 只调用 `engine.ProcessEventFrom(ctx, event, "mq")`，活动匹配、规则、奖励、quota、grant_order、outbox 仍在 engine/repository 链路内处理。

## Commands

~~~bash
go test -count=1 ./internal/mqconsumer -v
go test ./...
go build ./...
go test -count=1 ./...
git diff --check
rg -n "ProcessEventFrom|FindMatchingActivities|Evaluate|Reward|Quota|GrantOrder|Outbox|CreateGrant|CreateOutbox|CreateParticipation|quota|grant|outbox|reward|rule|activity" internal/mqconsumer internal/engine/service.go
~~~

## Result

~~~text
PASS
~~~

## Evidence

`go test -count=1 ./internal/mqconsumer -v`：

~~~text
=== RUN   TestConsumerProcessesEventAndAcksDuplicateWithoutNewGrants
--- PASS: TestConsumerProcessesEventAndAcksDuplicateWithoutNewGrants (0.00s)
=== RUN   TestConsumerAllowsDerivedIdempotencyKeyAndRecordsSourceAudit
--- PASS: TestConsumerAllowsDerivedIdempotencyKeyAndRecordsSourceAudit (0.00s)
=== RUN   TestConsumerRejectsInvalidMessagesToDLQ
=== RUN   TestConsumerRejectsInvalidMessagesToDLQ/bad_json
=== RUN   TestConsumerRejectsInvalidMessagesToDLQ/missing_field
=== RUN   TestConsumerRejectsInvalidMessagesToDLQ/unknown_field
=== RUN   TestConsumerRejectsInvalidMessagesToDLQ/mismatched_key
=== RUN   TestConsumerRejectsInvalidMessagesToDLQ/forbidden_source
--- PASS: TestConsumerRejectsInvalidMessagesToDLQ (0.00s)
=== RUN   TestConsumerClassifiesEngineSystemErrorAsRetryable
--- PASS: TestConsumerClassifiesEngineSystemErrorAsRetryable (0.00s)
PASS
ok  	github.com/ex-stack/ExMarketing/internal/mqconsumer	0.242s
~~~

`go test ./...`：

~~~text
ok  	github.com/ex-stack/ExMarketing/internal/engine	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/httpapi	0.326s
ok  	github.com/ex-stack/ExMarketing/internal/marketing	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/mqconsumer	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/postgres	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/reward	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/rule	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/worker	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/workflow	0.502s
~~~

`go build ./...`：退出码 0，无输出。

`go test -count=1 ./...`：

~~~text
ok  	github.com/ex-stack/ExMarketing/internal/engine	0.167s
ok  	github.com/ex-stack/ExMarketing/internal/httpapi	0.412s
ok  	github.com/ex-stack/ExMarketing/internal/marketing	0.486s
ok  	github.com/ex-stack/ExMarketing/internal/mqconsumer	0.626s
ok  	github.com/ex-stack/ExMarketing/internal/postgres	0.744s
ok  	github.com/ex-stack/ExMarketing/internal/reward	0.926s
ok  	github.com/ex-stack/ExMarketing/internal/rule	1.116s
ok  	github.com/ex-stack/ExMarketing/internal/worker	1.309s
ok  	github.com/ex-stack/ExMarketing/internal/workflow	1.655s
~~~

`git diff --check`：退出码 0，无输出。

架构边界检查结果：

- `internal/mqconsumer/consumer.go` 只有一次业务链路调用：`c.engine.ProcessEventFrom(ctx, event, "mq")`。
- `internal/mqconsumer` 未发现直接调用 `CreateParticipation`、`CreateGrant`、`CreateOutbox`、活动匹配或规则/奖励计算逻辑。
- `FindMatchingActivities`、规则执行、奖励计算、quota、grant_order、outbox 创建仍位于 `internal/engine/service.go`。

## Notes

- 本轮按 `test-plan.md` 选择 fake adapter 作为可执行消费验证范围；未连接真实 Kafka/RabbitMQ/Pulsar broker。
- 未发现测试失败项，因此无需记录失败复现或打回 Dev Agent。

需求：2026-05-24-mq-event-consumer
