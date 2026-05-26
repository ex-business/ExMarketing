# Test

## Scope

根据 `task.md`、`analysis.md`、`review.md`、`test-plan.md`、`implementation.md` 执行 Test Agent 验证。

本轮未修改业务代码、测试代码、文档代码、配置文件或其他文件；仅写入本文件。

## Environment

- Date: 2026-05-26
- Workspace: `/Users/tong/Documents/GitHub/OracleBetX-Projects/ExMarketing`
- Go: `go1.25.4`
- PostgreSQL smoke DSN env: `EX_MARKETING_DATABASE_URL`
- PostgreSQL smoke DSN: `postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable`
- Docker compose service: `postgres` / container `exmarketing-postgres`

## Commands

```bash
go test ./internal/reward -run 'Test.*Reward.*|Test.*Calculator.*|Test.*Schema.*' -count=1
```

Result: PASS

```text
ok  	github.com/ex-stack/ExMarketing/internal/reward	0.364s
```

```bash
go test ./internal/marketing -run 'Test.*Prelaunch.*|Test.*Reward.*|Test.*Quota.*|Test.*Idempot.*|Test.*Concurrent.*' -count=1
```

Result: PASS

```text
ok  	github.com/ex-stack/ExMarketing/internal/marketing	0.236s
```

```bash
go test ./internal/httpapi -run 'Test.*Reward.*|Test.*Activity.*|Test.*Validation.*' -count=1
```

Result: PASS

```text
ok  	github.com/ex-stack/ExMarketing/internal/httpapi	0.640s
```

```bash
go test ./internal/worker -run 'Test.*Outbox.*|Test.*Grant.*' -count=1
```

Result: PASS

```text
ok  	github.com/ex-stack/ExMarketing/internal/worker	0.463s
```

```bash
go test ./internal/postgres -run 'Test.*Repository.*|Test.*Reward.*|Test.*Quota.*|Test.*Idempot.*|Test.*Concurrent.*' -count=1
```

Result: PASS

```text
ok  	github.com/ex-stack/ExMarketing/internal/postgres	0.721s
```

```bash
go test ./... -count=1
```

Result: PASS

```text
?   	github.com/ex-stack/ExMarketing/cmd/exmarketing	[no test files]
?   	github.com/ex-stack/ExMarketing/cmd/workflowctl	[no test files]
?   	github.com/ex-stack/ExMarketing/internal/decimal	[no test files]
ok  	github.com/ex-stack/ExMarketing/internal/engine	0.350s
ok  	github.com/ex-stack/ExMarketing/internal/fact	0.197s
ok  	github.com/ex-stack/ExMarketing/internal/httpapi	0.648s
ok  	github.com/ex-stack/ExMarketing/internal/marketing	0.614s
ok  	github.com/ex-stack/ExMarketing/internal/mqconsumer	0.773s
?   	github.com/ex-stack/ExMarketing/internal/plugin	[no test files]
ok  	github.com/ex-stack/ExMarketing/internal/postgres	1.177s
ok  	github.com/ex-stack/ExMarketing/internal/reward	0.860s
ok  	github.com/ex-stack/ExMarketing/internal/rule	0.994s
ok  	github.com/ex-stack/ExMarketing/internal/worker	1.290s
ok  	github.com/ex-stack/ExMarketing/internal/workflow	1.623s
```

```bash
go build ./...
```

Result: PASS

```text
<no output>
```

```bash
docker compose up -d postgres
```

Result: PASS

```text
Container exmarketing-postgres  Running
```

```bash
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./internal/postgres -run 'Test.*Smoke.*|Test.*Repository.*' -count=1
```

Result: PASS, but no matching tests in current repository.

```text
ok  	github.com/ex-stack/ExMarketing/internal/postgres	0.254s [no tests to run]
```

说明：`test-plan.md` 示例 DSN 变量为 `EXMARKETING_DATABASE_DSN`，但当前代码读取 `EX_MARKETING_DATABASE_URL`。当前 `internal/postgres` 测试命名也没有 `Smoke` 或 `Repository` 匹配项，因此该 smoke 正则没有执行实际用例。为覆盖真实 PostgreSQL，额外执行了以下命令：

```bash
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./internal/postgres -run 'Test.*Repository.*|Test.*Reward.*|Test.*Quota.*|Test.*Idempot.*|Test.*Concurrent.*' -count=1
```

Result: PASS

```text
ok  	github.com/ex-stack/ExMarketing/internal/postgres	0.247s
```

```bash
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./internal/postgres -count=1
```

Result: PASS

```text
ok  	github.com/ex-stack/ExMarketing/internal/postgres	0.489s
```

## Test Case Mapping

| Area | Evidence |
|---|---|
| 正常路径 rewardType 覆盖 | `internal/marketing` reward schema tests and `internal/reward` calculator tests passed. |
| calculateType 覆盖 | `internal/reward` targeted calculator command passed. |
| 非法配置拒绝 | `internal/marketing` prelaunch/reward schema targeted command passed. |
| rewardType 专项负向 | `internal/marketing` targeted command passed. |
| calculateType 专项负向 | `internal/reward` and `internal/marketing` targeted commands passed. |
| 额度不足 / quota 不足 | `internal/httpapi`, `internal/marketing`, and `internal/postgres` quota/idempotency targeted commands passed. |
| 幂等重复 | `internal/httpapi`, `internal/marketing`, and real PostgreSQL `internal/postgres` commands passed. |
| 并发 | `internal/marketing` concurrent targeted command passed. |
| Outbox / grant payload | `internal/worker`, `internal/httpapi`, and `internal/reward` targeted commands passed. |
| PostgreSQL smoke | Docker PostgreSQL started; real DSN `go test ./internal/postgres -count=1` passed. |

## Findings

- No failed commands.
- No P0/P1 blocker observed in executed tests.
- Residual test-plan naming gap: `EX_MARKETING_DATABASE_URL ... -run 'Test.*Smoke.*|Test.*Repository.*'` matches no current postgres tests and returns `[no tests to run]`. This is not a product failure in this run because the full real-PostgreSQL postgres package test passed, but the smoke command in the plan should be renamed in a future test-plan cleanup if strict smoke-name enforcement is required.

## Result

```text
PASS
```

需求：2026-05-24-reward-schema
