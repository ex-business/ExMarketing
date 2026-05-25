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

已完成参与记录与发奖单生命周期的最小修复：

```text
1. 固定 participation 与 grant_order 的稳定幂等键格式，并让内存仓储也按幂等键拒绝重复写入。
2. 将 participation.snapshot_json 从仅保存活动对象，扩展为活动配置、事件摘要、规则执行结果、奖励计算摘要。
3. 补齐 grant_order.reward_payload_json 的发奖和对账解释字段，包括 rewardCode、rewardType、calculateType、targetRef 等。
4. 同步 docs/data-model.md，明确幂等键格式、snapshot_json 和 reward_payload_json 的固定内容。
```

返工修复已按 test.md 的 FAIL 原因补齐验收覆盖：

```text
1. 新增内存仓储并发重复发奖请求测试，验证同一 participation/grant idempotency key 只有一次成功，quota 只扣一次。
2. 新增内存仓储并发 quota 竞争测试，验证预算/库存竞争只允许一次成功，不会超扣。
3. 新增 PostgreSQL participation.idempotency_key 与 grant_order.idempotency_key 唯一约束 smoke。
4. 新增 PostgreSQL grant_order 创建失败时 activity 级事务全量回滚 smoke。
5. 新增 PostgreSQL outbox 创建失败时 activity 级事务全量回滚 smoke。
```

## Changed Files

```text
internal/engine/service.go
internal/marketing/memory_repository.go
internal/marketing/memory_repository_test.go
internal/httpapi/handler_test.go
internal/postgres/repository_integration_test.go
docs/data-model.md
tasks/2026-05-24-113400-5/implementation.md
```

## Behavior Changes

```text
participation.idempotency_key:
  tenantId + ":" + activityId + ":" + eventInbox.idempotency_key

grant_order.idempotency_key:
  participation.idempotency_key + ":" + rewardCode
```

事件命中活动后的 `REJECTED`、`QUALIFIED`、`REWARDABLE` participation 都会携带可审计快照：

```text
activity
event
ruleResult
rewards
```

发奖单与 outbox 发奖命令使用增强后的 reward payload，包含奖励配置编码、计算方式、目标引用和计算解释，避免只能通过外部上下文还原发奖来源。

内存仓储对 duplicate participation/grant_order idempotency key 返回 `ErrIdempotencyConflict`；PostgreSQL 仍由已有唯一约束保证。

## Notes

`review.md` 与 `test-plan.md` 均为 PASS。本次按用户返工指令和 analysis/review/test-plan 的约束完成开发，并覆盖 review 提到的 P2 文档同步要求。

已执行验证：

```bash
go test ./internal/marketing -run 'TestMemory.*Concurrent|TestMemoryRepositoryRejectsDuplicateParticipationAndGrantIdempotencyKeys|TestMemoryAtomicGrant' -count=1 -v
go test ./... -run 'Test.*Concurrent|Test.*Idempotency|Test.*Quota' -count=1 -v
docker compose up -d postgres
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./internal/postgres -run 'TestPostgres' -count=1 -v
go test ./...
go build ./...
```

结果：

```text
PASS
```

PostgreSQL smoke 已实际覆盖并通过：

```text
TestPostgresAtomicGrantRollsBackOnStockExceeded
TestPostgresCreateInboxDetectsDuplicateAndConflict
TestPostgresParticipationAndGrantIdempotencyUniqueConstraints
TestPostgresAtomicGrantRollsBackWhenGrantOrderCreateFails
TestPostgresAtomicGrantRollsBackWhenOutboxCreateFails
```

需求：2026-05-24-113400-5
