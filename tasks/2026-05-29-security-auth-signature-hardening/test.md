# Test

## Agent 指令

你是 Test Agent。已根据 `task.md`、`analysis.md`、`review.md`、`test-plan.md` 和 `implementation.md` 执行测试。

本轮只写入本文件；未修改业务代码、测试代码、文档代码、配置文件或其他文件。

## Environment

- Date: 2026-05-30
- Workdir: `/Users/tong/Documents/GitHub/OracleBetX-Projects/ExMarketing`
- PostgreSQL docker service: `docker compose up -d postgres`
- PostgreSQL container ID: `17de8a7fed4644f171d9da08a4af6132c67de1c2506bc941db2d9a732fe67c02`
- Health check: `docker compose exec -T postgres pg_isready -U exmarketing -d exmarketing`
- Health result: `/var/run/postgresql:5432 - accepting connections`
- Cleanup: `docker compose stop postgres` completed; `docker compose ps postgres` showed no running postgres service.

No local `go run ./cmd/exmarketing` service was started.

## Test Cases Executed

- OpenAPI auth/scope/idempotency/quota cases through `internal/httpapi`:
  - `TestActivityEventOpenAPIAuthRejectsUnauthorizedBeforeInboxWrite`
  - `TestActivityEventOpenAPIAuthTenantAndSourceAuthorization`
  - `TestOpenAPIEventConfigFromEnv`
  - `TestActivityEventRequiresIdempotencyKey`
  - `TestActivityEventRejectsMismatchedIdempotencyKey`
  - `TestActivityEventDuplicateAndConflict`
  - quota exceeded cases for budget, user limit, and stock.
- Webhook HMAC/timestamp/receiver/mapping/allowlist cases:
  - `TestWebhookReceiverMapsSignedPayloadIntoEventChain`
  - `TestWebhookReceiverRejectsSecurityAndMappingFailures`
- Reward Callback HMAC/timestamp/locator/idempotency/conflict/quota finalization cases:
  - `TestRewardCallbackSuccessConfirmsQuotaAndIsIdempotent`
  - `TestRewardCallbackFailedReleasesQuotaAndRejectsConflictAndUnknown`
  - `TestRewardCallbackRejectsPendingGrantOrder`
  - `TestRewardCallbackSignedIdempotencyAndLocatorConflict`
- Admin boundary cases:
  - `TestOutboxDLQQueryReplayAndClose`
  - `TestOutboxDLQOperationRequiresOperatorAndReason`
  - `TestManualGrantDecisionAndExportWriteStableAuditActions`
  - `TestQuotaTimeoutReleaseEndpointDryRunReleaseAndAudit`
  - `TestActivityApprovalFlowBlocksGoLiveUntilRequiredApprovals`
- Memory and PostgreSQL repository idempotency/concurrency/quota/callback cases:
  - `TestMemoryConcurrentDuplicateGrantRequestOnlyProcessesOnce`
  - `TestMemoryConcurrentQuotaCompetitionDoesNotOverReserve`
  - `TestMemoryConcurrentQuotaCompetitionCoversAllQuotaTypes`
  - `TestPostgresParticipationAndGrantIdempotencyUniqueConstraints`
  - `TestPostgresQuotaInsufficientBudgetHasNoDirtyWrites`
  - `TestPostgresCallbackConfirmReleaseIdempotencyAndConflict`
  - `TestPostgresCallbackTenantIsolation`
  - `TestPostgresConcurrentBudgetDoesNotOverspend`
  - `TestPostgresConcurrentStockLastItem`
  - `TestPostgresOutboxStaleRecoveryConcurrentClaim`
  - `TestPostgresManualExportReadySuccessPersistsTraceAuditAndQuota`
  - `TestPostgresManualExportConcurrentUniqueInclusionAndFailedHistoryReexport`

## Commands

```bash
go test ./...
```

Result: PASS.

Summary:

```text
ok github.com/ex-stack/ExMarketing/internal/httpapi 0.758s
ok github.com/ex-stack/ExMarketing/internal/marketing (cached)
ok github.com/ex-stack/ExMarketing/internal/postgres (cached)
ok github.com/ex-stack/ExMarketing/internal/workflow 0.865s
all other packages passed or had no test files
```

```bash
docker compose up -d postgres
docker compose exec -T postgres pg_isready -U exmarketing -d exmarketing
```

Result: PASS.

Summary:

```text
Container exmarketing-postgres Started
/var/run/postgresql:5432 - accepting connections
```

```bash
go test -race ./internal/httpapi ./internal/marketing ./internal/postgres
```

Result: PASS.

Summary:

```text
ok github.com/ex-stack/ExMarketing/internal/httpapi 1.723s
ok github.com/ex-stack/ExMarketing/internal/marketing (cached)
ok github.com/ex-stack/ExMarketing/internal/postgres (cached)
```

Focused regression for the previous Test Agent failure:

```bash
go test -race ./internal/postgres -run '^TestPostgresOutboxStaleRecoveryConcurrentClaim$' -count=10
```

Result: PASS.

Summary:

```text
ok github.com/ex-stack/ExMarketing/internal/postgres 2.005s
```

```bash
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test -count=1 ./internal/postgres
```

Result: PASS.

Summary:

```text
ok github.com/ex-stack/ExMarketing/internal/postgres 0.928s
```

```bash
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go run ./cmd/engine-e2e-postgres
```

