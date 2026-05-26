# Test

## Scope

Test Agent 按 `task.md`、`analysis.md`、`review.md`、`test-plan.md`、`implementation.md` 执行测试。

测试前确认：

- `tasks/2026-05-24-fact-query/review.md` Decision 为 `PASS`。
- 本轮未修改业务代码、测试代码、文档代码、配置文件或其他文件；只写入本文件。

## Commands

~~~bash
go test ./...
go test -race ./...
go test ./... -run 'Fact|Provider|Mapping|Cache|Activity|Event|Quota|Idempot|Concurrent|Postgres|Integration'
go build ./...
go test ./internal/fact ./internal/httpapi ./internal/engine ./internal/postgres -list 'Fact|Provider|Mapping|Cache|Activity|Event|Quota|Idempot|Concurrent|Postgres|Integration'
~~~

未执行命令：

- `make test` / `make integration-test` / `make postgres-smoke`：仓库根目录未发现 `Makefile`。
- API curl smoke：当前未启动本地应用和 mock HTTP fact provider。
- 真实 PostgreSQL `psql` smoke：当前环境未配置 `DATABASE_URL`。

## Results

~~~text
PASS
~~~

已执行命令全部通过：

- `go test ./...`：通过。覆盖 `internal/engine`、`internal/fact`、`internal/httpapi`、`internal/marketing`、`internal/mqconsumer`、`internal/postgres`、`internal/reward`、`internal/rule`、`internal/worker`、`internal/workflow`。
- `go test -race ./...`：通过。未发现 data race。
- `go test ./... -run 'Fact|Provider|Mapping|Cache|Activity|Event|Quota|Idempot|Concurrent|Postgres|Integration'`：通过。
- `go build ./...`：通过。

## Evidence

重点测试覆盖信号：

- `internal/fact`
  - `TestQueryMapsAndCachesProviderResponse`
  - `TestQueryReturnsMappingFailureReason`
- `internal/httpapi`
  - `TestCreateActivityValidatesFactProviderReferences`
  - `TestFactQueryRejectsInlineProviderAndUsesRegistry`
  - `TestPrelaunchValidationBlocksMissingFactProvider`
  - 既有 Event / Quota / Idempotency / Simulation 相关测试通过。
- `internal/postgres`
  - `TestPostgresAtomicGrantRollsBackOnStockExceeded`
  - `TestPostgresCreateInboxDetectsDuplicateAndConflict`
  - `TestPostgresParticipationAndGrantIdempotencyUniqueConstraints`
  - `TestPostgresAtomicGrantRollsBackWhenGrantOrderCreateFails`
  - `TestPostgresAtomicGrantRollsBackWhenOutboxCreateFails`

## Residual Risk

真实 PostgreSQL 表结构 smoke、真实 HTTP API curl smoke、真实本地服务与 mock provider 联调未执行，原因是当前环境缺少运行中的应用、mock provider 和 `DATABASE_URL`。本轮已用仓库现有 Go 单元/集成测试、race 测试和构建覆盖可执行范围，未发现失败。

需求：2026-05-24-fact-query
