# Test

## Agent 指令

你是 Test Agent。请根据 task.md、analysis.md、review.md、test-plan.md 和 implementation.md 执行完整测试。

最低要求：

~~~bash
go test ./...
go build ./...
~~~

## Test Environment

- Working directory: `/Users/tong/Documents/GitHub/OracleBetX-Projects/ExMarketing`
- Date: 2026-05-30
- PostgreSQL dependency: `docker compose up -d postgres`
- Health check: `docker compose ps postgres` showed `exmarketing-postgres` as `Up ... (healthy)` on port `54329`.
- No `cmd/exmarketing` local app server was started, so no foreground `go run` or server PID cleanup was required.

## Test Cases

1. `marketing.RedactSensitive` nested/header/URL/DSN redaction.
   - Covered by `TestRedactSensitiveRedactsNestedKeysURLsAndHeaders`.
   - Covered by `TestRedactSensitiveStringHandlesProviderStatusAndDSNLikeText`.

2. HTTP error response redaction.
   - Covered by `TestWriteErrorCodeRedactsSensitiveMessage`.

3. Audit payload redaction before persistence.
   - Covered by `TestRecordAuditRedactsSensitivePayload`.

4. Fact provider failure redaction.
   - Covered by `TestQueryRedactsProviderFailureDetails`.

5. MQ secret isolation for business `targetRef` and runtime endpoint config.
   - Covered by `TestParseAllowsMQDestinationRefs`.
   - Covered by `TestParseRejectsCredentialsAndBrokerLikeRefs`.
   - Covered by `TestLoadMQRuntimeConfigFromEnv`.

6. Manual grant export security metadata and workflow.
   - Covered by `TestMemoryManualExportRequiresReadyFileBeforeSuccess`.
   - Covered by `TestPostgresManualExportReadySuccessPersistsTraceAuditAndQuota`.

7. Backup output metadata and DSN/stderr redaction.
   - Covered by `TestBackupCreatesUniqueCustomDumpPath`.
   - Covered by `TestBackupRemovesFailedDumpAndRedactsDSN`.

8. Full regression and build validation.
   - Covered by `go test ./...`.
   - Covered by `go build ./...`.

## Commands

~~~bash
docker compose up -d postgres
docker compose ps postgres
go test ./...
go build ./...
go test ./internal/marketing -run 'TestRedactSensitive|TestMemoryManualExportRequiresReadyFileBeforeSuccess' -count=1 -v
go test ./internal/httpapi ./internal/engine ./internal/fact -run 'TestWriteErrorCodeRedactsSensitiveMessage|TestRecordAuditRedactsSensitivePayload|TestClientExecuteRedactsProviderFailure' -count=1 -v
go test ./internal/mqtarget ./internal/worker ./internal/dbbackup -run 'TestParse|TestLoadMQRuntimeConfigFromEnv|TestBackup' -count=1 -v
go test ./internal/postgres -run 'TestPostgresManualExportReadySuccessPersistsTraceAuditAndQuota' -count=1 -v
go test ./internal/fact -run 'TestQueryRedactsProviderFailureDetails' -count=1 -v
~~~

Note: the combined `internal/httpapi ./internal/engine ./internal/fact` command intentionally produced `no tests to run` for `internal/fact` because the initially selected Fact test name did not match the repository's actual test name. The corrected Fact-specific command was run afterward and passed.

## Result

~~~text
PASS
~~~

## Evidence

`docker compose up -d postgres`:

~~~text
Container exmarketing-postgres Running
~~~

`docker compose ps postgres`:

~~~text
exmarketing-postgres ... Up ... (healthy) ... 0.0.0.0:54329->5432/tcp
~~~

`go test ./...`:

~~~text
ok github.com/ex-stack/ExMarketing/internal/dbbackup (cached)
ok github.com/ex-stack/ExMarketing/internal/dbmigration (cached)
ok github.com/ex-stack/ExMarketing/internal/decimal (cached)
ok github.com/ex-stack/ExMarketing/internal/engine (cached)
ok github.com/ex-stack/ExMarketing/internal/fact (cached)
ok github.com/ex-stack/ExMarketing/internal/httpapi 0.611s
ok github.com/ex-stack/ExMarketing/internal/marketing (cached)
ok github.com/ex-stack/ExMarketing/internal/mqconsumer (cached)
ok github.com/ex-stack/ExMarketing/internal/mqtarget (cached)
ok github.com/ex-stack/ExMarketing/internal/plugin (cached)
ok github.com/ex-stack/ExMarketing/internal/postgres (cached)
ok github.com/ex-stack/ExMarketing/internal/reward (cached)
ok github.com/ex-stack/ExMarketing/internal/rule (cached)
ok github.com/ex-stack/ExMarketing/internal/worker (cached)
ok github.com/ex-stack/ExMarketing/internal/workflow 0.702s
~~~

`go build ./...`:

~~~text
PASS
~~~

Targeted security tests:

~~~text
PASS: TestRedactSensitiveRedactsNestedKeysURLsAndHeaders
PASS: TestRedactSensitiveStringHandlesProviderStatusAndDSNLikeText
PASS: TestWriteErrorCodeRedactsSensitiveMessage
PASS: TestRecordAuditRedactsSensitivePayload
PASS: TestQueryRedactsProviderFailureDetails
PASS: TestParseAllowsMQDestinationRefs
PASS: TestParseRejectsCredentialsAndBrokerLikeRefs
PASS: TestLoadMQRuntimeConfigFromEnv
PASS: TestMemoryManualExportRequiresReadyFileBeforeSuccess
PASS: TestPostgresManualExportReadySuccessPersistsTraceAuditAndQuota
PASS: TestBackupCreatesUniqueCustomDumpPath
PASS: TestBackupRemovesFailedDumpAndRedactsDSN
~~~

## Failure Notes

No failing test commands remained. No business code, test code, documentation code, configuration, or other files were modified by Test Agent.

需求：2026-05-29-security-secrets-data-export
