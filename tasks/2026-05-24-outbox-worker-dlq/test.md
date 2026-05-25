# Test

## Agent 指令

你是 Test Agent。请根据 task.md、analysis.md、review.md、test-plan.md 和 implementation.md 执行完整测试。

最低要求：

~~~bash
go test ./...
go build ./...
~~~

## Test Cases

已按 `test-plan.md` 执行必要验证。

覆盖确认：

- `internal/worker/outbox_worker_test.go`
  - `TestOutboxWorkerSendsHTTPAndMarksSent`
  - `TestOutboxWorkerRetryableFailureUsesExponentialBackoff`
  - `TestOutboxWorkerRetryAfterOverridesBackoff`
  - `TestOutboxWorkerNonRetryableFailureMovesToDLQ`
  - `TestOutboxWorkerMaxRetriesMovesToDLQ`
  - `TestOutboxWorkerSkipsMQMessages`
- `internal/httpapi/handler_test.go`
  - `TestOutboxDLQQueryReplayAndClose`
  - `TestOutboxDLQOperationRequiresOperatorAndReason`
- 全量回归：
  - `go test ./...`
  - `go build ./...`

本轮未修改业务代码、测试代码、文档代码、配置文件或其他文件；只写入本 `test.md`。

## Commands

~~~bash
go test ./...
go build ./...
~~~

## Result

~~~text
PASS
~~~

## Evidence

`go test ./...` 结果：

~~~text
?   	github.com/ex-stack/ExMarketing/cmd/exmarketing	[no test files]
?   	github.com/ex-stack/ExMarketing/cmd/workflowctl	[no test files]
?   	github.com/ex-stack/ExMarketing/internal/decimal	[no test files]
?   	github.com/ex-stack/ExMarketing/internal/engine	[no test files]
?   	github.com/ex-stack/ExMarketing/internal/fact	[no test files]
ok  	github.com/ex-stack/ExMarketing/internal/httpapi	0.557s
ok  	github.com/ex-stack/ExMarketing/internal/marketing	(cached)
?   	github.com/ex-stack/ExMarketing/internal/plugin	[no test files]
ok  	github.com/ex-stack/ExMarketing/internal/postgres	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/reward	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/rule	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/worker	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/workflow	0.709s
~~~

`go build ./...` 结果：

~~~text
PASS
~~~

失败命令：无。

失败日志摘要：无。

失败原因：无。

复现方式：

~~~bash
cd /Users/tong/Documents/GitHub/OracleBetX-Projects/ExMarketing
go test ./...
go build ./...
~~~

需求：2026-05-24-outbox-worker-dlq
