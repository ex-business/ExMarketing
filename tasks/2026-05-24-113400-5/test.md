# Test

## Agent 指令

你是 Test Agent。请根据 task.md、analysis.md、review.md、test-plan.md 和 implementation.md 执行完整测试。

本阶段只写本文件，未修改业务代码、测试代码、文档代码或配置文件。

## Result

```text
PASS
```

本轮复验确认 Dev 返工后已补齐此前阻塞项：

```text
1. 内存仓储并发重复发奖请求只处理一次。
2. 内存仓储并发 quota 竞争不超扣。
3. PostgreSQL participation.idempotency_key 与 grant_order.idempotency_key 唯一约束 smoke。
4. PostgreSQL grant_order 创建失败时 activity 级事务回滚 smoke。
5. PostgreSQL outbox 创建失败时 activity 级事务回滚 smoke。
```

## Commands

已执行命令：

```bash
go test ./...
go build ./...
go test ./internal/marketing -run 'TestMemory.*Concurrent|TestMemoryRepositoryRejectsDuplicateParticipationAndGrantIdempotencyKeys|TestMemoryAtomicGrant' -count=1 -v
go test ./... -run 'Test.*Concurrent|Test.*Idempotency|Test.*Quota' -count=1 -v
go test -race ./...
docker compose up -d postgres
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./internal/postgres -run 'TestPostgres' -count=1 -v
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./... -count=1
```

命令结果：

```text
go test ./...：PASS
go build ./...：PASS
internal/marketing 定向并发/幂等/原子测试：PASS
全仓 Concurrent/Idempotency/Quota 定向测试：PASS
go test -race ./...：PASS
docker compose up -d postgres：PASS，容器 exmarketing-postgres Running
PostgreSQL TestPostgres smoke：PASS
带 EX_MARKETING_DATABASE_URL 的全仓测试：PASS
```

说明：

```text
go test ./... 在未设置 EX_MARKETING_DATABASE_URL 时 internal/postgres 集成用例走 skip，这是该包现有行为。
已单独启动 Docker PostgreSQL 并设置 EX_MARKETING_DATABASE_URL 复跑 internal/postgres 和全仓测试，真实数据库 smoke 已通过。
```

## Covered Test Cases

已通过覆盖：

```text
1. Happy Path：
   TestActivityEventCreatesGrantCommand 覆盖 participation、grant_order、outbox 创建，以及 participation idempotency key、snapshotJson、rewardPayloadJson 基础字段。

2. Rule Rejected：
   TestActivityEventRuleRejectionReturnsExplainWithoutGrant 覆盖规则拒绝不生成 grant_order/outbox。

3. Duplicate / Conflict：
   TestActivityEventDuplicateAndConflict、TestMemoryCreateInboxDetectsDuplicateAndConflict、TestPostgresCreateInboxDetectsDuplicateAndConflict 覆盖重复事件与幂等冲突。

4. Quota Exceeded：
   TestActivityEventBudgetExceededReturnsConflictAndAudit、TestActivityEventUserLimitExceededReturnsConflictAndAudit、TestActivityEventStockExceededReturnsConflictAndNoRewardableResult 覆盖预算、用户次数、库存不足场景。

5. 多奖励 all-or-nothing 与 quota 回滚：
   TestMemoryAtomicGrantRollsBackWhenSecondRewardStockExceeded、TestPostgresAtomicGrantRollsBackOnStockExceeded 通过。

6. 幂等唯一：
   TestMemoryRepositoryRejectsDuplicateParticipationAndGrantIdempotencyKeys、TestPostgresParticipationAndGrantIdempotencyUniqueConstraints 通过。

7. 并发重复请求：
   TestMemoryConcurrentDuplicateGrantRequestOnlyProcessesOnce 通过，验证同一 participation/grant idempotency key 仅一次成功，quota 只扣一次。

8. 并发额度竞争：
   TestMemoryConcurrentQuotaCompetitionDoesNotOverReserve 通过，验证 quota 只允许一次成功，失败请求不生成 grant_order/outbox 且不超扣。

9. PostgreSQL 事务回滚：
   TestPostgresAtomicGrantRollsBackWhenGrantOrderCreateFails、TestPostgresAtomicGrantRollsBackWhenOutboxCreateFails 通过。
```

PostgreSQL smoke 实际通过项：

```text
=== RUN   TestPostgresAtomicGrantRollsBackOnStockExceeded
--- PASS: TestPostgresAtomicGrantRollsBackOnStockExceeded
=== RUN   TestPostgresCreateInboxDetectsDuplicateAndConflict
--- PASS: TestPostgresCreateInboxDetectsDuplicateAndConflict
=== RUN   TestPostgresParticipationAndGrantIdempotencyUniqueConstraints
--- PASS: TestPostgresParticipationAndGrantIdempotencyUniqueConstraints
=== RUN   TestPostgresAtomicGrantRollsBackWhenGrantOrderCreateFails
--- PASS: TestPostgresAtomicGrantRollsBackWhenGrantOrderCreateFails
=== RUN   TestPostgresAtomicGrantRollsBackWhenOutboxCreateFails
--- PASS: TestPostgresAtomicGrantRollsBackWhenOutboxCreateFails
```

## Residual Risk

```text
1. 未发现 Zero Reward 的独立测试用例；现有测试未显示 0 金额奖励会写 REWARDABLE 或生成 grant_order/outbox。
2. 未发现 Sensitive Data 过滤的独立测试用例；现有 Happy Path 仅验证 snapshotJson 和 rewardPayloadJson 的关键解释字段存在。
3. 未发现状态流转禁止规则的独立测试用例；当前代码和测试主要覆盖生命周期状态承载与事件链路行为。
```

上述为覆盖深度风险，不阻塞本轮 PASS；test-plan.md 的最低通过标准已由本轮命令和 PostgreSQL smoke 覆盖。

需求：2026-05-24-113400-5
