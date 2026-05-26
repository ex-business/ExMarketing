# Test

## Agent 指令

你是 Test Agent。请根据 task.md、analysis.md、review.md、test-plan.md 和 implementation.md 执行完整测试。

最低要求：

~~~bash
go test ./...
go build ./...
~~~

## Test Cases

- 规则引擎局部回归：覆盖 REWARD CONDITION 条件判断、空 conditions 配置错误、条件级 `Detail.conditions` 输出、disabled rule、Fact 缺失、数组 `IN/NOT_IN`、QUALIFY 失败阻断奖励和 quota 等实现说明中的规则行为。
- Engine 服务局部回归：覆盖事件处理和模拟运行链路，确认 REWARD 失败不进入奖励计算、不生成或预览 grant_order/outbox、不扣 quota；确认 COUPON 等非金额 reward 即使 `reward_amount=0.0000`，数量类权益有效时仍可生成 grant_order/outbox。
- 全量 Go 测试：覆盖仓储、HTTP API、reward、worker、workflow 等包的回归。
- 全量 Go 构建：确认所有包可编译。

## Commands

~~~bash
go test ./internal/rule ./internal/engine
go test ./...
go build ./...
~~~

## Result

~~~text
PASS
~~~

## Evidence

- `go test ./internal/rule ./internal/engine`
  - 结果：PASS
  - 输出摘要：
    - `ok github.com/ex-stack/ExMarketing/internal/rule`
    - `ok github.com/ex-stack/ExMarketing/internal/engine`
- `go test ./...`
  - 结果：PASS
  - 输出摘要：
    - `cmd/exmarketing`、`cmd/workflowctl`、`internal/decimal`、`internal/plugin`：no test files
    - `internal/engine`、`internal/fact`、`internal/httpapi`、`internal/marketing`、`internal/mqconsumer`、`internal/postgres`、`internal/reward`、`internal/rule`、`internal/worker`、`internal/workflow`：PASS
- `go build ./...`
  - 结果：PASS
  - 输出摘要：无错误输出。

## Failure Notes

无失败命令，无失败日志。无需打回 Dev Agent。

需求：2026-05-24-reward
