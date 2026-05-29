# Test

## Scope

根据 `task.md`、`analysis.md`、`review.md`、`test-plan.md`、`implementation.md` 执行测试。

本轮未修改业务代码、测试代码、文档代码、配置文件或其他文件；只更新本文件。

## Commands

~~~bash
go test ./...
~~~

结果：PASS。

摘要：

~~~text
? github.com/ex-stack/ExMarketing/cmd/engine-e2e-postgres [no test files]
? github.com/ex-stack/ExMarketing/cmd/exmarketing [no test files]
? github.com/ex-stack/ExMarketing/cmd/workflowctl [no test files]
ok github.com/ex-stack/ExMarketing/internal/decimal (cached)
ok github.com/ex-stack/ExMarketing/internal/engine (cached)
ok github.com/ex-stack/ExMarketing/internal/fact (cached)
ok github.com/ex-stack/ExMarketing/internal/httpapi 0.432s
ok github.com/ex-stack/ExMarketing/internal/marketing (cached)
ok github.com/ex-stack/ExMarketing/internal/mqconsumer (cached)
? github.com/ex-stack/ExMarketing/internal/mqtarget [no test files]
ok github.com/ex-stack/ExMarketing/internal/plugin (cached)
ok github.com/ex-stack/ExMarketing/internal/postgres (cached)
ok github.com/ex-stack/ExMarketing/internal/reward (cached)
ok github.com/ex-stack/ExMarketing/internal/rule (cached)
? github.com/ex-stack/ExMarketing/internal/ruleexpr [no test files]
ok github.com/ex-stack/ExMarketing/internal/worker (cached)
ok github.com/ex-stack/ExMarketing/internal/workflow 0.533s
~~~

~~~bash
go build ./...
~~~

结果：PASS。

~~~bash
docker compose up -d postgres
~~~

结果：PASS。

摘要：

~~~text
Container exmarketing-postgres Running
~~~

~~~bash
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./...
~~~

结果：PASS。

摘要：

~~~text
? github.com/ex-stack/ExMarketing/cmd/engine-e2e-postgres [no test files]
? github.com/ex-stack/ExMarketing/cmd/exmarketing [no test files]
? github.com/ex-stack/ExMarketing/cmd/workflowctl [no test files]
ok github.com/ex-stack/ExMarketing/internal/decimal (cached)
ok github.com/ex-stack/ExMarketing/internal/engine (cached)
ok github.com/ex-stack/ExMarketing/internal/fact (cached)
ok github.com/ex-stack/ExMarketing/internal/httpapi (cached)
ok github.com/ex-stack/ExMarketing/internal/marketing (cached)
ok github.com/ex-stack/ExMarketing/internal/mqconsumer (cached)
? github.com/ex-stack/ExMarketing/internal/mqtarget [no test files]
ok github.com/ex-stack/ExMarketing/internal/plugin (cached)
ok github.com/ex-stack/ExMarketing/internal/postgres 0.634s
ok github.com/ex-stack/ExMarketing/internal/reward (cached)
ok github.com/ex-stack/ExMarketing/internal/rule (cached)
? github.com/ex-stack/ExMarketing/internal/ruleexpr [no test files]
ok github.com/ex-stack/ExMarketing/internal/worker (cached)
ok github.com/ex-stack/ExMarketing/internal/workflow (cached)
~~~

~~~bash
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./internal/postgres -run 'TestPostgres.*Manual|TestPostgres.*Export|TestPostgres.*Grant|TestPostgres.*Quota|TestPostgres.*Concurrent' -count=1
~~~

结果：PASS。

摘要：

~~~text
ok github.com/ex-stack/ExMarketing/internal/postgres 0.493s
~~~

追加执行 `test-plan.md` 建议的包级聚焦命令：

~~~bash
go test ./internal/marketing -run 'Test.*Manual|Test.*Export|Test.*Quota|Test.*Concurrent' -count=1
~~~

结果：PASS。

摘要：

~~~text
ok github.com/ex-stack/ExMarketing/internal/marketing 0.205s
~~~

~~~bash
go test ./internal/httpapi -run 'Test.*Manual|Test.*Export|Test.*Audit|Test.*Reconciliation' -count=1
~~~

结果：PASS。

摘要：

~~~text
ok github.com/ex-stack/ExMarketing/internal/httpapi 0.326s
~~~

~~~bash
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./internal/postgres -run 'Test.*Manual|Test.*Export|Test.*Concurrent|Test.*Quota' -count=1
~~~

结果：PASS。

摘要：

~~~text
ok github.com/ex-stack/ExMarketing/internal/postgres 0.659s
~~~

## Evidence

用例列表确认命令：

~~~bash
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./internal/postgres -list 'TestPostgres.*Manual|TestPostgres.*Export|TestPostgres.*Grant|TestPostgres.*Quota|TestPostgres.*Concurrent'
~~~

结果：PASS。

命中的相关 PostgreSQL 用例：

~~~text
TestPostgresAtomicGrantRollsBackOnStockExceeded
TestPostgresParticipationAndGrantIdempotencyUniqueConstraints
TestPostgresAtomicGrantRollsBackWhenGrantOrderCreateFails
TestPostgresAtomicGrantRollsBackWhenOutboxCreateFails
TestPostgresQuotaInsufficientBudgetHasNoDirtyWrites
TestPostgresConcurrentBudgetDoesNotOverspend
TestPostgresConcurrentStockLastItem
TestPostgresManualExportReadySuccessPersistsTraceAuditAndQuota
TestPostgresManualExportConcurrentUniqueInclusionAndFailedHistoryReexport
ok github.com/ex-stack/ExMarketing/internal/postgres 0.611s
~~~

命中的相关内存仓储用例：

~~~text
TestMemoryCallbackTreatsMissingReservationsAsUnlimitedWhenNoQuotaWasConfigured
TestMemoryConcurrentDuplicateGrantRequestOnlyProcessesOnce
TestMemoryConcurrentQuotaCompetitionDoesNotOverReserve
TestMemoryManualExportRequiresReadyFileBeforeSuccess
TestMemoryTimeoutReleaseSkipsManualGrantOrders
TestCreateQuotaCommandValidatesDecimalAndIntegerSemantics
~~~

命中的相关 HTTP API 用例：

~~~text
TestAuditQueryFiltersAndSnapshots
TestActivityEventBudgetExceededReturnsConflictAndAudit
TestActivityEventUserLimitExceededReturnsConflictAndAudit
TestRewardReconciliationSupportsActivitySubjectAndEventQueries
TestRewardReconciliationValidatesQueryAndSupportsPartialLinks
TestQuotaTimeoutReleaseEndpointDryRunReleaseAndAudit
~~~

## Result

~~~text
PASS
~~~

需求：2026-05-27-manual-grant-export
