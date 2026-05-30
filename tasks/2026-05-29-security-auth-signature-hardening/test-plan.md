# Test Plan

## Scope

This plan is written after `review.md` Decision is `PASS`.

The Test Agent must verify auth/signature hardening for:

- OpenAPI Event API: bearer token, tenant/source scope, idempotency, quota rollback.
- Webhook Receiver: HMAC signature, timestamp replay window, receiver/source allowlist, mapped event idempotency.
- Reward Callback: tenant-scoped HMAC, locator binding, terminal idempotency, quota finalization.
- Admin high-risk writes: tenant guard, operator/reason boundary, audit redaction.
- PostgreSQL smoke: same security and idempotency behavior on the durable repository path.

Review clarification to enforce in tests:

- Reward Callback acceptance path is tenant-scoped `X-ExMarketing-Timestamp` plus `X-ExMarketing-Signature` HMAC-SHA256. Do not accept an unspecified "equivalent token" mode as passing this task.
- Audit persistence failure is a hard blocker for Webhook because `WEBHOOK_AUDIT_FAILED` is documented. For non-Webhook entries, assert "no business mutation on audit failure" only where the implementation has an explicit stable error contract; otherwise record as manual/follow-up evidence instead of inventing a new expected code.

## Test Strategy

Use unit/handler tests for deterministic request validation and memory-repository state assertions. Use PostgreSQL integration tests and the existing PostgreSQL e2e command for durable transaction behavior, quota rollback, duplicate handling, and callback finalization.

For every negative security request, assert all of the following:

- HTTP status and stable machine `errorCode`.
- No mutation to event inbox, participation, grant order, outbox, quota reservation, quota aggregate, or terminal grant state.
- Security audit or security log exists when the implementation exposes it.
- Audit/log/metrics output does not contain full `Authorization`, token, secret, raw request body, full `targetRef`, cookie, password, or plaintext credentials.

Use unique tenant IDs, event IDs, business IDs, command IDs, and idempotency keys per test run. Cross-tenant cases must create valid resources in tenant A, then attempt update or callback from tenant B with otherwise valid identifiers.

## Test Cases

### 1. OpenAPI Normal Path

- Configure `RequireAuth=true` with a token scoped to `tenantId=site_001` and `sourceSystem=payment`.
- POST `/openapi/v1/events` with:
  - `Authorization: Bearer <valid-token>`
  - `Content-Type: application/json`
  - `Idempotency-Key: site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:<bizId>`
  - matching standard event payload.
- Expect accepted success.
- Assert exactly one event inbox is created and downstream participation/grant/outbox/quota effects match the existing reward flow.
- Assert response and audit do not expose token or Authorization header.

### 2. OpenAPI Auth And Scope Failures

Run separate cases for:

- Missing `Authorization`.
- Non-Bearer scheme.
- Empty bearer token.
- Unknown token.
- Token scoped to `site_001`, payload uses `site_002`.
- Token scoped to `payment`, payload uses `crm`.

Expected results:

- Missing/invalid token returns `401 UNAUTHORIZED`.
- Tenant mismatch returns `403 TENANT_FORBIDDEN`.
- Source mismatch returns `403 SOURCE_FORBIDDEN`.
- No event inbox, participation, grant order, outbox, quota reservation, or quota aggregate mutation.
- Failure audit/log is present and redacted.

### 3. OpenAPI Idempotency

- Submit one valid event and keep its response.
- Submit the exact same request again with the same `Idempotency-Key`.
- Expect duplicate response, no second participation/grant/outbox/quota reservation, and no extra quota movement.
- Submit same `Idempotency-Key` with changed `eventId`, `bizId`, `subject.id`, or attributes.
- Expect `400 INVALID_IDEMPOTENCY_KEY`.
- Assert state remains equal to the state after the first successful request.

### 4. OpenAPI Quota Insufficient

- Create an active activity with budget/user/stock quota that is already exhausted or has total lower than the reward reservation amount.
- Submit an otherwise valid authenticated event.
- Expect stable quota failure, preferably `409 QUOTA_EXCEEDED`.
- Assert rollback:
  - no participation remains for the failed event,
  - no grant order remains,
  - no outbox remains,
  - quota `used_value` and `frozen_value` are unchanged.
- Assert `QUOTA_EXCEEDED` audit is redacted and includes enough non-sensitive identifiers for support.

