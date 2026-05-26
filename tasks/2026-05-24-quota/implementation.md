# Implementation

## Agent 指令

你是 Dev Agent。只有 review.md 为 PASS 且 test-plan.md 为 PASS 时才能开发。

必须按 task.md、analysis.md、review.md、test-plan.md 执行，不得扩大范围。

完成开发后必须先做基础验证：

~~~bash
go test ./...
go build ./...
~~~

## Summary

已按返工反馈做最小修复：补齐 `test-plan.md` 要求但此前缺失的 PostgreSQL quota、callback、concurrent 仓储级集成测试。

本轮返工检查结果：

- `review.md` Decision 为 `PASS`。
- `test-plan.md` 已补齐并为 `PASS`。
- `test.md` Result 为 `FAIL`，失败原因是 PostgreSQL 覆盖不足；本轮优先修复该失败点。
- `commit.md` 仍为模板态，未发现实际提交失败上下文。

本轮修复要点：

- 新增 `TestPostgresQuotaInsufficientBudgetHasNoDirtyWrites`，覆盖 PostgreSQL 预算不足时无 participation、grant_order、outbox、reservation 脏写，quota 聚合不漂移。
- 新增 `TestPostgresCallbackConfirmReleaseIdempotencyAndConflict`，覆盖 PostgreSQL callback `SUCCESS` 确认、重复 `SUCCESS` 幂等、`SUCCESS -> FAILED` 冲突、`FAILED` 释放、重复 `FAILED` 幂等。
- 新增 `TestPostgresCallbackTenantIsolation`，覆盖相同 `commandId` 下 callback 必须按 `tenantId + commandId` 定位，不跨租户更新 grant_order、reservation 或 quota。
- 新增 `TestPostgresConcurrentBudgetDoesNotOverspend`，覆盖 50 并发抢 `BUDGET total=100`，最多 10 个成功，`used_value + frozen_value <= total_value`。
- 新增 `TestPostgresConcurrentStockLastItem`，覆盖 20 并发抢 `STOCK total=1`，只允许 1 个成功，不超卖。
- 调整 PostgreSQL 测试 cleanup，先删除 `marketing_quota_reservation`，避免新 reservation 测试数据残留。

## Changed Files

- `internal/postgres/repository_integration_test.go`
  - 补齐 PostgreSQL quota / callback / concurrent 测试。
  - 增加测试辅助方法：PostgreSQL repo 打开、quota 值断言、grant 状态断言、并发结果统计、callback quota fixture。
- `tasks/2026-05-24-quota/implementation.md`
  - 更新本轮返工内容和验证结果。

## Behavior Changes

- 未修改生产代码行为。
- 本轮只扩大 PostgreSQL 集成测试覆盖，验证既有 quota 原子冻结、callback reservation 终结、幂等、租户隔离和并发条件更新语义。

## Notes

已执行基础验证：

```bash
docker compose up -d postgres
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./internal/postgres -run 'TestPostgres.*Quota|TestPostgres.*Callback|TestPostgres.*Concurrent|TestPostgresAtomic' -count=1 -v
go test ./...
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./... -count=1
go build ./...
```

结果均为 PASS。

已执行 SQL smoke：

```bash
docker compose exec -T postgres psql -U exmarketing -d exmarketing -c "SELECT COUNT(*) AS over_quota_count FROM marketing_quota WHERE used_value + frozen_value > total_value;"
docker compose exec -T postgres psql -U exmarketing -d exmarketing -c "SELECT tenant_id, grant_order_id, quota_type, scope, scope_key, COUNT(*) FROM marketing_quota_reservation GROUP BY tenant_id, grant_order_id, quota_type, scope, scope_key HAVING COUNT(*) > 1;"
docker compose exec -T postgres psql -U exmarketing -d exmarketing -c "SELECT COUNT(*) AS orphan_reservation_count FROM marketing_quota_reservation r LEFT JOIN marketing_grant_order g ON g.id = r.grant_order_id WHERE g.id IS NULL;"
```

结果：`over_quota_count=0`，重复 reservation 0 rows，`orphan_reservation_count=0`。

需求：2026-05-24-quota
