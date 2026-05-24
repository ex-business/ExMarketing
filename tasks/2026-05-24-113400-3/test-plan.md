# Test Plan

## Preconditions

- `tasks/2026-05-24-113400-3/review.md` Decision is `PASS`.
- Test Agent must run this plan only after Dev Agent has implemented the four-stage rule execution feature.
- Test Agent may adapt exact package/class names to the final implementation, but must preserve the behavioral coverage below.
- This plan intentionally does not execute tests; it only defines cases and command checklist for the later Test Agent.

## Scope

Validate the rule engine four-stage execution model:

```text
MATCH -> QUALIFY -> REWARD -> RISK/LIMIT/BUDGET
```

The test scope must cover:

- Normal successful path.
- Insufficient quota/budget/stock/user limit.
- Idempotent duplicate event handling.
- Concurrent event handling and final atomic quota deduction.
- PostgreSQL smoke verification.
- Rule explanations at activity, stage, and rule levels.
- Field reads from `attributes.*`, `state.*`, and optional `facts.*`.
- Formal processing behavior vs simulation behavior.

## Test Strategy

Use layered tests:

- Unit tests for rule evaluator behavior, field path resolution, stage ordering, skipped rules, and error reason codes.
- Integration tests for formal event processing, transaction boundaries, quota deduction, grant order creation, outbox creation, and rejection audit/participation traces.
- Simulation tests to verify full explain output and no business table writes.
- Concurrency tests against PostgreSQL-backed quota rows to prove precheck cannot over-grant under contention.
- PostgreSQL smoke tests to verify schema, persistence, transaction rollback, and audit visibility.

Formal processing口径:

- Stop at the first failed stage for a campaign.
- Persist a traceable final activity reason and a compact rule/stage summary where the implementation supports audit or participation rejection details.
- Do not create grant order, outbox, or quota deduction when any blocking rule fails.
- If all rule stages pass, still rely on the existing activity-level transaction for quota deduction, grant order, and outbox.

Simulation口径:

- Return matched and rejected/skipped campaign results with stage and rule details.
- Include disabled rules as `SKIPPED`.
- Prefer collecting all rules within an executed stage.
- It is acceptable to stop later stages after a blocking stage if final implementation documents that behavior, but the failed stage and all executed rule reasons must be stable and explicit.
- Never write inbox, participation, grant order, outbox, or quota state.

## Test Data

Prepare deterministic fixtures:

- Tenant: `tenant-rule-stage`.
- Subject: `USER:user-1001`.
- Event type: `DEPOSIT_SUCCESS`.
- Base event attributes:

```json
{
  "amount": "100.00",
  "currency": "USDT",
  "isFirstDeposit": true,
  "channel": "app",
  "riskLevel": "LOW"
}
```

- Facts fixture:

```json
{
  "USER_PROFILE": {
    "vipLevel": 3,
    "registeredDays": 30,
    "tag": "normal"
  }
}
```

- Campaign A:
  - Active status.
  - Trigger event `DEPOSIT_SUCCESS`.
  - Current time inside campaign window.
  - Priority higher than Campaign B.
  - `exclusiveGroup = "deposit-bonus"`.
  - `stackable = false`.
  - Enabled reward.
  - Quota rows for `BUDGET`, `STOCK`, and `USER_LIMIT`.

- Campaign B:
  - Same trigger event and exclusive group.
  - Lower priority.
  - Used for priority/exclusive rejection tests.

Rules must include stable `sort` values and at least one `enabled=false` rule.

## Test Cases

### TC01 Normal Path: All Four Stages Pass

Steps:

1. Configure Campaign A with passing `MATCH`, `QUALIFY`, `REWARD`, and `LIMIT/RISK` rules.
2. Submit one formal event with `amount = 100.00`, `isFirstDeposit = true`, sufficient facts, sufficient budget, sufficient stock, and available user limit.
3. Verify response/result decision is `PASSED` or equivalent rewardable success.
4. Verify stage order is exactly `MATCH`, `QUALIFY`, `REWARD`, `RISK_LIMIT_BUDGET`.
5. Verify every executed rule contains `ruleCode`, `rulePhase`, `ruleType`, `status`, `passed`, `reasonCode`, and `reason`.
6. Verify participation is `REWARDABLE` or equivalent success state.
7. Verify quota is deducted once.
8. Verify grant order is created once.
9. Verify outbox reward command is created once.

Expected:

- Final failed stage is empty.
- Final reason indicates success.
- No duplicate rule output.
- Stage and rule order is stable across repeated reads.

### TC02 MATCH Rejection

Steps:

1. Use an event with wrong `eventType` or wrong `tenantId`.
2. Submit formal event.
3. Verify failure occurs at `MATCH`.
4. Verify reason code indicates trigger/tenant mismatch.
5. Verify later formal stages are not required to run.
6. Verify no quota deduction, grant order, or outbox is created.

Expected:

- Activity decision is `REJECTED` or equivalent non-rewardable result.
- Rejection/audit trace can identify `MATCH` as failed stage.

