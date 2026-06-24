# Core Node Contract

This document freezes the production contract for the first ExMarketing workflow
node set. New node types should not be added until these contracts are aligned
across schema, handler behavior, validation, templates, and run history.

## Scope

The production main chain is intentionally explicit:

```text
event-in -> inbox -> normalize -> match -> fact-query -> activity-state -> quota-check -> qualify -> candidate-reward -> budget-reserve -> participation -> grant-order -> outbox
```

Activities that need multi-source facts can insert compose/normalize nodes
before `qualify`:

```text
event-in -> inbox -> normalize -> match -> fact-query -> facts-compose -> facts-normalize -> activity-state -> quota-check -> qualify -> candidate-reward -> budget-reserve -> participation -> grant-order -> outbox
```

`audit` is a mandatory sidecar/terminal node for blocked, risk, skipped, and
successful terminal paths. It is not counted as a business main-chain node.

`condition`, `switch`, `code`, `combine`, `split`, `callback`, `retry`, `dlq`,
`reconcile`, `alert`, `http-call`, `mq-publish`, `webhook`, `decimal`,
`approval-gate`, `compensation`, `random-pick`, `plugin-execute`, `ab-bucket`,
`risk-score`, `settlement-window`, and `manual-adjust` remain optional,
advanced, composition, or boundary nodes. They must not be required by the
default production chain.

Node schema categories are:

- `基础节点`: default production-chain nodes, including `event-in`, `inbox`,
  `normalize`, `match`, `fact-query`, `activity-state`, `quota-check`,
  `qualify`, `candidate-reward`, `budget-reserve`, `participation`,
  `grant-order`, `outbox`, and `audit`.
- `高级节点`: optional enhancement nodes such as `user-level-check`,
  `facts-compose`, `facts-normalize`, `risk-score`, `ab-bucket`,
  `random-pick`, `plugin-execute`, `approval-gate`, `settlement-window`,
  `manual-adjust`, `compensation`, `reward-reconcile`, and `reconcile`.
- `组合节点`: controlled branch, expression, script, and data-shaping nodes
  such as `condition`, `switch`, `code`, `lua-query`, `combine`, `split`, and
  `decimal`.
- `其他节点`: trigger/adaptor, low-level query, delivery-boundary, and
  operational helper nodes such as `http-event`, `webhook-event`, `mq-event`,
  `schedule-event`, `state-event`, `manual-event`, `state-store`,
  `context-query`, `http-query`, `redis-query`, `http-call`, `mq-publish`,
  `webhook`, `callback`, `retry`, `dlq`, and `alert`.

The node library should group by this product-facing category and keep
`高级节点`, `组合节点`, and `其他节点` outside the default quick-start path.

## Runtime Rules

- Core handlers must prefer resolved node input and upstream node output over
  global runtime fields. Runtime fields are allowed as compatibility fallback.
- Core handlers must not expose raw infrastructure connection fields in new
  node configs. HTTP URL, Redis address, DB DSN, MQ broker, topic credentials,
  and secrets belong to Service Provider or Resource definitions.
- Nodes that need external facts or business capabilities must use
  `ProviderChainStep(targetType=SERVICE_INTERFACE)` and call a registered
  Service Interface. Direct `resourceRef` is reserved for controlled state,
  counter, lock, cache, store, and other low-level Resource operations.
- Legacy `resourceRef`, `providerCode`, `endpoint`, `redisRef`, and
  `source=PROVIDER/FACT_SERVICE` fields may be read for compatibility, but new
  schemas and templates must not make them the default configuration path.
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
- Node config schema must only expose handler-supported semantics. `condition`
  supports `RULE_EXPR` and `JSON_LOGIC`; `CEL` must not be offered until a real
  CEL evaluator exists. `code` supports `LUA` and `EXPRESSION`; JavaScript and
  Python must fail fast until real runtimes are wired. `decimal` currently
  parses `amount_expr` / `amountExpr` / `amount` and emits the fixed 4-decimal
  Decimal output; `scale` and `roundingMode` must not be advertised until the
  handler implements them.
- PostgreSQL and in-memory behavior must be checked with the same Event In to
  Outbox contract: unit tests run against memory by default; PostgreSQL smoke
  uses `EX_MARKETING_DATABASE_URL go run ./cmd/engine-e2e-postgres` and must
  prove event inbox, participation, grant order, outbox, callback, and
  reconciliation state consistency.

