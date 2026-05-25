# Test

## Scope

根据 `task.md`、`analysis.md`、`review.md`、`test-plan.md`、`implementation.md` 执行 Test Agent 验证。

重点覆盖：

- 上线前配置校验基础单元与 API 测试。
- 全仓 Go 单元/集成测试。
- 构建验证。
- race 检测。
- PostgreSQL smoke 环境与执行状态。

## Commands

```bash
go test ./... -count=1
go build ./...
go test ./... -race -count=1
if [ -n "$POSTGRES_DSN" ]; then printf 'POSTGRES_DSN=set\n'; else printf 'POSTGRES_DSN=empty\n'; fi
if [ -n "$EX_MARKETING_DATABASE_URL" ]; then printf 'EX_MARKETING_DATABASE_URL=set\n'; else printf 'EX_MARKETING_DATABASE_URL=empty\n'; fi
go test ./... -count=1 -run 'Postgres|PostgreSQL|Smoke|Approval|Validation'
go test ./internal/postgres -count=1 -run 'Postgres|PostgreSQL|Smoke' -v
```

## Results

| Command | Result | Notes |
| --- | --- | --- |
| `go test ./... -count=1` | PASS | 全仓测试通过。 |
| `go build ./...` | PASS | 全仓构建通过。 |
| `go test ./... -race -count=1` | PASS | race 检测通过。 |
| `go test ./... -count=1 -run 'Postgres|PostgreSQL|Smoke|Approval|Validation'` | PASS, but incomplete | 命令退出 0，但真实 PostgreSQL 集成测试因缺少数据库 DSN 被跳过。 |
| `go test ./internal/postgres -count=1 -run 'Postgres|PostgreSQL|Smoke' -v` | SKIPPED/BLOCK | 所有 PostgreSQL 集成测试均跳过，无法满足 test-plan 的 PostgreSQL smoke 必测要求。 |

## Failure / Blocker

结论：`BLOCK`

阻塞项不是业务断言失败，而是测试环境不满足 `test-plan.md` 的硬性要求：PostgreSQL smoke 不允许跳过；本地环境缺少仓库实际使用的 `EX_MARKETING_DATABASE_URL`，同时 `POSTGRES_DSN` 也为空。

失败/阻塞命令：

```bash
go test ./internal/postgres -count=1 -run 'Postgres|PostgreSQL|Smoke' -v
```

失败日志摘要：

```text
=== RUN   TestPostgresAtomicGrantRollsBackOnStockExceeded
    repository_integration_test.go:18: EX_MARKETING_DATABASE_URL is not set
--- SKIP: TestPostgresAtomicGrantRollsBackOnStockExceeded (0.00s)
=== RUN   TestPostgresCreateInboxDetectsDuplicateAndConflict
    repository_integration_test.go:87: EX_MARKETING_DATABASE_URL is not set
--- SKIP: TestPostgresCreateInboxDetectsDuplicateAndConflict (0.00s)
=== RUN   TestPostgresParticipationAndGrantIdempotencyUniqueConstraints
    repository_integration_test.go:145: EX_MARKETING_DATABASE_URL is not set
--- SKIP: TestPostgresParticipationAndGrantIdempotencyUniqueConstraints (0.00s)
=== RUN   TestPostgresAtomicGrantRollsBackWhenGrantOrderCreateFails
    repository_integration_test.go:184: EX_MARKETING_DATABASE_URL is not set
--- SKIP: TestPostgresAtomicGrantRollsBackWhenGrantOrderCreateFails (0.00s)
=== RUN   TestPostgresAtomicGrantRollsBackWhenOutboxCreateFails
    repository_integration_test.go:222: EX_MARKETING_DATABASE_URL is not set
--- SKIP: TestPostgresAtomicGrantRollsBackWhenOutboxCreateFails (0.00s)
PASS
```

失败原因：

- `test-plan.md` 明确要求 PostgreSQL smoke 必须执行，缺少 PostgreSQL 测试环境时测试结论必须标记为 `BLOCK`。
- 当前环境变量检查结果：
  - `POSTGRES_DSN=empty`
  - `EX_MARKETING_DATABASE_URL=empty`
- 仓库 PostgreSQL 集成测试实际依赖 `EX_MARKETING_DATABASE_URL`，因此 smoke 用例全部 `SKIP`，未验证真实 PostgreSQL 表结构、迁移、DAO/repository、审批/审计持久化等要求。

复现方式：

```bash
unset POSTGRES_DSN
unset EX_MARKETING_DATABASE_URL
go test ./internal/postgres -count=1 -run 'Postgres|PostgreSQL|Smoke' -v
```

预期会看到 PostgreSQL 集成测试因 `EX_MARKETING_DATABASE_URL is not set` 被跳过。

## Evidence

全仓测试通过摘要：

```text
ok   github.com/ex-stack/ExMarketing/internal/httpapi    0.727s
ok   github.com/ex-stack/ExMarketing/internal/marketing  0.431s
ok   github.com/ex-stack/ExMarketing/internal/postgres   0.740s
ok   github.com/ex-stack/ExMarketing/internal/reward     1.010s
ok   github.com/ex-stack/ExMarketing/internal/rule       0.869s
ok   github.com/ex-stack/ExMarketing/internal/worker     1.166s
ok   github.com/ex-stack/ExMarketing/internal/workflow   1.467s
```

race 通过摘要：

```text
ok   github.com/ex-stack/ExMarketing/internal/httpapi    1.478s
ok   github.com/ex-stack/ExMarketing/internal/marketing  2.229s
ok   github.com/ex-stack/ExMarketing/internal/postgres   1.989s
ok   github.com/ex-stack/ExMarketing/internal/reward     1.740s
ok   github.com/ex-stack/ExMarketing/internal/rule       2.755s
ok   github.com/ex-stack/ExMarketing/internal/worker     2.542s
ok   github.com/ex-stack/ExMarketing/internal/workflow   1.790s
```

## Decision

```text
BLOCK
```

基础 Go 测试、构建和 race 均通过；但 PostgreSQL smoke 未真实执行，未满足 `test-plan.md` 的必测条件。需要 Dev/环境提供可用 PostgreSQL 测试库并设置 `EX_MARKETING_DATABASE_URL` 后重新执行 smoke。

需求：2026-05-24-113400-7
