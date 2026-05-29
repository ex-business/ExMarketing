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

Plugin execution failures are audited as `PLUGIN_EXECUTION_FAILED` on resource
`ACTIVITY_PLUGIN` with tenant, activity, plugin, lifecycle, event, status, and
error details.
