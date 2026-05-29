# Implementation

## Agent 指令

你是 Dev Agent。只有 review.md 为 PASS 且 test-plan.md 为 PASS 时才能开发或修复。

必须按 task.md、analysis.md、review.md、test-plan.md 执行，不得扩大范围。
如果 test.md 已存在且 Result 为 FAIL，必须优先读取失败原因，做最小修复后再更新 implementation.md。
如果 commit.md 或提交日志存在 BLOCK / FAILED / 提交失败，必须优先读取提交失败上下文，做最小修复后再更新 implementation.md。

完成开发后必须先做基础验证：

~~~bash
go test ./...
go build ./...
~~~

## Summary

按 Test Agent 返工反馈做最小修复：`cmd/engine-load-fault-tests` 的 local profile 现在按 test-plan 要求执行真实并发，而不是把数据库写入并发降为 1。

```bash
go run ./cmd/engine-load-fault-tests --profile local --scenario pg-smoke
go run ./cmd/engine-load-fault-tests --profile local --scenario all
```

runner 使用真实 PostgreSQL repository、内置 `httptest` API server、内置 reward stub 和 outbox worker，不以内存仓储作为验收依据。输出包含 `PASS` / `FAIL`、runId、seed、profile、复现命令、吞吐、延迟 percentile、错误统计、retry / DLQ 和一致性摘要；失败与成功都会写 `tasks/2026-05-27-engine-load-fault-tests/runs/<runId>-load-fault.log`。

返工失败的根因有两个：

- runner 之前通过 `localDBSafeConcurrency` 把 `target > 1` 的执行并发降为 1，导致 S1/S2 只验证串行提交。
- 去掉节流后，PostgreSQL repository 的 `FindMatchingActivities` 在活动 rows 尚未关闭时继续查询 rules / rewards；25 并发下会把 10 个连接全部占住，内层查询等待连接，形成连接池饥饿。

## Changed Files

- `cmd/engine-load-fault-tests/main.go`
  - 新增 `--profile local`、`--scenario pg-smoke|all`、`--run-id`、`--seed` 参数。
  - `pg-smoke` 覆盖真实 PostgreSQL 下活动创建、审批激活、quota、事件、outbox、callback 和一致性检查。
  - `all` 覆盖 S1 并发事件规模、S2 `BUDGET` / `STOCK` / `USER_LIMIT` / `DAILY_LIMIT` 四类 quota 瓶颈、S3 重复事件和重复 callback、S4 相反终态 / 未知 commandId、S5 outbox retry + DLQ、S7 PostgreSQL timeout fault。
  - 移除本地数据库并发节流，S1 使用实际并发 25，S2 四类 quota 子用例均使用实际并发 50，并在 `executed_concurrency` 中输出真实 worker 池并发。
  - 自动 runId 改为包含真实纳秒尾号，避免同一秒并行运行覆盖日志。
  - 增加稳定 JSON line 输出：`SCENARIO ...` 和 `SUMMARY ...`。
  - 增加失败诊断：last HTTP、脱敏 DSN、数据库计数快照、logPath、reproduce command。
- `internal/postgres/repository.go`
  - `ListActivities` / `FindMatchingActivities` 先读完并关闭活动查询 rows，再加载 rules / rewards，避免真实并发下外层 rows 占满连接池导致内层查询无法获取连接。
- `internal/marketing/memory_repository_test.go`
  - 保留并发 quota 回归测试 `TestMemoryConcurrentQuotaCompetitionCoversAllQuotaTypes`，覆盖四类 quota 作为瓶颈时不会超扣、不会持久化失败链路。
- `tasks/2026-05-27-engine-load-fault-tests/implementation.md`
  - 更新本轮实现和验证记录。

## Behavior Changes

新增一个开发 / 测试命令入口，不改变生产服务启动路径和运行时业务逻辑。

local profile 使用真实 PostgreSQL 和现有业务 API / worker 链路。S1/S2 按 test-plan 要求执行真实并发压力；压测报告同时输出 target concurrency、executed concurrency、吞吐、延迟和一致性结果。

## Notes

已执行验证：

```bash
go test ./...
go build ./...
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' \
EX_MARKETING_BASE_URL='http://localhost:8080' \
go run ./cmd/engine-load-fault-tests --profile local --scenario pg-smoke
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' \
EX_MARKETING_BASE_URL='http://localhost:8080' \
go run ./cmd/engine-load-fault-tests --profile local --scenario all
```

结果：

- `go test ./...` PASS
- `go build ./...` PASS
- `pg-smoke` PASS，最近 runId：`load_fault_20260529_115845_546366000`
- `all` PASS，最近 runId：`load_fault_20260529_115747_425406000`
- `all` 输出确认：
  - S1 `target_concurrency=25`、`executed_concurrency=25`
  - S2 `BUDGET` / `STOCK` / `USER_LIMIT` / `DAILY_LIMIT` 均为 `target_concurrency=50`、`executed_concurrency=50`

需求：2026-05-27-engine-load-fault-tests
