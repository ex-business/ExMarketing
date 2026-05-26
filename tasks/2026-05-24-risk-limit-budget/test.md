# Test

## Agent 指令

你是 Test Agent。请根据 task.md、analysis.md、review.md、test-plan.md 和 implementation.md 执行完整测试。

最低要求：

~~~bash
go test ./...
go build ./...
~~~

## Test Cases

- RISK/LIMIT 阶段规则编排：已通过 `internal/rule`、`internal/engine` 相关测试覆盖 RISK/LIMIT grouped stage、风险标签拒绝、规则解释。
- USER_LIMIT / DAILY_LIMIT：已通过 HTTP quota conflict、engine DAILY_LIMIT activity timezone、quota preview 和 repository 原子扣减相关测试覆盖。
- BUDGET / STOCK：已通过 HTTP conflict + audit、memory/postgres atomic grant rollback 相关测试覆盖，确认失败时不生成部分 grant/outbox。
- 非金额 reward / 零奖励：已通过 reward calculator 与 engine non-amount reward 测试覆盖。
- 同一事件互斥：已通过 simulation exclusive selection 和 loser rejected participation 测试覆盖，落选原因包含 `EXCLUSIVE_GROUP_LOST`。
- 幂等重复 / 幂等冲突：已通过 memory/postgres inbox duplicate/conflict、HTTP duplicate/conflict 和 MQ duplicate ack 测试覆盖。
- 模拟运行无副作用：已通过 HTTP simulate event quota preview / no persistence 相关测试覆盖。

## Commands

~~~bash
go test ./...
go build ./...
go test ./internal/engine ./internal/httpapi ./internal/marketing ./internal/postgres -run 'Risk|Limit|Budget|Stock|Daily|Quota|Exclusive|Duplicate|Idempot|Simulation|AtomicGrant'
~~~

## Result

~~~text
PASS
~~~

## Evidence

~~~text
$ go test ./...
?   	github.com/ex-stack/ExMarketing/cmd/exmarketing	[no test files]
?   	github.com/ex-stack/ExMarketing/cmd/workflowctl	[no test files]
?   	github.com/ex-stack/ExMarketing/internal/decimal	[no test files]
ok  	github.com/ex-stack/ExMarketing/internal/engine	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/fact	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/httpapi	0.567s
ok  	github.com/ex-stack/ExMarketing/internal/marketing	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/mqconsumer	(cached)
?   	github.com/ex-stack/ExMarketing/internal/plugin	[no test files]
ok  	github.com/ex-stack/ExMarketing/internal/postgres	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/reward	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/rule	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/worker	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/workflow	0.514s

$ go build ./...
exit code 0

$ go test ./internal/engine ./internal/httpapi ./internal/marketing ./internal/postgres -run 'Risk|Limit|Budget|Stock|Daily|Quota|Exclusive|Duplicate|Idempot|Simulation|AtomicGrant'
ok  	github.com/ex-stack/ExMarketing/internal/engine	0.331s
ok  	github.com/ex-stack/ExMarketing/internal/httpapi	0.500s
ok  	github.com/ex-stack/ExMarketing/internal/marketing	0.162s
ok  	github.com/ex-stack/ExMarketing/internal/postgres	0.590s
~~~

未发现测试失败。无需打回 Dev Agent。

需求：2026-05-24-risk-limit-budget
