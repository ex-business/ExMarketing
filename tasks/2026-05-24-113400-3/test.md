# Test

## Agent 指令

你是 Test Agent。请根据 task.md、analysis.md、review.md、test-plan.md 和 implementation.md 执行完整测试。

最低要求：

~~~bash
go test ./...
go build ./...
~~~

## Test Cases

- 已确认 `review.md` Decision 为 `PASS`。
- 已按 `test-plan.md` 执行必需命令：
  - `go test ./...`
  - `go build ./...`
- 已额外执行非缓存全量测试：
  - `go test -count=1 ./...`
- 已执行规则引擎聚焦测试，覆盖四阶段顺序、`RISK/LIMIT` 合并阶段、稳定规则明细、disabled rule、missing fact、未注册 evaluator、CONDITION 字段读取等：
  - `go test -count=1 ./internal/rule -run 'TestExecute|TestRegistry|TestCondition'`
- 已执行 HTTP API 聚焦测试，覆盖规则拒绝 explain、模拟不落库、幂等重复/冲突、预算不足、用户次数不足、库存不足等：
  - `go test -count=1 ./internal/httpapi -run 'TestActivityEventRuleRejection|TestSimulateEvent|TestActivityEventDuplicateAndConflict|TestActivityEventBudgetExceeded|TestActivityEventUserLimitExceeded|TestActivityEventStockExceeded'`
- 已执行 PostgreSQL smoke 包：
  - `go test -count=1 ./internal/postgres`
- 已执行 race 测试：
  - `go test -race ./...`

未新增或修改任何业务代码、测试代码、文档代码或配置文件。

## Commands

~~~bash
go test ./...
go build ./...
go test -count=1 ./internal/rule -run 'TestExecute|TestRegistry|TestCondition'
go test -count=1 ./internal/httpapi -run 'TestActivityEventRuleRejection|TestSimulateEvent|TestActivityEventDuplicateAndConflict|TestActivityEventBudgetExceeded|TestActivityEventUserLimitExceeded|TestActivityEventStockExceeded'
go test -count=1 ./internal/postgres
go test -count=1 ./...
go test -race ./...
~~~

## Result

~~~text
PASS
~~~

## Evidence

~~~text
$ go test ./...
?    github.com/ex-stack/ExMarketing/cmd/exmarketing      [no test files]
?    github.com/ex-stack/ExMarketing/cmd/workflowctl      [no test files]
?    github.com/ex-stack/ExMarketing/internal/decimal     [no test files]
?    github.com/ex-stack/ExMarketing/internal/engine      [no test files]
?    github.com/ex-stack/ExMarketing/internal/fact        [no test files]
ok   github.com/ex-stack/ExMarketing/internal/httpapi     0.420s
ok   github.com/ex-stack/ExMarketing/internal/marketing   (cached)
?    github.com/ex-stack/ExMarketing/internal/plugin      [no test files]
ok   github.com/ex-stack/ExMarketing/internal/postgres    (cached)
ok   github.com/ex-stack/ExMarketing/internal/reward      (cached)
ok   github.com/ex-stack/ExMarketing/internal/rule        (cached)
?    github.com/ex-stack/ExMarketing/internal/worker      [no test files]
ok   github.com/ex-stack/ExMarketing/internal/workflow    0.304s

$ go build ./...
exit status: 0

$ go test -count=1 ./internal/rule -run 'TestExecute|TestRegistry|TestCondition'
ok   github.com/ex-stack/ExMarketing/internal/rule        0.164s

$ go test -count=1 ./internal/httpapi -run 'TestActivityEventRuleRejection|TestSimulateEvent|TestActivityEventDuplicateAndConflict|TestActivityEventBudgetExceeded|TestActivityEventUserLimitExceeded|TestActivityEventStockExceeded'
ok   github.com/ex-stack/ExMarketing/internal/httpapi     0.219s

$ go test -count=1 ./internal/postgres
ok   github.com/ex-stack/ExMarketing/internal/postgres    0.327s

$ go test -count=1 ./...
?    github.com/ex-stack/ExMarketing/cmd/exmarketing      [no test files]
?    github.com/ex-stack/ExMarketing/cmd/workflowctl      [no test files]
?    github.com/ex-stack/ExMarketing/internal/decimal     [no test files]
?    github.com/ex-stack/ExMarketing/internal/engine      [no test files]
?    github.com/ex-stack/ExMarketing/internal/fact        [no test files]
ok   github.com/ex-stack/ExMarketing/internal/httpapi     0.301s
ok   github.com/ex-stack/ExMarketing/internal/marketing   0.325s
?    github.com/ex-stack/ExMarketing/internal/plugin      [no test files]
ok   github.com/ex-stack/ExMarketing/internal/postgres    0.473s
ok   github.com/ex-stack/ExMarketing/internal/reward      0.938s
ok   github.com/ex-stack/ExMarketing/internal/rule        0.609s
?    github.com/ex-stack/ExMarketing/internal/worker      [no test files]
ok   github.com/ex-stack/ExMarketing/internal/workflow    0.909s

$ go test -race ./...
?    github.com/ex-stack/ExMarketing/cmd/exmarketing      [no test files]
?    github.com/ex-stack/ExMarketing/cmd/workflowctl      [no test files]
?    github.com/ex-stack/ExMarketing/internal/decimal     [no test files]
?    github.com/ex-stack/ExMarketing/internal/engine      [no test files]
?    github.com/ex-stack/ExMarketing/internal/fact        [no test files]
ok   github.com/ex-stack/ExMarketing/internal/httpapi     1.608s
ok   github.com/ex-stack/ExMarketing/internal/marketing   1.776s
?    github.com/ex-stack/ExMarketing/internal/plugin      [no test files]
ok   github.com/ex-stack/ExMarketing/internal/postgres    2.335s
ok   github.com/ex-stack/ExMarketing/internal/reward      1.204s
ok   github.com/ex-stack/ExMarketing/internal/rule        2.134s
?    github.com/ex-stack/ExMarketing/internal/worker      [no test files]
ok   github.com/ex-stack/ExMarketing/internal/workflow    1.858s
~~~

## Coverage Notes

- `rg` 检查现有 `*_test.go` 未发现专门的 `WaitGroup` / goroutine / limited-budget 业务并发测试用例；本轮通过 `go test -race ./...` 补充了通用 race 检测，但它不能完全替代 `test-plan.md` 中 TC09/TC10 的并发业务场景。
- PostgreSQL 包 `go test -count=1 ./internal/postgres` 已通过，覆盖仓储层 quota 原子提交/回滚和 inbox 幂等冲突 smoke；HTTP API 聚焦测试覆盖预算、库存、用户次数不足时不返回 grant/outbox 并写 audit。
- 本轮未发现失败命令，无失败日志。

需求：2026-05-24-113400-3
