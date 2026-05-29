# Test Plan

## Test Strategy

围绕 ActivityPlugin 生命周期契约做单元测试和全量回归：

- 注册表层验证 `pluginCode + version` 唯一、语义化版本、版本范围解析、能力声明、Schema 元数据和禁用过滤。
- 活动创建 / 上线前校验验证插件不存在、活动类型不兼容、能力缺失、插件禁用和配置校验失败均会 BLOCK。
- 运行时验证插件上下文只进入 `state.plugin.<pluginCode>.*` 和 participation snapshot，不直接写 grant_order / outbox / quota。
- 执行失败验证插件错误结构化落到 snapshot / audit，且当前活动不产生 grant_order / outbox 脏写。
- 保留现有 FIRST_DEPOSIT 插件兼容性，确保旧插件默认 `1.0.0` 元数据不破坏已有流程。

## Test Cases

1. `TestRegistryRejectsDuplicatePluginCode`
   - 同一 `pluginCode + version` 重复注册应失败。

2. `TestRegistryResolvesHighestCompatibleVersionAndCapabilities`
   - 注册 `1.0.0`、`1.2.0`、`2.0.0` 后，`^1.0.0` 应解析到 `1.2.0`。
   - 校验注册元数据包含 capabilities、configSchema、stateSchema。

3. `TestRegistryRejectsMissingCapabilityAndDisabledPlugin`
   - 缺少 required capability 时解析失败。
   - disabled 插件在默认解析路径中不可用。

4. `TestValidatePrelaunchBlocksUnknownPluginCode`
   - 未注册插件上线前校验返回 `PLUGIN_NOT_FOUND` 并 BLOCK。

5. `TestValidatePrelaunchBlocksIncompatiblePluginCode`
   - 插件不支持当前 `activityType` 时返回 `PLUGIN_ACTIVITY_TYPE_UNSUPPORTED` 并 BLOCK。

6. `TestValidatePrelaunchAllowsRegisteredCompatiblePlugin`
   - 已注册且支持当前活动类型的插件通过插件校验。

7. `TestActivityPluginExecutionFailureRejectsWithoutGrantOrOutboxAndAudits`
   - `BuildContext` 失败时 participation 为 rejected。
   - 不生成 grant_order / outbox。
   - snapshot 包含 `pluginVersion` 和 `PLUGIN_EXECUTION_FAILED`。
   - audit 中存在 `PLUGIN_EXECUTION_FAILED` / `ACTIVITY_PLUGIN`。

8. 既有插件回归
   - `TestActivityPluginContextFeedsRulesRewardsAndSnapshot`
   - `TestActivityPluginUnknownCodeRejectedAtCreateAndRuntime`
   - `TestSimulateEventIncludesPluginPreviewWithoutPersistence`
   - `TestFirstDepositPluginIsRegisteredByDefaultAndFeedsRulesRewards`
   - `TestFirstDepositPluginPrecheckRejectsBeforeGrant`
   - `TestFirstDepositPluginRejectsIncompatibleActivityTypeAtCreate`

## Required Commands

~~~bash
go test ./...
go build ./...
~~~

## Data / Smoke

使用现有内存 repository 和测试插件数据即可。无需外部数据库、Kafka 或 HTTP 服务。

## Decision

~~~text
PASS
~~~

需求：2026-05-29-activityplugin-lifecycle
