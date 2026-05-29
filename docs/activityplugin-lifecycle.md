# ActivityPlugin Lifecycle Contract

ActivityPlugin is a controlled extension point for gameplay context. Plugins may
build context, validate configuration, return candidate results, and propose
state-change intents. They must not write participation, grant orders, outbox
messages, quotas, or call external reward systems directly.

## Registration

Each plugin registration is identified by `pluginCode + version`. Versions must
use `MAJOR.MINOR.PATCH`. A registration declares:

- `pluginCode`
- `version`
- `capabilities`
- `supportedActivityTypes`
- `configSchema`
- `stateSchema`
- `disabled`
- `disableStrategies`

The registry rejects duplicate `pluginCode + version` and resolves a version
range to the highest compatible version. Existing plugins without explicit
metadata default to `1.0.0` and the legacy context/validation capabilities.

## Capabilities

Supported capability names:

- `CONFIG_VALIDATE`
- `CONTEXT_BUILD`
- `BEFORE_MATCH`
- `BEFORE_REWARD`
- `AFTER_REWARD_CALCULATED`
- `EVENT_HANDLE`
- `ROLLBACK`
- `DISABLE_POLICY`

Activity creation and prelaunch validation block missing plugins, unsupported
activity types, disabled plugins, and missing required context capability.

Scheduler jobs reuse `EVENT_HANDLE`; no separate schedule or event-generation
capability is currently defined. A plugin schedule job can only be registered or
executed when the plugin is registered, enabled, compatible with the activity
type, declares `EVENT_HANDLE`, and is the plugin bound on the activity.

## Runtime

Runtime plugin execution is recorded in the participation snapshot with:

- `pluginCode`
- `pluginVersion`
- `lifecycleStep`
- `status`
- `reasonCode`
- `reason`
- `pluginContext`
- `precheckResult`
- `stateChangeIntents`
- `rollbackToken`

`BeforeMatch` builds `state.plugin.<pluginCode>.*` for rules and rewards.
`AfterRewardCalculated` maps plugin state-change intents back into the
participation snapshot after reward calculation and before grant/outbox writes.

If a plugin is missing, disabled, incompatible, or fails execution, the activity
is rejected for that event and the service does not create grant orders or
outbox messages for that activity.

## Context And State Boundary

`PluginContext` is a read-only event-processing snapshot built by the main
engine. It contains the standard event, activity configuration snapshot, rule
results, resolved facts, engine-provided `now`, and `traceId`. Plugins must not
mutate this snapshot or use process-global state as a replacement for it.

Plugin-owned process state must go through `PluginStateStore`. The minimum
operations are:

- `Get`
- `PutIfAbsent`
- `CompareAndSwap`
- `AppendEvent`
- `ListByRound`

Every state record is isolated by the full scope:

- `tenantId`
- `activityId`
- `activityVersion`
- `pluginCode`
- `subject`
- `biz`
- `stateKey`

Reads do not relax this scope. Writes with a missing tenant, activity, version,
plugin, subject, biz, or state key are rejected. Round queries must still include
tenant, activity, version, plugin, and biz so they cannot aggregate across
tenants or plugins.

All write operations require an idempotency key. The key should be the standard
event idempotency key, or a stable suffix derived from it for plugin-internal
commands. Duplicate writes for the same full scope, `pluginCode`, `stateKey`,
and idempotency key return a no-op result and do not consume state again.
Reusing the same idempotency key with a different operation or payload is an
idempotency conflict.

`PutIfAbsent` creates only when the state is absent. `CompareAndSwap` updates
only when the expected version matches and returns a version conflict otherwise;
failed version conflicts do not consume the idempotency key, so callers may
reload and retry intentionally. `AppendEvent` appends immutable state events and
deduplicates by idempotency key.

State audit records keep operation, result, scope, event id, idempotency key,
trace id, lifecycle step, and before/after summaries. Duplicate no-op requests
are observable from the returned result but do not create a second business-state
change audit record.

Plugin state is only gameplay process state, such as draw counts, round progress,
card flipping progress, or plugin event streams. It must not replace
`participation`, `grant_order`, `quota`, or `outbox`; final reward facts and
delivery still belong to the main engine.

The built-in `RED_ENVELOPE` and `LOTTERY` sample plugins follow this boundary:
they validate gameplay configuration, produce auditable candidate context, and
emit state-change intents only after the main engine qualifies the event. Sample
configuration templates are documented in
`docs/activityplugin-red-envelope-lottery.md`.

## Scheduler Events

`PluginScheduleJob` models gameplay timers such as `START`, `END`, `DRAW`,
`SETTLE`, and `COMPENSATE`. Jobs move through `PENDING`, `RUNNING`, `SUCCESS`,
`FAILED`, `RETRY`, and `CLOSED`; `SUCCESS` and `CLOSED` are terminal. Claiming a
job is atomic so only one executor can run a job at a time, and retryable
failures are moved to `RETRY` with `nextRetryAt`.

Schedule registration is idempotent on:

```text
tenantId + activityId + pluginCode + jobType + scheduledAt
```

The job snapshot stores the activity version and a hash of the activity config
at registration time. Standard event payloads generated later use the job
snapshot, not the current mutable activity config.

Plugin-generated schedule events are converted to standard `ActivityEvent`
payloads and written to `marketing_event_inbox` with source system
`activity_plugin`. The default standard event identity is stable:

- `eventId`: `plugin:<jobId>:<sequence>`
- `eventType`: mapped from the job type, for example `PLUGIN_DRAWN`
- `biz.bizType`: `PLUGIN_SCHEDULE_JOB`
- `biz.bizId`: `jobId` or `jobId:<sequence>` for later events
- `occurredAt`: `job.scheduledAt` unless the plugin supplies a stable business
  time
- `attributes`: job snapshot fields, including `activityId`, `activityVersion`,
  `activityConfigHash`, `pluginCode`, `jobId`, `jobType`, `scheduledAt`, and
  `sequence`

Repeated execution of the same job must produce the same Event Inbox key and
the same standard payload. A duplicate Inbox write with the same payload is
treated as schedule success; an Inbox idempotency conflict is a non-retryable
`IDEMPOTENCY_CONFLICT` failure requiring manual action. Scheduler code and
plugins must not create `grant_order`, `outbox`, quota reservations, or call
external reward systems directly.

## Error Codes

- `PLUGIN_NOT_FOUND`
- `PLUGIN_VERSION_UNRESOLVED`
- `PLUGIN_VERSION_INCOMPATIBLE`
- `PLUGIN_CAPABILITY_MISSING`
- `PLUGIN_ACTIVITY_TYPE_UNSUPPORTED`
- `PLUGIN_CONFIG_INVALID`
- `PLUGIN_STATE_SCHEMA_INVALID`
- `PLUGIN_DISABLED`
- `PLUGIN_EXECUTION_FAILED`
- `PLUGIN_ROLLBACK_FAILED`
- `EVENT_MAPPING_INVALID`
- `EVENT_INBOX_FAILED`
- `IDEMPOTENCY_CONFLICT`

Plugin execution failures are audited as `PLUGIN_EXECUTION_FAILED` on resource
`ACTIVITY_PLUGIN` with tenant, activity, plugin, lifecycle, event, status, and
error details.
