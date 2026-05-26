# Test

## Agent 指令

你是 Test Agent。请根据 task.md、analysis.md、review.md、test-plan.md 和 implementation.md 执行完整测试。

最低要求：

~~~bash
go test ./...
go build ./...
~~~

## Test Cases

- Reward callback 定向回归：执行实现记录指定的 HTTP / memory / PostgreSQL callback 测试集合，覆盖成功回调、失败回调、重复回调、签名校验、locator 兼容、locator 冲突、未知单号、quota reservation 处理和 tenant 隔离相关行为。
- 全量 Go 测试：执行 `go test ./...`，覆盖仓库现有全部 Go package 测试。
- 全量 Go 构建：执行 `go build ./...`，验证所有 package 可编译。

## Commands

~~~bash
go test ./internal/httpapi ./internal/marketing ./internal/postgres -run 'TestRewardCallback|TestMemoryCallback|TestPostgresCallback' -count=1
go test ./...
go build ./...
~~~

## Result

~~~text
PASS
~~~

## Evidence

- `go test ./internal/httpapi ./internal/marketing ./internal/postgres -run 'TestRewardCallback|TestMemoryCallback|TestPostgresCallback' -count=1`
  - `ok github.com/ex-stack/ExMarketing/internal/httpapi 0.504s`
  - `ok github.com/ex-stack/ExMarketing/internal/marketing 0.331s`
  - `ok github.com/ex-stack/ExMarketing/internal/postgres 0.641s`
- `go test ./...`
  - 所有 package 通过。
  - 无测试文件 package：`cmd/exmarketing`、`cmd/workflowctl`、`internal/mqtarget`、`internal/plugin`。
  - 已测试 package 包括：`internal/decimal`、`internal/engine`、`internal/fact`、`internal/httpapi`、`internal/marketing`、`internal/mqconsumer`、`internal/postgres`、`internal/reward`、`internal/rule`、`internal/worker`、`internal/workflow`。
- `go build ./...`
  - 命令退出码为 0，无错误输出。

## Failure Details

无失败命令。

需求：2026-05-24-reward-callback