### 5. Webhook Normal Path

- Configure enabled receiver with secret, allowed tenant `site_001`, allowed source `payment`, and mapping to a standard `DEPOSIT_SUCCESS` event.
- Build signature as lowercase hex HMAC-SHA256 over `<timestamp>.<raw_body>`.
- POST `/openapi/v1/webhooks/{receiverCode}` with valid timestamp, signature, JSON body, and optional matching `Idempotency-Key`.
- Expect accepted success through the same standard event chain as OpenAPI.
- Assert mapped tenant/source cannot be overridden by raw provider fields outside the configured mapping.

### 6. Webhook Signature, Timestamp, Receiver, And Allowlist Failures

Run separate cases for:

- Unknown receiver: `WEBHOOK_RECEIVER_NOT_FOUND`.
- Disabled receiver: `WEBHOOK_RECEIVER_DISABLED`.
- Missing signature: `WEBHOOK_SIGNATURE_MISSING`.
- Missing, malformed, future-skewed, or expired timestamp: `WEBHOOK_TIMESTAMP_INVALID`.
- Wrong HMAC, wrong body after signing, wrong secret, or non-hex signature: `WEBHOOK_SIGNATURE_INVALID`.
- Source IP, mapped tenant, or mapped source not allowed: `WEBHOOK_SOURCE_FORBIDDEN` or the implementation's documented stable equivalent.
- Mapping failure or missing mapped required field: `WEBHOOK_MAPPING_FAILED`.
- Optional `Idempotency-Key` does not match mapped event: `INVALID_IDEMPOTENCY_KEY`.

Expected results for all failures:

- No standard event business write.
- No quota movement.
- Failure audit/log is redacted.
- Webhook audit persistence failure, if injectable, returns `WEBHOOK_AUDIT_FAILED` and does not write business state.

### 7. Reward Callback Normal Path

- Create a pending grant order in `tenantId=site_001` with quota reservation in `FROZEN`.
- Configure callback secret for `site_001`.
- POST `/openapi/v1/reward/callback` with valid timestamp/HMAC, `tenantId`, matching `commandId` or `billNo`, matching `idempotencyKey`, `status=SUCCESS`, and `externalGrantId`.
- Expect success.
- Assert grant order becomes terminal success, participation state is updated as expected, quota reservation becomes `CONFIRMED`, quota aggregate moves from frozen to used, and callback audit is redacted.

### 8. Reward Callback Failure And Release Path

- Create a pending grant order in `tenantId=site_001` with quota reservation in `FROZEN`.
- POST valid signed callback with `status=FAILED`.
- Expect success.
- Assert grant order becomes terminal failed, reservation becomes `RELEASED`, frozen quota is released, used quota is not increased, and no outbox retry remains incorrectly pending for the terminal order.

### 9. Reward Callback Auth, Tenant, And Locator Failures

Run separate cases for:

- Missing signature: `401 CALLBACK_SIGNATURE_MISSING`.
- Missing, malformed, future-skewed, or expired timestamp: `401 CALLBACK_TIMESTAMP_INVALID`.
- Wrong HMAC or body changed after signing: `401 CALLBACK_SIGNATURE_INVALID`.
- Missing `tenantId`.
- Missing all locators: no `billNo`, no `commandId`, no `idempotencyKey`.
- `commandId` exists in tenant A, request uses tenant B.
- `billNo` and `commandId` point to different orders.
- `commandId` and `idempotencyKey` point to different orders.
- `status=SUCCESS` without `externalGrantId`.

Expected results:

- Stable callback error code, including `CALLBACK_LOCATOR_CONFLICT` for locator mismatches.
- No grant order terminal state change.
- No quota reservation or aggregate movement.
- No participation terminal update.

### 10. Reward Callback Idempotency And Conflict

- Repeat the exact same successful `SUCCESS` callback.
- Expect duplicate/idempotent response and no second quota movement.
- Repeat the exact same successful `FAILED` callback.
- Expect duplicate/idempotent response and no second quota release.
- Send `SUCCESS` then `FAILED` for the same order.
- Send `SUCCESS` twice with different `externalGrantId`.
- Send callback for already closed or manually finalized order.
- Expect stable conflict code such as `CALLBACK_STATUS_CONFLICT`, `CALLBACK_LOCATOR_CONFLICT`, `GRANT_ORDER_CLOSED`, or documented equivalent.
- Assert state remains equal to the first terminal state.

