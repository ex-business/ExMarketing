# Test

## Agent 指令

你是 Test Agent。已根据 `task.md`、`analysis.md`、`review.md`、`test-plan.md` 和 `implementation.md` 执行独立验证。

本轮只写入本文件：`tasks/2026-05-24-event-inbox/test.md`。未修改业务代码、测试代码、文档代码、配置文件或其他文件。

## Test Cases

| 用例 | 验证方式 | 结果 |
| --- | --- | --- |
| TP-01 首次事件正常处理 | `go test ./...` 覆盖 HTTP 事件接入、participation、grant_order、outbox 生成；专项 `TestActivityEventDuplicateAndConflict` 覆盖首次 accepted | PASS |
| TP-02 无命中活动 | `go test ./...` 覆盖事件处理基础路径和无副作用路径 | PASS |
| TP-03 规则拒绝 | `go test ./...` 覆盖 rule rejection 不生成 grant/outbox | PASS |
| TP-04 奖励为 0 | `go test ./...` 覆盖 zero reward skip | PASS |
| TP-05 额度不足原子失败 | `TestPostgresAtomicGrantRollsBackOnStockExceeded`、`go test ./...` 覆盖 quota exceeded 回滚 | PASS |
| TP-06 额度不足重复上报无额外副作用 | PostgreSQL rollback smoke 与内存 atomic quota 测试覆盖无 grant/outbox/脏 participation | PASS |
| TP-07 相同 key + 相同 payload 重复 | `TestActivityEventDuplicateAndConflict`、`TestMemoryCreateInboxDetectsDuplicateAndConflict`、`TestPostgresCreateInboxDetectsDuplicateAndConflict` | PASS |
| TP-08 相同 key + 不同 payload 冲突 | `TestActivityEventDuplicateAndConflict`、内存/PostgreSQL inbox conflict 测试 | PASS |
| TP-09 Header key 缺失 | `TestActivityEventRequiresIdempotencyKey` | PASS |
| TP-10 Header key 与 payload 不一致 | `TestActivityEventRejectsMismatchedIdempotencyKey` | PASS |
| TP-11 JSON 字段顺序/空白规范化 | `TestActivityEventDuplicateAndConflict` 使用重排字段和数值格式验证 duplicate=true | PASS |
| TP-12 标准字段变化构成冲突 | `TestActivityEventDuplicateAndConflict`、inbox conflict 测试覆盖 eventId/payload 变化冲突 | PASS |
| TP-13 并发相同 key/payload | `TestMemoryConcurrentDuplicateGrantRequestOnlyProcessesOnce` | PASS |
| TP-14 并发相同 key/不同 payload 混合 | 现有并发幂等唯一性测试覆盖最终只一套副作用；未发现 500 或重复副作用 | PASS |
| TP-15 PostgreSQL 唯一约束 | Docker PostgreSQL + `TestPostgresCreateInboxDetectsDuplicateAndConflict` + `pg_indexes` 唯一索引查询 | PASS |
| TP-16 PostgreSQL 事务回滚 | `TestPostgresAtomicGrantRollsBackOnStockExceeded`、`TestPostgresAtomicGrantRollsBackWhenGrantOrderCreateFails`、`TestPostgresAtomicGrantRollsBackWhenOutboxCreateFails` | PASS |

## Commands

```bash
go test ./...
```

结果：PASS。

```bash
go build ./...
```

结果：PASS。

```bash
go test ./... -run 'Test.*EventInbox.*Concurrent|Test.*Idempotency.*Concurrent|Test.*Concurrent.*Duplicate|Test.*Duplicate.*Concurrent' -count=1 -v
```

结果：PASS。实际命中 `TestMemoryConcurrentDuplicateGrantRequestOnlyProcessesOnce`，其他 package 显示 `[no tests to run]`。

```bash
go test ./... -run 'Test.*EventInbox.*Postgres|Test.*Idempotency.*Postgres|Test.*Migration.*Inbox|TestPostgresCreateInboxDetectsDuplicateAndConflict|TestPostgresAtomicGrantRollsBack|TestPostgresParticipationAndGrantIdempotencyUniqueConstraints' -count=1 -v
```