### TC03 QUALIFY Rejection From Facts

Steps:

1. Configure a `QUALIFY` rule reading `facts.USER_PROFILE.vipLevel >= 3`.
2. Run with `vipLevel = 1`, then run with missing `USER_PROFILE`, then run with provider timeout/unavailable.
3. Verify each run fails in `QUALIFY`.
4. Verify reason codes distinguish value mismatch, missing fact field, and fact unavailable/timeout.

Expected:

- No facts failure silently passes.
- Sensitive provider details, URLs, tokens, or authorization headers are not present in `reason` or `detail`.

### TC04 REWARD Threshold Rejection

Steps:

1. Configure `REWARD` rule `attributes.amount >= 100.00`.
2. Submit event with `amount = 99.99`.
3. Submit event with missing `attributes.amount`.
4. Submit event with non-decimal `attributes.amount = "abc"`.
5. Verify failures occur at `REWARD` with distinct reason codes for threshold not met, field missing, and decimal parse/type error.

Expected:

- Numeric comparison uses decimal semantics, not string order.
- No quota deduction, grant order, or outbox for rejected events.

### TC05 RISK/LIMIT/BUDGET Rejection: User Limit

Steps:

1. Configure user limit remaining as `0` or pre-create previous participation count at limit.
2. Submit otherwise valid formal event.
3. Verify stages before limit pass.
4. Verify final failure stage is `RISK_LIMIT_BUDGET`.
5. Verify reason code identifies user limit exceeded.

Expected:

- No grant order or outbox.
- No dirty `REWARDABLE` participation remains.

### TC06 RISK/LIMIT/BUDGET Rejection: Budget Insufficient

Steps:

1. Configure reward amount greater than remaining `BUDGET`.
2. Submit otherwise valid formal event.
3. Verify precheck explain reports insufficient budget.
4. Verify final transaction does not create grant order/outbox and does not leave quota negative.
5. Verify audit or rejection trace includes `QUOTA_EXCEEDED` or the implementation's canonical budget insufficient reason code.

Expected:

- This explicitly covers the required insufficient quota path.
- Budget remains unchanged or rolls back to the original value.

### TC07 RISK/LIMIT/BUDGET Rejection: Stock Insufficient

Steps:

1. Configure enabled reward stock remaining as `0`.
2. Submit otherwise valid formal event.
3. Verify final failure stage is `RISK_LIMIT_BUDGET`.
4. Verify reason identifies stock insufficient.

Expected:

- No grant order/outbox.
- Stock remains `0`, never negative.

### TC08 Idempotent Duplicate Event

Steps:

1. Submit a valid formal event with a deterministic idempotency key/event id.
2. Submit the exact same event again with the same idempotency key/event id.
3. Verify the second submission is detected as duplicate/idempotent replay.
4. Verify no second participation, quota deduction, grant order, or outbox is created.
5. Verify duplicate response is stable and does not rerun external facts in a way that changes persisted result.

Expected:

- Exactly one rewardable business outcome exists.
- Idempotency conflict with mismatched payload, if supported, returns an explicit conflict reason and does not award.

### TC09 Concurrent Valid Events Competing For Limited Budget

Steps:

1. Configure budget sufficient for exactly one reward.
2. Submit two or more valid events concurrently for different event ids and eligible subjects, or same campaign with separate idempotency keys.
3. Use a barrier/latch in the test harness so requests enter rule precheck close together.
4. Verify at most one event commits quota deduction and reward artifacts.
5. Verify all losing events are rejected or rolled back with quota exceeded reason.
6. Verify no negative budget, duplicate outbox, or dirty rewardable participation.

Expected:

- Final PostgreSQL quota state is consistent.
- Precheck passing concurrently does not bypass atomic quota deduction.

### TC10 Concurrent Duplicate Event

Steps:

1. Submit the same valid idempotency key/event id concurrently from multiple workers.
2. Verify exactly one processing path wins.
3. Verify all other workers return duplicate/replay/conflict according to implementation semantics.
4. Verify only one quota deduction, one grant order, and one outbox row.

Expected:

- Idempotency lock/unique constraint prevents duplicate award under race.

### TC11 Disabled Rule Is Skipped

Steps:

1. Add a failing rule with `enabled=false`.
2. Run simulation and formal processing with otherwise valid event.
3. Verify the disabled rule appears in explain details as `SKIPPED` with `RULE_DISABLED`.
4. Verify the disabled rule does not block the activity.

Expected:

- Activity still passes if all enabled rules pass.

### TC12 Unknown Phase, BUDGET Phase, Unknown Evaluator

Steps:

1. Configure rule with unknown `rulePhase`.
2. Configure rule with `rulePhase = BUDGET`.
3. Configure rule with known phase but unknown/unregistered `ruleType`.
4. Run config validation if available, otherwise run execution path.
5. Verify all three are rejected with explicit configuration/evaluator reason codes.

Expected:

- No invalid rule silently passes.
- `BUDGET` is not accepted as a standalone `rule_phase`.

### TC13 State Namespace Reads

