# Implementation

## Agent 指令

你是 Dev Agent。只有 review.md 为 PASS 且 test-plan.md 为 PASS 时才能开发。

必须按 task.md、analysis.md、review.md、test-plan.md 执行，不得扩大范围。

完成开发后必须先做基础验证：

~~~bash
go test ./...
go build ./...
~~~

## Summary

已完成活动插件体系的最小实现，严格限定为插件上下文、前置校验解释和无副作用 `stateChangeIntents`：

1. 新增活动级 `pluginCode`，未配置时保持普通活动纯配置链路。
2. 扩展 `internal/plugin` 注册表和 `ActivityPlugin` 契约，支持注册、activityType 兼容性、配置校验、上下文构建和资格通过后的状态变更意图预览。
3. 创建活动和上线前校验会拒绝未知或不兼容 `pluginCode`；运行时遇到历史脏配置会写拒绝型 participation，不发奖、不扣 quota、不写 outbox。
4. 规则和奖励计算只通过 `state.plugin.<pluginCode>.*` 读取插件上下文，且插件上下文被挂载到独立命名空间，不覆盖 `attributes.*`、`facts.*` 或既有 `state.*`。
5. 正式处理和模拟运行都会展示插件 `pluginCode`、`pluginContext`、`precheckResult`、`explain`、`stateChangeIntents`；本期没有新增插件状态表、仓储或真实玩法状态消费。

## Changed Files

- `internal/plugin/plugin.go`
  - 定义 ActivityPlugin 注册、兼容性、配置校验、上下文构建和状态意图接口。
- `internal/marketing/model.go`
  - 增加 `pluginCode`、插件 snapshot / simulate 输出模型，并收紧规则字段白名单到明确 state 命名空间。
- `internal/marketing/prelaunch_validation.go`
  - 增加上线前插件注册和 activityType 兼容性校验。
- `internal/engine/service.go`
  - 将插件上下文接入正式事件处理、奖励计算、snapshot 和 simulate。
- `internal/postgres/repository.go`
  - 持久化读写 `marketing_activity.plugin_code`。
- `db/migration/001_marketing_core.sql`
  - 增加 `plugin_code` 字段。
- `internal/engine/plugin_test.go`
  - 覆盖插件上下文参与规则/奖励、snapshot 意图、模拟预览、未知插件运行时兜底。
- `internal/marketing/prelaunch_validation_test.go`
  - 覆盖未知、不兼容和兼容插件的上线前校验。
- `README.md`、`docs/api.md`、`docs/architecture.md`、`docs/data-model.md`
  - 同步 ActivityPlugin 职责边界、字段路径和无副作用约束。

## Behavior Changes

- 普通活动不配置 `pluginCode` 时行为不变，仍通过活动配置、规则、奖励和 quota 完成。
- 配置 `pluginCode` 的活动必须使用已注册且兼容 activityType 的插件。
- `state.plugin.<pluginCode>.*` 是插件上下文唯一规则/奖励可读路径；`plugin.*`、`state.<activityType>.*`、短路径均不被配置校验接受。
- 插件 precheck 拒绝、上下文构建失败、运行时未知插件都会阻断该活动发奖，并产生明确 reasonCode。
- `AfterQualified` 只生成并记录/预览状态变更意图，不执行真实插件状态写入。

## Notes

- `tasks/2026-05-24-113400-9/test-plan.md` 当前为 PASS 且已包含测试矩阵；本轮按其中重点场景补了内存仓储单元测试。PostgreSQL 集成层已补字段读写，未新增插件状态表。
- 仓库进入本轮前已存在多处其它任务目录和 workflow/worker 文件的未提交改动；本轮未回退这些用户/既有改动。

## Verification

~~~bash
go test ./...
go build ./...
~~~

结果：均通过。

需求：2026-05-24-113400-9
