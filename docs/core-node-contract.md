# Core Node Contract

This document freezes the production contract for the first ExMarketing workflow
node set. New node types should not be added until these contracts are aligned
across schema, handler behavior, validation, templates, and run history.

## Scope

The production main chain is intentionally small:

```text
event-in -> inbox -> normalize -> match -> fact-query -> qualify -> reward -> limit -> quota -> participation -> grant-order -> outbox
```

`audit` is a mandatory sidecar/terminal node for blocked, risk, skipped, and
successful terminal paths. It is not counted as a business main-chain node.

`condition`, `switch`, `callback`, `retry`, `dlq`, `reconcile`, `alert`,
`http-call`, `mq-publish`, `webhook`, and `decimal` remain advanced or
operational nodes. They must not be required by the default production chain.

Node schema categories are:

- `CORE`: default production-chain nodes.
- `ADVANCED`: external dispatch/callback integration nodes such as `http-call`,
  `mq-publish`, `webhook`, and `callback`.
- `OPS`: operational observability/recovery nodes such as `retry`, `dlq`,
  `audit`, `reconcile`, and `alert`.

The node library should group by this category and keep `ADVANCED` / `OPS`
outside the default quick-start path.

## Runtime Rules

- Core handlers must prefer resolved node input and upstream node output over
  global runtime fields. Runtime fields are allowed as compatibility fallback.
- Every core handler must write a stable top-level output key matching the node
  schema.
- Every core handler must include `audit` metadata in output.
- Every failed or skipped result must include `errorCode` or `skipReason`.
- Branch nodes must expose route-friendly keys at the top level.
- Run history must be able to explain: input, decision reason, and downstream
  output.
- `participation`, `grant-order`, `outbox`, and actual quota reservation commit
  must share one transaction authority:
  `CreateParticipationGrantOutboxesWithQuota`. Workflow nodes may explain and
  expose committed runtime data, but must not independently double-write these
  records.

## Terminal Semantics

- `grant-order` creates the reward command and owns the grant state machine, but
  `CREATED`, `PENDING`, `SENDING`, or `SENT` must not be presented as final
  user reward success.
- `outbox` is a delivery boundary. `SENT` means the sender adapter accepted or
  delivered the command to HTTP / MQ / Webhook transport; it does not mean the
  downstream reward provider has completed the user-visible reward.
- `callback` is an advanced integration node and an OpenAPI endpoint concern,
  not part of the default workflow main chain. Provider callbacks or
  reconciliation are the terminal truth for final grant success/failure and
  quota confirmation/release.
- `audit` is the workflow terminal node for explainability. Successful outbox,
  blocked qualification, risk routes, skipped branches, and failed terminal
  paths must all be explainable from audit/run-history evidence.
- Advanced/OPS workflow handlers must declare whether they are `EXPLAIN_ONLY`,
  `EXTERNAL_ENDPOINT`, or real execution handlers. They must not imply that a
  workflow node synchronously delivered HTTP/MQ/Webhook, consumed callback, ran
  reconciliation, or fired alerts when that authority belongs to outbox workers,
  callback endpoints, reconciliation jobs, metrics, or Admin APIs.
- PostgreSQL and in-memory behavior must be checked with the same Event In to
  Outbox contract: unit tests run against memory by default; PostgreSQL smoke
  uses `EX_MARKETING_DATABASE_URL go run ./cmd/engine-e2e-postgres` and must
  prove event inbox, participation, grant order, outbox, callback, and
  reconciliation state consistency.

## Core Contracts

| Node | Required Input | Required Config | Stable Output | Failure / Route |
| --- | --- | --- | --- | --- |
| `event-in` | external standard event | `eventTypes`, `sourceSystems`, `tenantScope` | `raw_event`, `eventId`, `eventType`, `tenantId` | fail with `EVENT_IN_*` |
| `inbox` | `raw_event` | `idempotencyKeyTemplate`, `duplicatePolicy`, `ttlSeconds` | `event_context` | duplicate policy decides return/reject/reprocess |
| `normalize` | `raw_event` | `mapping`, `defaults` | `normalized_event` | fail only on invalid mapping |
| `match` | `event_context`, optional `activity_config` | `mode`, `tenantMatchPolicy`, `fallbackAction` | `matched_activity` | fail/skip according to fallback action |
| `fact-query` | `subject`, `fact_keys` | `factKeys`, `source`, `timeoutSeconds`, `failureAction` | `facts` | block/use-empty/skip according to failure action |
| `qualify` | `event_context`, `matched_activity`, optional `facts` | `ruleCodes`, `evaluationMode`, `riskPolicy`, `blockAction` | `stage_result`, `PASS`, `BLOCK`, `RISK`, `decision` | route by `PASS/BLOCK/RISK` |
| `reward` | `qualified_context`, optional `reward_config` | `rewardCode`, `rewardType`, `calculateType`, `amount`, `currency` | `reward_items` | skip when not qualified; fail on invalid reward config |
| `limit` | `subject`, `activity_id` | `scope`, `scopeKeyTemplate`, `period`, `maxCount`, `consumeMode` | `limit_hold` | route/fail by `exceedAction` |
| `quota` | `reward_items`, optional `limit_hold` | `quotaCode`, `quotaType`, `scope`, `consumeMode`, `amountField` | `quota_hold` | route/fail by `failAction` |
| `participation` | `qualified_context`, optional `quota_hold` | `status`, `deduplicate` | `participation_id`, `commitMode`, `commitAuthority` | fail if participation snapshot cannot be created |
| `grant-order` | `participation_id`, `reward_items` | `provider`, `expireSeconds` | `grant_order`, `grant_orders`, `commitMode`, `commitAuthority` | skip if no reward; fail on state-machine violation |
| `outbox` | `grant_order` | `channel`, `maxAttempts` | `HTTP`, `MQ`, `Webhook`, `Failed`, `commitMode`, `commitAuthority` | failed dispatch must remain replayable |
| `audit` | `stage_result` | `action`, `severity`, `targetType`, `includePayload`, `retentionPolicy` | `audit_log` | audit write failure must be visible |

## Template Requirements

Built-in workflow templates must:

- use the core chain above as their default production path;
- include explicit `qualify` routes for `PASS`, `BLOCK`, and `RISK`;
- route `PASS` to `reward`;
- route `BLOCK` and `RISK` to `audit`;
- mark exactly one branch route as default;
- end the successful path at `audit` after `outbox`;
- pass `ValidateWorkflowDefinition` without manual edits.

## Run History Requirements

For each executed node, the persisted node execution must include:

- `input`: resolved inputs and input mapping;
- `output`: stable output key, `audit`, route key or decision where applicable;
- `errorMessage`: populated for failed and skipped terminal reasons;
- timing fields: `startedAt`, `finishedAt`, and duration derivable by clients.
