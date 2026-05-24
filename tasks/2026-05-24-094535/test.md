# Test

## Agent 指令

你是 Test Agent。请根据 `task.md`、`analysis.md` 和 `implementation.md` 设计并执行测试。

最低要求：

~~~bash
go test ./...
go build ./...
~~~

## Test Cases

### TC01 基础回归

目标：确认本次原子扣减改动没有破坏现有包级测试与编译。

步骤：

- 执行 `go test ./...`。
- 执行 `go build ./...`。

预期：

- 全部测试通过。
- 全部 package 可编译。

### TC02 HTTP: 预算不足

覆盖测试：`internal/httpapi.TestActivityEventBudgetExceededReturnsConflictAndAudit`

目标：验证 `BUDGET + ACTIVITY + TOTAL` 不足时，对外返回明确错误，并且不会生成发奖结果。

预期：

- 返回 `409 Conflict`。
- 响应 body 包含 `code=QUOTA_EXCEEDED`、`quotaType=BUDGET`。
- 响应 body 不包含 `grantOrders` 或 `outboxMessages`。
- 写入 `QUOTA_EXCEEDED` audit，payload 包含 `quotaType=BUDGET`。

### TC03 HTTP: 用户参与次数不足

覆盖测试：`internal/httpapi.TestActivityEventUserLimitExceededReturnsConflictAndAudit`

目标：验证 `USER_LIMIT + USER + <subjectID>` 不足时，返回结构化 quota 错误并记录审计。

预期：

- 返回 `409 Conflict`。
- 响应 body 包含 `code=QUOTA_EXCEEDED`、`quotaType=USER_LIMIT`、`scopeKey=<subjectID>`。
- 写入 `QUOTA_EXCEEDED` audit，payload 包含 `quotaType=USER_LIMIT`。

### TC04 HTTP: 单奖励库存不足

覆盖测试：`internal/httpapi.TestActivityEventStockExceededReturnsConflictAndNoRewardableResult`

目标：验证 `STOCK + ACTIVITY + <rewardCode>` 不足时，不返回 rewardable/grant/outbox 结果。

预期：

- 返回 `409 Conflict`。
- 响应 body 包含 `code=QUOTA_EXCEEDED`、`quotaType=STOCK`、`scopeKey=<rewardCode>`。
- 响应 body 不包含 `status=REWARDABLE`、`grantOrders` 或 `outboxMessages`。
- 写入 `QUOTA_EXCEEDED` audit，payload 包含 `quotaType=STOCK`。

### TC05 Memory Repository: 多奖励库存失败 all-or-nothing

覆盖测试：`internal/marketing.TestMemoryAtomicGrantRollsBackWhenSecondRewardStockExceeded`

目标：验证第二个 reward 库存不足时，第一个 reward 的库存扣减也回滚，且 participation/grant/outbox 均不落库。

预期：

- 返回 `ErrQuotaExceeded`。
- `participations`、`grantOrders`、`outboxMessages` 数量均为 `0`。
- 所有相关 quota 的 `used_value` 保持 `0`。

### TC06 Memory Repository: 重复幂等事件不重复扣减

覆盖测试：`internal/marketing.TestMemoryAtomicGrantDoesNotDoubleReserveOnDuplicateInbox`

目标：验证同一 inbox 幂等事件重复请求不会再次触发发奖或 quota 扣减。

预期：

- 首次 inbox 创建成功并完成原子发奖。
- 第二次同 idempotency key 返回 duplicate。
- grant/outbox 数量不增加。
- `BUDGET` 和 `STOCK` 的 `used_value` 不重复增加。

### TC07 PostgreSQL: 多奖励库存失败事务回滚

覆盖测试：`internal/postgres.TestPostgresAtomicGrantRollsBackOnStockExceeded`

目标：使用真实 PostgreSQL transaction 验证多 reward 库存不足时，participation、grant_order、outbox、quota 扣减全部 rollback。

预期：

- 返回 `ErrQuotaExceeded`。
- `marketing_participation` 中无本次测试 participation。
- `marketing_grant_order` 中无本次测试 grant。
- `marketing_outbox` 中无本次测试 outbox。
- 本次 activity 下 `marketing_quota.used_value` 均未变化。

### TC08 兼容性: 缺少 quota 行按无限制

覆盖测试：`internal/httpapi.TestActivityEventCreatesGrantCommand` 以及基础回归。

目标：验证没有创建 BUDGET/STOCK/USER_LIMIT quota 行时，既有活动仍可正常发奖。

预期：

- 返回 `202 Accepted`。
- 响应 body 包含 `grantOrders` 和 `outboxMessages`。
- 生成的奖励金额符合既有计算逻辑。

## Commands

~~~bash
go test ./...
go build ./...
docker compose up -d postgres
docker compose exec -T postgres psql -U exmarketing -d exmarketing -f /docker-entrypoint-initdb.d/002_audit_fact_outbox_indexes.sql
docker compose exec -T postgres psql -U exmarketing -d exmarketing -c "select to_regclass('public.marketing_activity') as marketing_activity, to_regclass('public.marketing_quota') as marketing_quota;"
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./...
go test -count=1 ./internal/httpapi -run 'TestActivityEvent(BudgetExceeded|UserLimitExceeded|StockExceeded)'
go test -count=1 ./internal/marketing -run 'TestMemoryAtomicGrant'
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test -count=1 ./internal/postgres -run 'TestPostgresAtomicGrantRollsBackOnStockExceeded'
~~~

说明：本地 PostgreSQL 容器已有数据卷，`001_marketing_core.sql` 由容器初始化阶段加载；该脚本不是可重复执行脚本。本轮通过 `to_regclass` 确认核心表存在，并重新执行幂等的 `002_audit_fact_outbox_indexes.sql`。

## Result

~~~text
PASS
~~~

## Evidence

- `go test ./...`：PASS。
- `go build ./...`：PASS。
- `docker compose up -d postgres`：PASS，`exmarketing-postgres` 已运行。
- `002_audit_fact_outbox_indexes.sql`：PASS，已存在对象按 notice 跳过。
- 核心表检查：PASS，`marketing_activity` 和 `marketing_quota` 均存在。
- `EX_MARKETING_DATABASE_URL=... go test ./...`：PASS，`internal/postgres` 集成测试连接本地 PostgreSQL 执行。
- HTTP quota 失败专项测试：PASS。
- Memory atomic grant 专项测试：PASS。
- PostgreSQL multi-reward stock rollback 专项测试：PASS。

## Coverage Notes

已覆盖 analysis 中要求的核心验收点：

- 预算不足返回 `409`，无 grant/outbox，并写失败审计。
- 用户次数不足返回 `409`，错误和审计包含 `USER_LIMIT`。
- 库存不足返回 `409`，错误和审计包含 `STOCK` 与 reward code。
- 多 reward 库存不足时，quota、participation、grant、outbox all-or-nothing rollback。
- 重复幂等事件不重复扣减 quota，不重复生成 grant/outbox。
- 缺少 quota 行保持无限制兼容。

剩余风险：

- 本轮未新增 PostgreSQL 并发双事件压测用例。当前 PostgreSQL 扣减路径使用条件更新并有集成事务回滚测试覆盖，但“预算或库存只剩 1 份时并发两个不同事件最终只能一个成功”的建议用例仍可作为后续增强测试补充。

需求：2026-05-24-094535