Result: PASS.

Summary:

```text
PASS runId=e2e_pg_20260530_132425_000000000 tenantId=site_001_e2e_pg_20260530_132425_000000000 activityId=act_463040b12e2794998cde650e activityCode=e2e_pg_20260530_132425_000000000 successCommand=cmd_72801c2bf9ed50b9cd2ca859 failedCommand=cmd_3fd9cba6c189cb4ee66d72e0
```

```bash
go test -count=1 ./internal/httpapi -run 'Test.*(OpenAPI|Webhook|Callback|Admin|Idempotency|Quota|Auth|Signature|Tenant|Concurrent)'
```

Result: PASS.

Summary:

```text
ok github.com/ex-stack/ExMarketing/internal/httpapi 0.419s
```

```bash
go test -count=1 ./internal/httpapi -run 'Test(OutboxDLQQueryReplayAndClose|OutboxDLQOperationRequiresOperatorAndReason|ManualGrantDecisionAndExportWriteStableAuditActions|QuotaTimeoutReleaseEndpointDryRunReleaseAndAudit|ActivityApprovalFlowBlocksGoLiveUntilRequiredApprovals)'
```

Result: PASS.

Summary:

```text
ok github.com/ex-stack/ExMarketing/internal/httpapi 0.217s
```

```bash
go build ./...
```

Result: PASS.

No output.

```bash
docker compose stop postgres
docker compose ps postgres
```

Result: PASS.

Summary:

```text
Container exmarketing-postgres Stopped
docker compose ps postgres showed no running postgres service
```

## Observed Security Results

- OpenAPI missing/non-Bearer/empty/unknown token paths return `401 UNAUTHORIZED`; existing assertions verify no event inbox write and response redaction from full token/Authorization content.
- OpenAPI tenant mismatch returns `403 TENANT_FORBIDDEN`; source mismatch returns `403 SOURCE_FORBIDDEN`.
- OpenAPI missing or mismatched idempotency key returns `400 INVALID_IDEMPOTENCY_KEY`; duplicate same payload is idempotent; conflicting same key is rejected without extra grant/outbox/quota movement.
- OpenAPI quota exceeded paths return quota conflict behavior and audit coverage; tests assert no rewardable result and rollback/no dirty writes.
- Webhook unknown/disabled receiver, missing signature, invalid signature, expired timestamp, mapping failure, source/tenant allowlist failure, and mismatched idempotency key are covered by `TestWebhookReceiverRejectsSecurityAndMappingFailures`; tests assert stable codes including `WEBHOOK_RECEIVER_NOT_FOUND`, `WEBHOOK_RECEIVER_DISABLED`, `WEBHOOK_SIGNATURE_MISSING`, `WEBHOOK_TIMESTAMP_INVALID`, `WEBHOOK_SIGNATURE_INVALID`, `WEBHOOK_MAPPING_FAILED`, `WEBHOOK_SOURCE_FORBIDDEN`, and `INVALID_IDEMPOTENCY_KEY`.
- Webhook success/duplicate path writes receiver audit and test asserts audit payload does not leak receiver secret or raw provider body.
- Reward Callback missing/invalid/expired HMAC paths return `401` callback signature/timestamp errors; locator conflicts return `CALLBACK_LOCATOR_CONFLICT`; opposite terminal transitions return `CALLBACK_STATUS_CONFLICT`; success confirms quota once and failure releases quota once.
- Admin high-risk paths cover DLQ replay/close, operator/reason requirements, manual grant/export audit, quota timeout release, and activity approval gate behavior.
- Redaction/metrics coverage exists in `internal/httpapi` tests: metrics tests assert no high-cardinality labels such as event, activity, bill, command, idempotency, subject, or outbox IDs; OpenAPI auth tests assert token/Authorization are not returned; Webhook audit tests assert secret/raw body are not persisted in audit.

## PostgreSQL Smoke Evidence

Run ID: `e2e_pg_20260530_132425_000000000`

Tenant ID: `site_001_e2e_pg_20260530_132425_000000000`

Activity ID: `act_463040b12e2794998cde650e`

DB summary after e2e:

```text
marketing_grant_order:
  FAILED  1
  SUCCESS 1

marketing_quota_reservation:
  CONFIRMED 4 amount=13.0000
  RELEASED  4 amount=13.0000

marketing_quota:
  BUDGET      used=10.0000 frozen=0.0000
  DAILY_LIMIT used=0.0000  frozen=0.0000
  DAILY_LIMIT used=1.0000  frozen=0.0000
  STOCK       used=1.0000  frozen=0.0000
  USER_LIMIT  used=0.0000  frozen=0.0000
  USER_LIMIT  used=1.0000  frozen=0.0000

marketing_outbox:
  SENT 2
```

This proves the e2e callback success/failure smoke reached terminal grant states, confirmed/released reservations, and left no frozen quota.

## Uncovered / Follow-up

- Non-Webhook audit persistence failure behavior remains a manual/follow-up item per `test-plan.md` clarification because no explicit stable error contract was required for non-Webhook entries in this task.
- No standalone local `cmd/exmarketing` HTTP service smoke was executed. Handler-level HTTP tests and PostgreSQL e2e covered the required auth/signature/idempotency/callback behaviors without starting a foreground app service.

## Result

```text
PASS
```

需求：2026-05-29-security-auth-signature-hardening
