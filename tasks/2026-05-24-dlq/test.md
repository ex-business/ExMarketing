# Test

## Agent 指令

你是 Test Agent。请根据 task.md、analysis.md、review.md、test-plan.md 和 implementation.md 执行完整测试。

本轮严格遵守写入限制：只修改 `tasks/2026-05-24-dlq/test.md`，未修改业务代码、测试代码、文档代码、配置文件或其他文件。

## Test Cases

已按 `test-plan.md` 执行现有自动化测试与构建验证。

覆盖到的关键用例：

- DLQ 查询、tenant scoped 查询、replay、close、close 后不可 replay、审计字段：`internal/httpapi.TestOutboxDLQQueryReplayAndClose`
- replay / close 入参校验：`internal/httpapi.TestOutboxDLQOperationRequiresOperatorAndReason`
- close 只关闭 outbox，不联动修改 grant_order / participation / quota reservation：`internal/marketing.TestMemoryCloseOutboxDLQOnlyClosesOutbox`
- PostgreSQL 下 close 只关闭 outbox，不联动修改业务状态：`internal/postgres.TestPostgresCloseOutboxDLQOnlyClosesOutbox`
- Outbox Worker 进入 DLQ 的失败路径：`internal/worker.TestOutboxWorkerNonRetryableFailureMovesToDLQ`、`TestOutboxWorkerMaxRetriesMovesToDLQ`、`TestOutboxWorkerInvalidTargetMovesToDLQ`、`TestOutboxWorkerInvalidPayloadMovesToDLQ`、`TestOutboxWorkerMQInvalidTargetMovesToDLQ`
- 事件 / grant / outbox 幂等与并发基础保护：`internal/marketing.TestMemoryRepositoryRejectsDuplicateParticipationAndGrantIdempotencyKeys`、`TestMemoryConcurrentDuplicateGrantRequestOnlyProcessesOnce`、`TestMemoryConcurrentQuotaCompetitionDoesNotOverReserve`
- PostgreSQL 幂等、事务回滚、并发 quota / stock 保护：`internal/postgres.TestPostgresParticipationAndGrantIdempotencyUniqueConstraints`、`TestPostgresAtomicGrantRollsBackWhenGrantOrderCreateFails`、`TestPostgresAtomicGrantRollsBackWhenOutboxCreateFails`、`TestPostgresConcurrentBudgetDoesNotOverspend`、`TestPostgresConcurrentStockLastItem`

测试缺口记录：

- `go test ./... -race -run 'Test.*DLQ.*Concurrent|Test.*Outbox.*Concurrent|Test.*Replay.*Concurrent|Test.*Close.*Concurrent|Test.*Concurrent.*DLQ|Test.*Concurrent.*Outbox|Test.*Concurrent.*Replay|Test.*Concurrent.*Close'` 未匹配到现有 DLQ replay/close 并发专项用例，命令通过但各包报告 no tests to run。
- 本轮不能新增测试代码，因此未补写 test-plan 中的并发 replay vs replay、并发 replay vs close、审计失败事务一致性专项测试。该缺口未导致现有测试失败，但建议后续 Dev/Test 补齐。

## Commands

```bash
go test ./... -list '.*DLQ|.*Outbox.*Replay|.*Outbox.*Close|.*Audit|.*Postgres|.*Repository|.*Integration|.*Concurrent'
go test ./... -run 'Test.*DLQ|Test.*Outbox.*Replay|Test.*Outbox.*Close|Test.*Audit'
go test ./... -run 'Test.*Postgres|Test.*Repository|Test.*Integration'
go test ./... -race -run 'Test.*DLQ.*Concurrent|Test.*Outbox.*Concurrent|Test.*Replay.*Concurrent|Test.*Close.*Concurrent|Test.*Concurrent.*DLQ|Test.*Concurrent.*Outbox|Test.*Concurrent.*Replay|Test.*Concurrent.*Close'
go test ./... -race -run 'Test.*Concurrent|Test.*concurrent'
go test ./...
go build ./...
```

## Result

```text
PASS
```

## Evidence

命令结果摘要：

- `go test ./... -list ...`：PASS，列出 DLQ / Audit / Postgres / Repository / Concurrent 相关现有测试；确认存在 HTTP、MemoryRepository、PostgreSQL、Worker DLQ 覆盖。
- `go test ./... -run 'Test.*DLQ|Test.*Outbox.*Replay|Test.*Outbox.*Close|Test.*Audit'`：PASS。
- `go test ./... -run 'Test.*Postgres|Test.*Repository|Test.*Integration'`：PASS。
- `go test ./... -race -run 'Test.*DLQ.*Concurrent|...'`：PASS，但未匹配 DLQ 并发专项用例。
- `go test ./... -race -run 'Test.*Concurrent|Test.*concurrent'`：PASS，覆盖现有并发测试。
- `go test ./...`：PASS，所有 Go package 测试通过。
- `go build ./...`：PASS，无构建错误。

未出现测试失败，因此无失败命令、失败日志、失败原因或复现步骤需要打回 Dev Agent。

需求：2026-05-24-dlq
