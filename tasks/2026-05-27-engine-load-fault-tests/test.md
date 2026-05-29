# Test

## Agent 指令

你是 Test Agent。请根据 task.md、analysis.md、review.md、test-plan.md 和 implementation.md 执行完整测试。

只允许写 `tasks/2026-05-27-engine-load-fault-tests/test.md`。

## Test Environment

- Repository: `/Users/tong/Documents/GitHub/OracleBetX-Projects/ExMarketing`
- PostgreSQL DSN used by load/fault tests: `postgres://****:****@localhost:54329/exmarketing?sslmode=disable`
- Base URL env: `http://localhost:8080`
- Test date: 2026-05-29

## Test Cases

| Case | Result | Notes |
| --- | --- | --- |
| 基础单元 / 集成测试 `go test ./...` | PASS | 全仓库 Go 测试通过。 |
| 基础构建 `go build ./...` | PASS | 全仓库 Go 构建通过。 |
| PostgreSQL 基础闭环 `--scenario pg-smoke` | PASS | 真实 PostgreSQL repository 下通过，runId=`load_fault_20260529_120053_007671000`，seed=`1780056053007793000`。 |
| TC1 并发事件压测 | PASS | S1 执行 500 条事件，`target_concurrency=25`，`executed_concurrency=25`，一致性检查 PASS。 |
| TC2 quota 并发扣减压测 | PASS | S2 覆盖 BUDGET / STOCK / USER_LIMIT / DAILY_LIMIT；每类 200 条、成功 50、预期拒绝 150，四类均 `executed_concurrency=50`。 |
| TC3 重复事件和重复 callback | PASS | duplicate=31，expected_reject=31，unexpected_error=0，一致性检查 PASS。 |
| TC4 callback 乱序 / 相反终态 | PASS | 输出 `CALLBACK_STATUS_CONFLICT` 和 `GRANT_ORDER_NOT_FOUND`，已终结状态未被覆盖，一致性检查 PASS。 |
| TC5 outbox 重试和 DLQ | PASS | retry=50，dlq=3，unexpected_error=0，一致性检查 PASS。 |
| TC7 PostgreSQL 重启或网络超时故障注入 | PASS | 使用 `context timeout via pg_sleep` 模拟数据库超时，输出 `context deadline exceeded`，恢复后一致性检查 PASS。 |
| TC8 失败诊断和复现能力 | PASS | 未设置 `EX_MARKETING_DATABASE_URL` 时退出码为 1，明确拒绝无真实 PostgreSQL 模式；成功报告包含 runId、seed、profile、logPath、reproduce。 |

## Commands

```bash
go test ./...
```

Exit code: 0

Output summary:

```text
?   	github.com/ex-stack/ExMarketing/cmd/engine-e2e-postgres	[no test files]
?   	github.com/ex-stack/ExMarketing/cmd/engine-load-fault-tests	[no test files]
?   	github.com/ex-stack/ExMarketing/cmd/exmarketing	[no test files]
?   	github.com/ex-stack/ExMarketing/cmd/workflowctl	[no test files]
ok  	github.com/ex-stack/ExMarketing/internal/httpapi	0.631s
ok  	github.com/ex-stack/ExMarketing/internal/marketing	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/postgres	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/worker	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/workflow	0.785s
```

```bash
go build ./...
```

Exit code: 0

Output summary: no output.

```bash
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' \
EX_MARKETING_BASE_URL='http://localhost:8080' \
go run ./cmd/engine-load-fault-tests --profile local --scenario pg-smoke
```

Exit code: 0

Output summary:

```text
START runId=load_fault_20260529_120053_007671000 seed=1780056053007793000 profile=local scenario=pg-smoke repository=postgres dsn=postgres://****:****@localhost:54329/exmarketing?sslmode=disable baseUrl=http://localhost:8080 log=tasks/2026-05-27-engine-load-fault-tests/runs/load_fault_20260529_120053_007671000-load-fault.log
SCENARIO pg-smoke result=PASS total=6 success=6 unexpected_error=0
events_per_second=141.03 outbox_per_second=141.03 callbacks_per_second=141.03
latency p50=15ms p95=25ms p99=25ms
consistency event_inbox=6 participation=6 grant_order=6 outbox=6 reservation=24 quota_drift=0 orphan_grant=0 orphan_outbox=0 result=PASS
PASS runId=load_fault_20260529_120053_007671000 seed=1780056053007793000 profile=local scenario=pg-smoke log=tasks/2026-05-27-engine-load-fault-tests/runs/load_fault_20260529_120053_007671000-load-fault.log
```