## Core Contracts

| Node | Required Input | Required Config | Stable Output | Failure / Route |
| --- | --- | --- | --- | --- |
| `event-in` | committed standard event | `eventTypes`, `sourceSystems`, `tenantScope` | `raw_event`, `eventId`, `eventType`, `tenantId` | fail with `EVENT_IN_*` |
| `inbox` | `raw_event` | `idempotencyKeyTemplate`, `duplicatePolicy`, `ttlSeconds` | `event_context` | duplicate policy decides return/reject/reprocess |
| `normalize` | `raw_event` | `mapping`, `defaults` | `normalized_event` | fail only on invalid mapping |
| `match` | `event_context`, optional `activity_config` | `mode`, `tenantMatchPolicy`, `fallbackAction` | `matched_activity` | fail/skip according to fallback action |
| `fact-query` | `subject`, `fact_keys` | `providerChain[]` with `targetType=SERVICE_INTERFACE` by default; `factKeys`, `subjectPath`, `inputMapping`, `outputAs`, `timeoutMs`, `onMiss`; `RESOURCE/PROVIDER/FACT_SERVICE` only for controlled store or compatibility | `facts` | block/use-empty/skip according to failure action; see [Fact Query node guide](fact-query-node.zh-CN.md) |
| `activity-state` | `event_context`, optional `state_patch` | `providerChain[]` with `targetType=RESOURCE` or internal state adapter, `resourceRef`, `operation`, `keyTemplate`, `namespace`, `valuePath`, `outputAs`; no endpoint/redisRef | `activity_state`, `state_version` | durable state authority remains ActivityPlugin/StateStore |
| `quota-check` | `subject`, `activity_id` | `providerChain[]` with `targetType=RESOURCE`, `resourceRef`, `operation=COUNTER_CHECK/COUNTER_CONSUME`, `keyTemplate`, `namespace`, `scope`, `scopeKeyTemplate`, `window`, `maxCount`, `consumeMode` | `quota_check`, `limit_hold` | frequency/limit semantics; emits `limit_hold` as a stable downstream field |
| `qualify` | `event_context`, `matched_activity`, optional `facts` | `ruleCodes`, `evaluationMode`, `riskPolicy`, `blockAction` | `stage_result`, `PASS`, `BLOCK`, `RISK`, `decision` | route by `PASS/BLOCK/RISK` |
| `candidate-reward` | `qualified_context`, optional `plugin_result` | `providerChain[]` with `targetType=RESOURCE` or `SERVICE_INTERFACE`, `resourceRef` or `serviceRef/interfaceRef`, `keyTemplate`, `namespace`, `candidatePolicy`, `notQualifiedAction`, `emptyAction` | `candidate_rewards`, `reward_items` | apply candidate policy; skip/block when not qualified or empty |
| `budget-reserve` | `candidate_rewards`, optional `reward_items` | `providerChain[]` with `targetType=SERVICE_INTERFACE`, `serviceRef/interfaceRef`, `providerPolicy`, `idempotencyKeyTemplate`, `failAction`, `releaseOnFailure`; legacy `reserveProviderRef` only as compatibility alias | `budget_reservation`, `quota_hold` | budget/inventory reservation semantics; internal quota fields live in the selected service provider |
| `participation` | `qualified_context`, optional `quota_hold` | `status`, `deduplicate` | `participation_id`, `commitMode`, `commitAuthority` | fail if participation snapshot cannot be created |
| `grant-order` | `participation_id`, `reward_items` | `provider`, `expireSeconds` | `grant_order`, `grant_orders`, `commitMode`, `commitAuthority` | skip if no reward; fail on state-machine violation |
| `outbox` | `grant_order` | `channel`, `maxAttempts`; optional `targetType=SERVICE_INTERFACE` for controlled publish interface or `resourceRef` for existing outbox target compatibility | `HTTP`, `MQ`, `Webhook`, `Failed`, `commitMode`, `commitAuthority` | failed dispatch must remain replayable |
| `audit` | optional routed payload | `action`, `severity`, `targetType`, `includePayload`, `retentionPolicy` | `audit_log` | can terminate multiple PASS/BLOCK/RISK branches without a fixed upstream input |

## Template Requirements

Built-in workflow templates must:

- use the core chain above as their default production path;
- include explicit `qualify` routes for `PASS`, `BLOCK`, and `RISK`;
- route `PASS` to `candidate-reward`;
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
