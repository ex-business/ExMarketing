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

本轮先检查了返工上下文：

- `review.md` 为 `PASS`，无 P0/P1 阻塞项。
- `test-plan.md` 为 `PASS`。
- `test.md` 为 `FAIL`，失败点为 `go test -race ./internal/httpapi ./internal/marketing ./internal/postgres` 中的 `TestPostgresOutboxStaleRecoveryConcurrentClaim`。
- `commit.md` 仍是模板态，未发现实质 Commit BLOCK/FAILED 上下文。

按 `test.md` 的失败原因做了最小修复：

- `TestPostgresOutboxStaleRecoveryConcurrentClaim` 改为每轮使用唯一 prefix，避免共享 PostgreSQL 或重复运行时残留的同名前缀数据影响断言。
- 本轮创建的 stale outbox 使用确定的最早 `created_at` / `updated_at`，确保并发 stale recovery 与后续 claim 优先覆盖本轮测试数据。
- 并发恢复结果只统计本轮 prefix 的 outbox ID，避免全局 `RecoverStaleOutbox` 在共享数据库中合法恢复其他历史 stale outbox 时污染本测试断言。

该修复不改变生产 durable outbox 行为；只让集成测试与 `RecoverStaleOutbox` 的全局恢复语义一致，并对共享 PostgreSQL 脏数据保持稳定。

## Changed Files

- `internal/postgres/repository_integration_test.go`
- `tasks/2026-05-29-security-auth-signature-hardening/implementation.md`

## Behavior Changes

无生产代码行为变更。本轮仅修复 PostgreSQL stale outbox recovery 并发集成测试的隔离性和 race 稳定性。

## Verification

已启动 PostgreSQL 并确认健康：

```bash
docker compose up -d postgres
docker compose exec -T postgres pg_isready -U exmarketing -d exmarketing
```

结果：

```text
/var/run/postgresql:5432 - accepting connections
```

已执行失败复现命令：

```bash
go test -race ./internal/postgres -run '^TestPostgresOutboxStaleRecoveryConcurrentClaim$' -count=10
```

结果：

```text
ok github.com/ex-stack/ExMarketing/internal/postgres 2.091s
```

已执行要求命令：

```bash
go test ./...
```

结果：`PASS`。

```bash
go test -race ./internal/httpapi ./internal/marketing ./internal/postgres
```

结果：`PASS`。

```bash
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./internal/postgres
```

结果：`PASS`。

```bash
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go run ./cmd/engine-e2e-postgres
```

结果：

```text
PASS runId=e2e_pg_20260530_132204_000000000 tenantId=site_001_e2e_pg_20260530_132204_000000000 activityId=act_99cdb6d2723018d053348c4e activityCode=e2e_pg_20260530_132204_000000000 successCommand=cmd_31d23cc81a1eb38c5a9c87fa failedCommand=cmd_92874a235d2a73038455bf51
```

```bash
go build ./...
```

结果：`PASS`。

PostgreSQL 容器已停止：

```bash
docker compose stop postgres
```

## Notes

本轮修复的是 Test Agent 返工反馈中的唯一阻塞项。OpenAPI、Webhook、Reward Callback、Admin 安全用例在上一轮 `test.md` 中已通过；本轮未扩大 auth/signature 生产代码范围。

需求：2026-05-29-security-auth-signature-hardening
