# Test

## Agent 指令

你是 Test Agent。请根据 task.md、analysis.md、review.md、test-plan.md 和 implementation.md 执行完整测试。

最低要求：

~~~bash
go test ./...
go build ./...
~~~

## Test Cases

已按 `test-plan.md` 的 Required Commands 执行自动化测试，覆盖本期最小可验收范围：

1. `POST /admin/v1/simulate` 规则解释但不持久化：
   - `TestSimulateEventReturnsRuleExplainWithoutPersistence`
   - 覆盖规则失败返回原因、模拟接口不生成真实 grant/outbox。
2. 命中 ACTIVE 活动并返回活动级 preview：
   - `TestSimulateEventReturnsActivityAndQuotaPreviewsWithoutPersistence`
   - 覆盖 `matchedActivities`、`rewardPreviews`、`quotaImpactPreviews`、`participationPreview`、`grantOrderPreviews`、`outboxCommandPreviews`。
3. quota 不足时只返回风险预览：
   - `TestSimulateEventQuotaExceededSuppressesGrantPreview`
   - 覆盖 `wouldExceed=true`、`QUOTA_EXCEEDED`、`hasQuotaRisk=true`，并断言不返回 grant/outbox preview。
4. 正式事件链路回归：
   - `TestActivityEventDuplicateAndConflict`
   - `TestActivityEventBudgetExceededReturnsConflictAndAudit`
   - `TestActivityEventUserLimitExceededReturnsConflictAndAudit`
   - `TestActivityEventStockExceededReturnsConflictAndNoRewardableResult`
   - 覆盖正式幂等、正式 quota 超限、无 grant/outbox 错误输出。
5. 仓储无副作用与并发回归：
   - `TestMemoryAtomicGrantRollsBackWhenSecondRewardStockExceeded`
   - `TestMemoryAtomicGrantDoesNotDoubleReserveOnDuplicateInbox`
   - `TestMemoryConcurrentDuplicateGrantRequestOnlyProcessesOnce`
   - `TestMemoryConcurrentQuotaCompetitionDoesNotOverReserve`
   - `TestPostgresAtomicGrantRollsBackOnStockExceeded`
   - `TestPostgresAtomicGrantRollsBackWhenGrantOrderCreateFails`
   - `TestPostgresAtomicGrantRollsBackWhenOutboxCreateFails`
   - 覆盖 participation、grant_order、outbox 回滚和 quota 不超扣。

## Commands

~~~bash
go test ./...
go test -race ./...
go build ./...
docker compose up -d postgres
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./...
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test -race ./...
docker compose down
~~~

执行结果摘要：

~~~text
go test ./...
PASS
主要包：
ok github.com/ex-stack/ExMarketing/internal/httpapi 0.791s
ok github.com/ex-stack/ExMarketing/internal/marketing cached
ok github.com/ex-stack/ExMarketing/internal/postgres cached
ok github.com/ex-stack/ExMarketing/internal/reward cached
ok github.com/ex-stack/ExMarketing/internal/rule cached
ok github.com/ex-stack/ExMarketing/internal/worker cached
ok github.com/ex-stack/ExMarketing/internal/workflow 0.673s

go test -race ./...
PASS
主要包：
ok github.com/ex-stack/ExMarketing/internal/httpapi 2.709s
ok github.com/ex-stack/ExMarketing/internal/marketing 1.723s
ok github.com/ex-stack/ExMarketing/internal/postgres 1.483s
ok github.com/ex-stack/ExMarketing/internal/rule 2.220s
ok github.com/ex-stack/ExMarketing/internal/worker 1.265s
ok github.com/ex-stack/ExMarketing/internal/workflow 2.174s

go build ./...
PASS

docker compose up -d postgres
PASS
Container exmarketing-postgres Running

EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./...
PASS
主要包：
ok github.com/ex-stack/ExMarketing/internal/httpapi cached
ok github.com/ex-stack/ExMarketing/internal/marketing cached
ok github.com/ex-stack/ExMarketing/internal/postgres 0.300s
ok github.com/ex-stack/ExMarketing/internal/reward cached
ok github.com/ex-stack/ExMarketing/internal/rule cached
ok github.com/ex-stack/ExMarketing/internal/worker cached
ok github.com/ex-stack/ExMarketing/internal/workflow cached

EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test -race ./...
PASS
主要包：
ok github.com/ex-stack/ExMarketing/internal/httpapi cached
ok github.com/ex-stack/ExMarketing/internal/marketing cached
ok github.com/ex-stack/ExMarketing/internal/postgres 1.363s
ok github.com/ex-stack/ExMarketing/internal/reward cached
ok github.com/ex-stack/ExMarketing/internal/rule cached
ok github.com/ex-stack/ExMarketing/internal/worker cached
ok github.com/ex-stack/ExMarketing/internal/workflow cached

docker compose down
PASS
Container exmarketing-postgres Stopped/Removed
Network exmarketing_default Removed
~~~

## Result

~~~text
PASS
~~~

## Evidence

1. 全量单元测试、race 测试和 build 均通过。
2. PostgreSQL 容器环境下的全量测试和 race 测试均通过，`internal/postgres` 集成测试实际执行。
3. 模拟运行相关 HTTP 测试覆盖成功 preview、规则拒绝、quota 超限风险预览，并断言模拟接口不返回真实 grant/outbox 写入结果。
4. 仓储层回滚与幂等测试覆盖真实 participation、grant_order、outbox、quota 的无副作用/不重复写入边界。
5. 未执行可选手工 HTTP smoke；当前自动化测试已覆盖本期 Required Commands 和核心验收点。

失败命令：无。

失败日志摘要：无。

失败原因：无。

复现方式：

~~~bash
cd /Users/tong/Documents/GitHub/OracleBetX-Projects/ExMarketing
go test ./...
go test -race ./...
go build ./...
docker compose up -d postgres
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./...
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test -race ./...
docker compose down
~~~

需求：2026-05-24-113400-8