```bash
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' \
EX_MARKETING_BASE_URL='http://localhost:8080' \
go run ./cmd/engine-load-fault-tests --profile local --scenario all
```

Exit code: 0

Run identifiers:

```text
runId=load_fault_20260529_120100_657535000
seed=1780056060657540000
log=tasks/2026-05-27-engine-load-fault-tests/runs/load_fault_20260529_120100_657535000-load-fault.log
reproduce=EX_MARKETING_DATABASE_URL='<dsn>' EX_MARKETING_BASE_URL='http://localhost:8080' go run ./cmd/engine-load-fault-tests --profile local --scenario all --run-id 'load_fault_20260529_120100_657535000' --seed '1780056060657540000'
```

Scenario summary:

```text
S1_concurrent_events:
  result=PASS total=500 success=500 unexpected_error=0
  target_concurrency=25 executed_concurrency=25
  events_per_second=286.09 outbox_per_second=286.09 callbacks_per_second=286.09
  latency p50=81ms p95=139ms p99=183ms
  consistency: event_inbox=500 participation=500 grant_order=500 outbox=500 reservation=2000 quota_drift=0 orphan_grant=0 orphan_outbox=0 result=PASS

S2_quota_contention:
  result=PASS total=800 success=200 expected_reject=600 unexpected_error=0
  quotaTypes=[BUDGET STOCK USER_LIMIT DAILY_LIMIT]
  BUDGET: total=200 success=50 expected_reject=150 target_concurrency=50 executed_concurrency=50 consistency=PASS
  STOCK: total=200 success=50 expected_reject=150 target_concurrency=50 executed_concurrency=50 consistency=PASS
  USER_LIMIT: total=200 success=50 expected_reject=150 target_concurrency=50 executed_concurrency=50 consistency=PASS
  DAILY_LIMIT: total=200 success=50 expected_reject=150 target_concurrency=50 executed_concurrency=50 consistency=PASS

S3_S4_duplicate_callback_ordering:
  result=PASS total=122 success=30 duplicate=31 expected_reject=31 unexpected_error=0
  callbacks_per_second=30
  oppositeTerminalState=CALLBACK_STATUS_CONFLICT unknownCommand=GRANT_ORDER_NOT_FOUND
  consistency: grant_order=30 outbox=30 reservation=120 quota_drift=0 orphan_grant=0 orphan_outbox=0 result=PASS

S5_outbox_retry_dlq:
  result=PASS total=53 success=50 retry=50 dlq=3 unexpected_error=0
  consistency: participation=53 grant_order=50 outbox=50 reservation=200 quota_drift=0 orphan_grant=0 orphan_outbox=0 result=PASS

S7_postgres_timeout_fault:
  result=PASS total=100 success=100 unexpected_error=0
  fault="context timeout via pg_sleep" faultError="context deadline exceeded"
  events_per_second=89.78 latency p50=244ms p95=342ms p99=370ms
  consistency: event_inbox=1483 participation=100 grant_order=100 outbox=100 reservation=400 quota_drift=0 orphan_grant=0 orphan_outbox=0 result=PASS

SUMMARY result=PASS profile=local scenario=all
PASS runId=load_fault_20260529_120100_657535000 seed=1780056060657540000 profile=local scenario=all
```

```bash
env -u EX_MARKETING_DATABASE_URL \
EX_MARKETING_BASE_URL='http://localhost:8080' \
go run ./cmd/engine-load-fault-tests --profile local --scenario pg-smoke
```

Exit code: 1

Output summary:

```text
FAIL EX_MARKETING_DATABASE_URL is required; load/fault tests must use a real PostgreSQL repository
exit status 1
```

## Verification Notes

本轮重新验证了旧 `test.md` 中的失败点。当前 `--scenario all` 输出显示：

- S1 `target_concurrency=25`、`executed_concurrency=25`
- S2 BUDGET / STOCK / USER_LIMIT / DAILY_LIMIT 均为 `target_concurrency=50`、`executed_concurrency=50`

因此旧失败原因已修复。全量 local profile 覆盖并通过并发事件、四类 quota 并发扣减、重复 / 乱序 callback、outbox retry / DLQ、PostgreSQL timeout fault 和一致性检查。

## Result

```text
PASS
```

需求：2026-05-27-engine-load-fault-tests