Steps:

1. Configure rules reading:
   - `state.activity.status`
   - `state.subject.participationCount`
   - `state.quota.BUDGET.remaining`
   - `state.quota.STOCK.remaining`
   - `state.exclusive.alreadyMatched`
2. Run passing and failing variants for each field.
3. Verify actual/expected values are present where safe and useful.

Expected:

- `state.*` fields are resolved consistently.
- Missing state field has an explicit missing-field reason.

### TC14 Multi-Campaign Priority, Exclusive Group, Stackable

Steps:

1. Configure Campaign A and Campaign B as both matching the same event.
2. Set same `exclusiveGroup`, Campaign A higher priority, `stackable=false`.
3. Submit simulation and formal event.
4. Verify Campaign A is retained and Campaign B is rejected/skipped with an exclusive/priority reason.
5. Repeat with `stackable=true` where business rules allow both campaigns.

Expected:

- Each excluded campaign has one clear final reason.
- Ordering is stable when priority ties exist, or the implementation reports a deterministic tie-break rule.

### TC15 Simulation Does Not Write Business Tables

Steps:

1. Snapshot counts and quota balances for inbox, participation, grant order, outbox, and quota tables.
2. Run simulation with valid event.
3. Verify output contains matched/rejected campaigns, stage details, rule details, estimated rewards, estimated outbox, and estimated quota impact.
4. Re-read counts and quota balances.

Expected:

- No inbox, participation, grant order, outbox, or quota mutation.
- Explain output remains complete enough for audit/debugging.

### TC16 Formal Rejection Trace Is Persisted

Steps:

1. Submit a formal event that fails in each stage: `MATCH`, `QUALIFY`, `REWARD`, and `RISK_LIMIT_BUDGET`.
2. For each failure, inspect participation rejection/audit tables or implementation-specific trace storage.
3. Verify final reason, failed stage, and rule summary are traceable.

Expected:

- Test Agent can prove rejected formal events are explainable after request completion.

### TC17 Rule Ordering And Stable Sort

Steps:

1. Configure multiple rules in the same stage with sort values `10`, `20`, and duplicate `20`.
2. Run simulation multiple times.
3. Verify order is stable and follows sort ascending plus deterministic tie-breaker.
4. Verify stage counters for executed, skipped, and failed rules are correct.

Expected:

- Rule result ordering does not drift between runs.

### TC18 Sensitive Data Redaction

Steps:

1. Simulate fact provider error containing token-like text, authorization header, internal URL, and raw sensitive payload.
2. Trigger rule failure depending on that fact.
3. Inspect activity/stage/rule `reason` and `detail`.

Expected:

- No token, `Authorization`, internal provider URL, private IP, or raw sensitive response is emitted.
- A safe reason code such as `FACT_UNAVAILABLE` or `FACT_TIMEOUT` is emitted.

## PostgreSQL Smoke

The later Test Agent must run a PostgreSQL-backed smoke, not only in-memory tests.

Smoke checklist:

1. Start or connect to the project PostgreSQL test database using the repository's standard local profile.
2. Apply the existing schema/migrations through the normal test setup path.
3. Insert campaign, rule, reward, and quota fixtures.
4. Run one successful formal event.
5. Query PostgreSQL to verify:
   - One rewardable participation or equivalent success row.
   - Quota deducted exactly once.
   - One grant order row.
   - One outbox reward command row.
6. Run one budget-insufficient formal event.
7. Query PostgreSQL to verify:
   - No dirty rewardable participation for the failed event.
   - No grant order row.
   - No outbox row.
   - Budget unchanged and non-negative.
   - Rejection/audit row contains quota exceeded reason.
8. Run one duplicate event.
9. Query PostgreSQL to verify:
   - No second quota deduction.
   - No duplicate grant order/outbox.
10. Run the concurrent limited-budget case and verify final quota consistency from PostgreSQL.

## Required Commands

```bash
go test ./...
go build ./...
```

Optional focused commands for diagnosis:

```bash
go test ./internal/rule -run 'TestExecute|TestRegistry|TestCondition'
go test ./internal/httpapi -run 'TestActivityEventRuleRejection|TestSimulateEvent'
```

PostgreSQL smoke can use the repository's existing integration test package when the local database profile is available:

```bash
go test ./internal/postgres
```

The Test Agent must record the actual commands executed, exit status, and failed test names if any.

## Block Criteria

Return `BLOCK` if any of these are true during later validation:

- Review decision is not `PASS`.
- The implementation has no way to observe activity/stage/rule explanations.
- Normal path cannot create the expected participation/quota/grant/outbox artifacts.
- Quota insufficient path leaves dirty rewardable participation, grant order, outbox, or negative quota.
- Duplicate/idempotent events can award twice.
- Concurrent limited-budget events can over-deduct or over-award.
- PostgreSQL smoke cannot run due to missing schema/test database setup and no documented alternative exists.
- Sensitive fact/provider details leak into explain output.

## Decision

```text
PASS
```

需求：2026-05-24-113400-3
