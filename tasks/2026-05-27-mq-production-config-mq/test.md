# Test

## Scope

根据 `task.md`、`analysis.md`、`review.md`、`test-plan.md` 和 `implementation.md` 执行 Test Agent 验证。

本轮只验证 Dev Agent 已实现内容，不修改业务代码、测试代码、文档代码、配置文件或其他文件。

## Test Cases

已覆盖并通过：

1. MQ `targetRef` parser 接受合法 Kafka / RabbitMQ / RabbitMQ default exchange / Pulsar destination。
2. MQ `targetRef` parser 拒绝 userinfo、query、fragment、broker-like port endpoint、疑似凭据 destination、缺失 routingKey 和不完整 Pulsar path。
3. `EX_MARKETING_MQ_SENDERS_JSON` runtime config 可加载 provider endpoint、`authRef`、TLS 文件引用、timeout 和 allowlist。
4. runtime config 拒绝带凭据的 endpoint。
5. `RuntimeMQSender` 对 provider 未配置、禁用、allowlist 拒绝、transport 缺失均不 ack，并返回 retryable failure。
6. 已注册 transport 成功返回时视为 provider ack，可让 MQ outbox 进入 `SENT`。
7. send timeout 会按 retryable failure 传播，不把 outbox 标记为 `SENT`。
8. outbox worker MQ 发送成功、失败、非法 targetRef DLQ 流转均通过。
9. HTTP / Webhook outbox 成功、retry、Retry-After、DLQ 回归通过。
10. MQ consumer 日期相关测试通过，未因当前日期过期失败。
11. DLQ query / replay / close、workflow 路由、callback 和 simulation 相关回归通过。
12. 全仓库 `go test ./...` 和 `go build ./...` 均通过。

## Commands

~~~bash
go test ./...
go build ./...
go test ./internal/mqtarget ./internal/worker -run 'Test' -v
go test ./internal/mqconsumer ./internal/httpapi ./internal/workflow -run 'Test' -v
~~~

## Result

~~~text
PASS
~~~

## Evidence

`go test ./...` 结果：

~~~text
?   	github.com/ex-stack/ExMarketing/cmd/engine-e2e-postgres	[no test files]
?   	github.com/ex-stack/ExMarketing/cmd/exmarketing	[no test files]
?   	github.com/ex-stack/ExMarketing/cmd/workflowctl	[no test files]
ok  	github.com/ex-stack/ExMarketing/internal/decimal	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/engine	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/fact	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/httpapi	0.463s
ok  	github.com/ex-stack/ExMarketing/internal/marketing	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/mqconsumer	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/mqtarget	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/plugin	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/postgres	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/reward	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/rule	(cached)
?   	github.com/ex-stack/ExMarketing/internal/ruleexpr	[no test files]
ok  	github.com/ex-stack/ExMarketing/internal/worker	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/workflow	0.544s
~~~

`go build ./...` 结果：命令退出码 `0`，无输出。

MQ 专项测试摘要：

~~~text
ok  	github.com/ex-stack/ExMarketing/internal/mqtarget	0.170s
ok  	github.com/ex-stack/ExMarketing/internal/worker	0.422s
~~~

回归专项测试摘要：

~~~text
ok  	github.com/ex-stack/ExMarketing/internal/mqconsumer	0.277s
ok  	github.com/ex-stack/ExMarketing/internal/httpapi	0.662s
ok  	github.com/ex-stack/ExMarketing/internal/workflow	0.573s
~~~

未执行真实 Kafka/RabbitMQ/Pulsar broker 冒烟；`analysis.md` 和 `test-plan.md` 明确真实 broker 不作为本轮最小 PASS 前置，当前使用单元测试和 fake transport 覆盖 ack/confirm/send ack、错误分类、retry/DLQ 和脱敏边界。

需求：2026-05-27-mq-production-config-mq
