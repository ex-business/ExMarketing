# Test

## Agent 指令

你是 Test Agent。已根据 `task.md`、`analysis.md`、`review.md`、`test-plan.md`、`implementation.md` 执行测试。

本轮仅修改本文件：`tasks/2026-05-24-113929-3/test.md`。

## Test Cases

- `FIXED` 固定金额：覆盖 `TestConfigCalculatorFixedReward`，验证固定金额计算输出。
- `RATIO` 比例奖励：覆盖 `TestConfigCalculatorRatioUsesAttributesBaseFieldAndCap`、`TestConfigCalculatorRatioAvoidsFloatDriftAndAcceptsJSONNumbers`，验证 `attributes.amount`、`rate`、`maxAmount`、Decimal 精度与封顶。
- `LADDER` 连续签到阶梯：覆盖 `TestConfigCalculatorLadderMatchesClosedInterval`，验证闭区间阶梯命中，包含 `continueDays=1/3/7` 对应 `5.0000`、`18.0000`、`58.0000`。
- 明确错误路径：覆盖 `TestConfigCalculatorReturnsExplicitErrors`，包含运行时字段异常和 `LADDER` 未命中明确失败语义。
- 回归范围：执行全量 `go test ./...`、`go build ./...`，并执行真实 PostgreSQL DSN smoke 和 `-tags=integration` 全量测试。

## Commands

~~~bash
go test ./internal/reward -run 'Test.*Fixed.*|Test.*Ratio.*|Test.*Ladder.*|Test.*Calculator.*' -count=1
go test ./internal/engine -run 'Test.*Reward.*|Test.*Quota.*|Test.*Idempot.*|Test.*Fixed.*|Test.*Ratio.*|Test.*Ladder.*' -count=1
go test ./internal/marketing -run 'Test.*Prelaunch.*|Test.*Reward.*|Test.*Quota.*|Test.*Idempot.*' -count=1
go test ./internal/httpapi -run 'Test.*Reward.*|Test.*Activity.*|Test.*Validation.*|Test.*Idempot.*' -count=1
go test ./...
go build ./...
docker compose up -d postgres
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./internal/postgres -count=1
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./internal/engine ./internal/httpapi -run 'Test.*Reward.*|Test.*Quota.*|Test.*Idempot.*|Test.*Concurrent.*|Test.*Fixed.*|Test.*Ratio.*|Test.*Ladder.*' -count=1
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./...
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test -tags=integration ./...
~~~

## Result

~~~text
PASS
~~~

## Evidence

- `go test ./internal/reward ...`：PASS，`ok github.com/ex-stack/ExMarketing/internal/reward 0.262s`。
- `go test ./internal/engine ...`：PASS，`ok github.com/ex-stack/ExMarketing/internal/engine 0.206s`。
- `go test ./internal/marketing ...`：PASS，`ok github.com/ex-stack/ExMarketing/internal/marketing 0.199s`。
- `go test ./internal/httpapi ...`：PASS，`ok github.com/ex-stack/ExMarketing/internal/httpapi 0.228s`。
- `go test ./...`：PASS，所有包通过，`cmd/exmarketing`、`cmd/workflowctl`、`internal/plugin` 为 `[no test files]`。
- `go build ./...`：PASS，无输出。
- `docker compose up -d postgres`：PASS，`exmarketing-postgres` 已运行。
- PostgreSQL smoke `go test ./internal/postgres -count=1`：PASS，`ok github.com/ex-stack/ExMarketing/internal/postgres 0.323s`。
- PostgreSQL engine/httpapi 过滤测试：PASS，`internal/engine` 和 `internal/httpapi` 均通过。
- PostgreSQL DSN 全量 `go test ./...`：PASS，所有包通过。
- PostgreSQL DSN `go test -tags=integration ./...`：PASS，所有包通过。

## Failure Log

无失败命令、无失败日志。

## Notes

- 本轮没有修改业务代码、测试代码、文档代码、配置文件或其他任务文件。
- 真实 PostgreSQL 容器可用，smoke 已按计划执行；结论不只依赖内存仓储。
- 工作区测试前已有大量未提交变更，本轮未回滚、未改动这些既有变更。

需求：2026-05-24-113929-3
