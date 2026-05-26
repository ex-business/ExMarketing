# Test

## Agent 指令

你是 Test Agent。请根据 task.md、analysis.md、review.md、test-plan.md 和 implementation.md 执行完整测试。

最低要求：

~~~bash
go test ./...
go build ./...
~~~

## Test Cases

已按 `test-plan.md` 和 `implementation.md` 执行本轮可自动化验证，重点覆盖：

1. `grantMode=AUTO` + `grantChannel=MQ` 接受合法 targetRef：
   - `kafka://reward.commands`
   - `rabbitmq://reward-exchange/reward.commands`
   - `rabbitmq:///reward.commands`
   - `pulsar://tenant-a/rewards/commands`
2. MQ targetRef 拒绝非法输入：
   - HTTP URL
   - 含凭据的 Kafka URI
   - 缺少 topic / routing key / pulsar topic
3. Outbox worker 对 HTTP / WEBHOOK 原有成功、redirect、timeout、network error、429 Retry-After、5xx retry、4xx DLQ、max retry DLQ 行为无回归。
4. MQ outbox sender 替身返回成功时：
   - worker 调用 MQ sender
   - outbox 标记为 `SENT`
   - MQ message provider / destination 正确
   - key / headers 带 `commandId`、`tenantId`、`idempotencyKey`
5. MQ sender 返回错误时：
   - outbox 保持为可重试 `FAILED`
   - retry_count 增加
   - error_message 脱敏，不保留 `token=secret`
6. MQ targetRef 非法时：
   - outbox 进入 `DLQ`
   - error_message 可诊断并包含 targetRef 语义
7. 全仓库 Go 测试和构建通过。

## Commands

~~~bash
go test ./internal/mqtarget ./internal/marketing ./internal/worker
go test ./...
go build ./...
~~~

## Result

~~~text
PASS
~~~

## Evidence

命令 1：

~~~bash
go test ./internal/mqtarget ./internal/marketing ./internal/worker
~~~

结果：

~~~text
?   	github.com/ex-stack/ExMarketing/internal/mqtarget	[no test files]
ok  	github.com/ex-stack/ExMarketing/internal/marketing	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/worker	(cached)
~~~

命令 2：

~~~bash
go test ./...
~~~

结果摘要：

~~~text
?   	github.com/ex-stack/ExMarketing/cmd/exmarketing	[no test files]
?   	github.com/ex-stack/ExMarketing/cmd/workflowctl	[no test files]
ok  	github.com/ex-stack/ExMarketing/internal/decimal	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/engine	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/fact	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/httpapi	0.577s
ok  	github.com/ex-stack/ExMarketing/internal/marketing	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/mqconsumer	(cached)
?   	github.com/ex-stack/ExMarketing/internal/mqtarget	[no test files]
?   	github.com/ex-stack/ExMarketing/internal/plugin	[no test files]
ok  	github.com/ex-stack/ExMarketing/internal/postgres	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/reward	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/rule	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/worker	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/workflow	0.530s
~~~

命令 3：

~~~bash
go build ./...
~~~

结果：

~~~text
PASS，命令退出码 0，无输出。
~~~

## Notes

- 本轮未接入真实 Kafka / RabbitMQ / Pulsar broker；按 `test-plan.md`，真实 broker 联调不作为本轮最低要求，当前通过可控 sender 替身验证 ack / confirm / send ack 收敛后的 sender contract 与 outbox 状态流转。
- 除本文件外未修改业务代码、测试代码、文档代码、配置文件或其他文件。

需求：2026-05-24-outbox-mq-sender
