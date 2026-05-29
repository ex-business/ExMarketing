# Implementation

## Summary

已按 `task.md` / `analysis.md` 的最小验收口径补齐 ActivityPlugin 生命周期与版本契约：

- 插件注册从单一 `pluginCode` 扩展为 `pluginCode + version`，版本必须为 `MAJOR.MINOR.PATCH`。
- 新增插件元数据契约：`capabilities`、`supportedActivityTypes`、`configSchema`、`stateSchema`、`disabled`、`disableStrategies`。
- 注册表支持版本范围解析，默认选择最高兼容版本，并拒绝重复 `pluginCode + version`。
- 活动创建和上线前校验接入插件禁用、活动类型、能力缺失和配置错误检查。
- 运行时插件快照记录 `pluginVersion`、`lifecycleStep`、`status`、`reasonCode`、`reason`、`rollbackToken`。
- 插件执行失败会拒绝当前活动，不产生 grant_order / outbox，并写 `PLUGIN_EXECUTION_FAILED` 审计。
- 新增生命周期契约文档 `docs/activityplugin-lifecycle.md`。

`test.md` 和 `commit.md` 实际文件只有模板占位，未提供真实 FAIL / BLOCK 原因；本轮未发现可按失败上下文定位的既有失败，只按需求做最小实现补齐。

## Changed Files

- `internal/plugin/plugin.go`
- `internal/plugin/plugin_test.go`
- `internal/marketing/model.go`
- `internal/marketing/prelaunch_validation.go`
- `internal/marketing/prelaunch_validation_test.go`
- `internal/engine/service.go`
- `internal/engine/plugin_test.go`
- `docs/activityplugin-lifecycle.md`
- `tasks/2026-05-29-activityplugin-lifecycle/test-plan.md`
- `tasks/2026-05-29-activityplugin-lifecycle/implementation.md`

## Behavior Changes

- 插件注册需要可解析语义化版本；旧插件未显式实现元数据时默认注册为 `1.0.0`，并保留原有上下文构建/配置校验能力。
- `PLUGIN_NOT_REGISTERED` / `PLUGIN_INCOMPATIBLE_ACTIVITY_TYPE` 口径更新为任务要求的 `PLUGIN_NOT_FOUND` / `PLUGIN_ACTIVITY_TYPE_UNSUPPORTED`。
- 禁用插件无法用于新活动创建或上线前校验；运行期禁用插件会 BLOCK 当前活动处理。
- 缺少 `CONTEXT_BUILD` 等必需能力的插件会在活动创建或 prelaunch 阶段 BLOCK。
- 插件 `BuildContext` 或 `AfterQualified` 执行失败时，当前活动被拒绝，不会进入 grant_order / outbox 副作用链路，并记录结构化审计。

## Verification

~~~bash
go test ./...
go build ./...
~~~

结果：均通过。

需求：2026-05-29-activityplugin-lifecycle