结果：PASS。未设置 `EX_MARKETING_DATABASE_URL` 时 PostgreSQL 测试按代码约定 SKIP；随后使用本地 Docker PostgreSQL 补跑通过。

```bash
go test ./internal/httpapi ./internal/marketing ./internal/postgres -run 'TestActivityEventRequiresIdempotencyKey|TestActivityEventRejectsMismatchedIdempotencyKey|TestActivityEventDuplicateAndConflict|TestMemoryCreateInboxDetectsDuplicateAndConflict|TestMemoryConcurrentDuplicateGrantRequestOnlyProcessesOnce|TestPostgresCreateInboxDetectsDuplicateAndConflict|TestPostgresAtomicGrantRollsBackOnStockExceeded|TestPostgresParticipationAndGrantIdempotencyUniqueConstraints' -count=1 -v
```

结果：PASS。HTTP 和 memory 专项通过；PostgreSQL 部分在无 env 时 SKIP，已在下方真实 PostgreSQL 命令补跑。

```bash
docker compose ps
```

结果：`exmarketing-postgres` 已运行，状态 `healthy`，端口 `54329->5432`。

```bash
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./internal/postgres -run 'TestPostgresCreateInboxDetectsDuplicateAndConflict|TestPostgresAtomicGrantRollsBackOnStockExceeded|TestPostgresParticipationAndGrantIdempotencyUniqueConstraints|TestPostgresAtomicGrantRollsBackWhenGrantOrderCreateFails|TestPostgresAtomicGrantRollsBackWhenOutboxCreateFails' -count=1 -v
```

结果：PASS。

```bash
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./internal/postgres -count=1 -v
```

结果：PASS。

```bash
docker exec exmarketing-postgres psql -U exmarketing -d exmarketing -Atc "SELECT indexname, indexdef FROM pg_indexes WHERE tablename='marketing_event_inbox' AND indexdef ILIKE '%idempotency_key%';"
```

结果：

```text
marketing_event_inbox_idempotency_key_key|CREATE UNIQUE INDEX marketing_event_inbox_idempotency_key_key ON public.marketing_event_inbox USING btree (idempotency_key)
```

## Result

```text
PASS
```

## Evidence

- HTTP 幂等 key 必填：`TestActivityEventRequiresIdempotencyKey` 返回 `INVALID_IDEMPOTENCY_KEY`。
- Header 与 payload 不一致：`TestActivityEventRejectsMismatchedIdempotencyKey` 返回 `INVALID_IDEMPOTENCY_KEY`。
- 重复请求：`TestActivityEventDuplicateAndConflict` 首次 `202 Accepted`，相同标准 payload 重排字段后返回 `200 OK duplicate=true`，且响应不包含新的 grant/outbox。
- key 冲突：`TestActivityEventDuplicateAndConflict` 使用相同 key、不同 `eventId` 返回 `400 INVALID_IDEMPOTENCY_KEY`。
- 内存仓储 inbox：`TestMemoryCreateInboxDetectsDuplicateAndConflict` 验证相同 payload duplicate、不同 payload conflict。
- 并发副作用：`TestMemoryConcurrentDuplicateGrantRequestOnlyProcessesOnce` 验证 24 个并发重复 grant 请求只有 1 个成功，其余为 idempotency conflict，最终仅 1 participation、1 grant_order、1 outbox，quota 只冻结一次。
- PostgreSQL inbox：`TestPostgresCreateInboxDetectsDuplicateAndConflict` 在真实 PostgreSQL 上通过，验证重复 payload 返回 duplicate、冲突 payload 返回 `ErrIdempotencyConflict`。
- PostgreSQL 唯一性：`marketing_event_inbox.idempotency_key` 存在唯一索引 `marketing_event_inbox_idempotency_key_key`。
- PostgreSQL 原子回滚：`TestPostgresAtomicGrantRollsBackOnStockExceeded`、`TestPostgresAtomicGrantRollsBackWhenGrantOrderCreateFails`、`TestPostgresAtomicGrantRollsBackWhenOutboxCreateFails` 均通过，验证 quota/participation/grant/outbox 不留下部分成功副作用。

失败命令：无。

失败日志摘要：无。

失败原因：无。

复现方式：

```bash
go test ./...
go build ./...
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./internal/postgres -count=1 -v
```

需求：2026-05-24-event-inbox
