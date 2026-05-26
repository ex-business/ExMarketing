# Test

## Agent 指令

你是 Test Agent。请根据 task.md、analysis.md、review.md、test-plan.md 和 implementation.md 执行完整测试。

最低要求：

~~~bash
go test ./...
go build ./...
~~~

## Test Cases

- 配置校验：执行 `internal/marketing` 定向测试，覆盖 Prelaunch / Validation / Reward 相关用例，验证非法概率、随机区间、奖池配置、金额类 currency 和 targetRef 等配置阻断路径。
- 奖励计算：执行 `internal/reward` 定向测试，覆盖 `RANDOM`、`PROBABILITY`、`POOL_SPLIT` 和 Decimal 相关用例，验证随机 step 边界、概率总和、空奖、奖池拆分和金额精度。
- 引擎链路：执行 `internal/engine` 定向测试，覆盖 Reward / Idempot / Quota / Concurrent 相关用例，验证 participation、quota、grant_order、outbox、幂等和并发链路。
- HTTP API：执行 `internal/httpapi` 定向测试，覆盖 Activity / Reward / Validation 相关用例，验证 Admin/API 层结构化校验行为。
- PostgreSQL smoke：执行 `internal/postgres` 定向测试，覆盖 Reward / Participation / Grant / Outbox / Idempot / Quota 相关用例，验证 repository 持久化、唯一约束和事务一致性。
- 回归：执行 `go test ./... -count=1` 和 `go build ./...`，验证全仓测试和构建。

## Commands

~~~bash
go test ./internal/reward -run 'Test.*(Random|Probability|Pool|Decimal)' -count=1
go test ./internal/marketing -run 'Test.*(Prelaunch|Validation|Reward)' -count=1
go test ./internal/engine -run 'Test.*(Reward|Idempot|Quota|Concurrent)' -count=1
go test ./internal/httpapi -run 'Test.*(Activity|Reward|Validation)' -count=1
go test ./internal/postgres -run 'Test.*(Reward|Participation|Grant|Outbox|Idempot|Quota)' -count=1
go test ./... -count=1
go build ./...
~~~

## Result

~~~text
PASS
~~~

## Evidence

- `go test ./internal/reward -run 'Test.*(Random|Probability|Pool|Decimal)' -count=1`
  - 结果：`ok github.com/ex-stack/ExMarketing/internal/reward 0.243s`
- `go test ./internal/marketing -run 'Test.*(Prelaunch|Validation|Reward)' -count=1`
  - 结果：`ok github.com/ex-stack/ExMarketing/internal/marketing 0.168s`
- `go test ./internal/engine -run 'Test.*(Reward|Idempot|Quota|Concurrent)' -count=1`
  - 结果：`ok github.com/ex-stack/ExMarketing/internal/engine 0.258s`
- `go test ./internal/httpapi -run 'Test.*(Activity|Reward|Validation)' -count=1`
  - 结果：`ok github.com/ex-stack/ExMarketing/internal/httpapi 0.472s`
- `go test ./internal/postgres -run 'Test.*(Reward|Participation|Grant|Outbox|Idempot|Quota)' -count=1`
  - 结果：`ok github.com/ex-stack/ExMarketing/internal/postgres 0.556s`
- `go test ./... -count=1`
  - 结果：全量通过。
  - 覆盖包：`internal/decimal`、`internal/engine`、`internal/fact`、`internal/httpapi`、`internal/marketing`、`internal/mqconsumer`、`internal/postgres`、`internal/reward`、`internal/rule`、`internal/worker`、`internal/workflow`。
- `go build ./...`
  - 结果：通过，无错误输出。

## Failure Details

无失败命令。

## Reproduce

在仓库根目录 `/Users/tong/Documents/GitHub/OracleBetX-Projects/ExMarketing` 执行 Commands 中列出的命令即可复现本轮测试结果。

需求：2026-05-24-113929-4
