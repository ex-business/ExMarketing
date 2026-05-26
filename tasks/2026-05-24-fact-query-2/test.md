# Test

## Agent 指令

你是 Test Agent。请根据 task.md、analysis.md、review.md、test-plan.md 和 implementation.md 执行完整测试。

最低要求：

~~~bash
go test ./...
go build ./...
~~~

## Test Cases

- 已确认 `review.md` 为 PASS，`test-plan.md` 为 PASS，允许执行测试。
- 仓库级基础测试：覆盖全部 Go package 的单元测试与现有集成型测试。
- 仓库级 race 测试：覆盖并发与数据竞争风险。
- 构建验证：覆盖所有 package 编译。
- integration tag 变体：按 test-plan 要求执行 `-tags=integration`。
- Fact Query 定向验证：执行 `internal/fact`、`internal/httpapi`、`internal/engine` 中 Fact/Provider/Mapping/Cache 相关测试，覆盖：
  - provider response mapping 与未声明字段不透传。
  - 成功结果缓存、`traceId` 不影响缓存 key。
  - 标准事件上下文和 provider 配置版本影响缓存 key。
  - `cache_ttl_seconds=0` 不缓存。
  - mapping 失败 reason：`MAPPING_REQUIRED_MISSING`、`MAPPING_TYPE_MISMATCH`、`MAPPING_KEY_INVALID`、`MAPPING_SOURCE_INVALID`、`MAPPING_CONFIG_INVALID`。
  - Fact Query 拒绝 inline provider 配置并使用 registry。
  - disabled / missing fact provider 的校验路径。
- PostgreSQL smoke：本地 Docker Compose Postgres 已运行且 healthy，使用 `EX_MARKETING_DATABASE_URL=postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable` 执行 Postgres 过滤测试。

## Commands

~~~bash
go test ./...
go build ./...
go test -race ./...
go test ./... -tags=integration
go test ./... -run 'Test.*Postgres.*|Test.*Fact.*Smoke.*' -count=1 -v
go test ./internal/fact ./internal/httpapi ./internal/engine -run 'Fact|facts|Facts|Provider|Mapping|Cache' -count=1 -v
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./internal/postgres -run 'Test.*Postgres.*|Test.*Fact.*Smoke.*' -count=1 -v
~~~

## Result

~~~text
PASS
~~~

## Evidence

- `go test ./...`：PASS。全部 package 通过；无失败用例。
- `go build ./...`：PASS。
- `go test -race ./...`：PASS。核心 package 均通过 race 检测，包括 `internal/fact`、`internal/httpapi`、`internal/postgres`、`internal/worker`。
- `go test ./... -tags=integration`：PASS。
- `go test ./... -run 'Test.*Postgres.*|Test.*Fact.*Smoke.*' -count=1 -v`：PASS；未设置数据库环境变量时，Postgres 测试按代码逻辑 skip。
- `go test ./internal/fact ./internal/httpapi ./internal/engine -run 'Fact|facts|Facts|Provider|Mapping|Cache' -count=1 -v`：PASS。实际执行并通过的关键用例包括：
  - `TestQueryMapsAndCachesProviderResponse`
  - `TestQueryReturnsMappingFailureReason`
  - `TestCacheKeyIncludesStandardContextAndProviderVersionButNotTraceID`
  - `TestQueryDoesNotCacheWhenTTLDisabled`
  - `TestQueryMappingFailureReasons`
  - `TestCreateActivityValidatesFactProviderReferences`
  - `TestCreateActivityRejectsDisabledAndMissingFactMapping`
  - `TestFactQueryRejectsInlineProviderAndUsesRegistry`
  - `TestFactQueryReturnsDisabledProviderCode`
  - `TestPrelaunchValidationBlocksMissingFactProvider`
- `EX_MARKETING_DATABASE_URL=... go test ./internal/postgres -run 'Test.*Postgres.*|Test.*Fact.*Smoke.*' -count=1 -v`：PASS。真实本地 Postgres smoke 执行并通过 12 个 Postgres 用例，包括 quota rollback、inbox idempotency、tenant isolation、concurrent budget/stock 等。

无失败命令，无需打回 Dev Agent。

需求：2026-05-24-fact-query-2
