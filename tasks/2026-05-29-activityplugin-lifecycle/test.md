# Test

## Scope

根据 `task.md`、`analysis.md`、`review.md`、`test-plan.md`、`implementation.md` 执行 ActivityPlugin 生命周期与版本契约测试。

本轮只写入本文件，未修改业务代码、测试代码、文档代码、配置文件或其他文件。

## Test Cases

- 插件注册表：
  - `TestRegistryRejectsDuplicatePluginCode`
  - `TestRegistryResolvesHighestCompatibleVersionAndCapabilities`
  - `TestRegistryRejectsMissingCapabilityAndDisabledPlugin`
- 上线前校验：
  - `TestValidatePrelaunchBlocksUnknownPluginCode`
  - `TestValidatePrelaunchBlocksIncompatiblePluginCode`
  - `TestValidatePrelaunchAllowsRegisteredCompatiblePlugin`
- 运行期生命周期和副作用边界：
  - `TestActivityPluginExecutionFailureRejectsWithoutGrantOrOutboxAndAudits`
  - `TestActivityPluginContextFeedsRulesRewardsAndSnapshot`
  - `TestActivityPluginUnknownCodeRejectedAtCreateAndRuntime`
  - `TestSimulateEventIncludesPluginPreviewWithoutPersistence`
- 既有 FIRST_DEPOSIT 插件回归：
  - `TestFirstDepositPluginBuildsContextFromStandardEvent`
  - `TestFirstDepositPluginPrecheckRejectsInvalidInputs`
  - `TestFirstDepositPluginIsRegisteredByDefaultAndFeedsRulesRewards`
  - `TestFirstDepositPluginPrecheckRejectsBeforeGrant`
  - `TestFirstDepositPluginRejectsIncompatibleActivityTypeAtCreate`

## Commands

~~~bash
go test ./...
go build ./...
go test ./internal/plugin ./internal/marketing ./internal/engine -run 'TestRegistry|TestValidatePrelaunch|TestActivityPlugin|TestFirstDepositPlugin|TestSimulateEventIncludesPluginPreview' -count=1 -v
~~~

## Result

~~~text
PASS
~~~

## Evidence

- `go test ./...` 通过。
  - 覆盖包包括 `internal/engine`、`internal/marketing`、`internal/plugin`、`internal/httpapi`、`internal/workflow` 等。
  - 未出现失败、panic 或 data race 报告。
- `go build ./...` 通过。
  - 命令退出码为 0，无错误输出。
- 插件相关定向测试通过。
  - `internal/plugin`：注册表重复版本、最高兼容版本解析、能力缺失和禁用插件拒绝均通过。
  - `internal/marketing`：未知插件、活动类型不兼容、兼容插件上线前校验均通过。
  - `internal/engine`：插件执行失败拒绝且不生成 grant/outbox 并记录 audit、插件上下文进入规则/奖励/snapshot、模拟运行无持久化、FIRST_DEPOSIT 回归均通过。

## Failure Details

无失败命令。

需求：2026-05-29-activityplugin-lifecycle