### 11. Admin Boundary Tests

For high-risk Admin writes, cover at least manual grant decision, manual export, DLQ replay/close, quota timeout release, activity approve/reject, and quota mutation where handlers exist.

Run separate cases for:

- Missing `tenantId`.
- Missing `operator`.
- Missing `reason`.
- Request tenant A attempts to mutate resource owned by tenant B.
- Resource ID exists but is not in the requested tenant.
- Invalid state transition, such as replaying a non-DLQ outbox or timeout-release for an already terminal grant.

Expected results:

- Stable Admin error code, for example `ADMIN_TENANT_FORBIDDEN`, `ADMIN_OPERATOR_REQUIRED`, `ADMIN_REASON_REQUIRED`, or documented equivalent.
- No business mutation.
- High-risk success and failure paths write redacted audit with action, tenant, resource, operator, reason, result, and no secret/raw credential content.

### 12. Concurrency Tests

Run with the race detector where practical.

- Concurrently submit the same OpenAPI event 20-50 times with the same valid `Idempotency-Key`; expect one success and the rest duplicate, with exactly one downstream grant/outbox/quota reservation set.
- Concurrently submit same `Idempotency-Key` with mixed payloads; expect at most one accepted canonical payload and all mismatched payloads rejected with `INVALID_IDEMPOTENCY_KEY`.
- Concurrently submit the same signed Reward Callback 20-50 times; expect one terminal transition and the rest duplicate/idempotent, with quota moved exactly once.
- Concurrently submit conflicting `SUCCESS` and `FAILED` callbacks; expect exactly one terminal state to win and all opposite terminal requests rejected without partial quota movement.
- Concurrently submit events against quota total `1`; expect no overspend and no negative or drifting `frozen_value`/`used_value`.

### 13. PostgreSQL Smoke

Use a real PostgreSQL database from `docker-compose.yml`.

Minimum smoke checks:

- Migrations apply successfully through repository initialization.
- OpenAPI duplicate and conflicting idempotency behavior persists across PostgreSQL.
- Quota insufficient path rolls back participation, grant order, outbox, reservation, and quota aggregate.
- Reward Callback `SUCCESS` confirms frozen quota exactly once.
- Reward Callback `FAILED` releases frozen quota exactly once.
- Cross-tenant callback locator cannot update an order from another tenant.
- Concurrent duplicate event/callback behavior has no duplicate rows and no quota drift.
- `cmd/engine-e2e-postgres` completes with a unique run ID and leaves reconciled grant/quota state.

### 14. Redaction And Metrics

- For each failed auth/signature path, inspect response body, audit payload, logs if captured, and `/metrics`.
- Assert none contains full token, full Authorization header, cookie, secret, password, raw body, full targetRef, plaintext provider credentials, or high-cardinality metric labels such as eventId, bizId, subjectId, activityId, billNo, commandId, or idempotencyKey.
- Assert allowed metric labels remain low-cardinality: tenant, source, event type, result, reason, quota type, action, callback status, and send status.

## Required Commands

The current Test Plan Agent must not run these commands. The follow-up Test Agent should run them after implementation.

```bash
go test ./...
```

```bash
go test -race ./internal/httpapi ./internal/marketing ./internal/postgres
```

```bash
docker compose up -d postgres
docker compose exec -T postgres pg_isready -U exmarketing -d exmarketing
export EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable'
go test ./internal/postgres
go run ./cmd/engine-e2e-postgres
```

```bash
go test ./internal/httpapi -run 'Test.*(OpenAPI|Webhook|Callback|Admin|Idempotency|Quota|Auth|Signature|Tenant|Concurrent)'
```

If PostgreSQL smoke uses a dirty shared local database, the Test Agent must use unique run prefixes and clean only those prefixed rows, or recreate a disposable database before the smoke.

## Evidence Required From Test Agent

The Test Agent should record:

- Exact command output summary and pass/fail status.
- Names of added or executed test cases.
- For each security failure class, the observed HTTP status, `errorCode`, and no-mutation evidence.
- PostgreSQL smoke run ID and a short DB state summary proving quota and callback state are reconciled.
- Any uncovered item, especially non-Webhook audit persistence failure behavior, with reason and follow-up owner.

## Decision

```text
PASS
```

需求：2026-05-29-security-auth-signature-hardening
