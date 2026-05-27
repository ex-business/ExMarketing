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

按 `analysis.md`、`review.md` 和已 PASS 的 `test-plan.md` 完成 ActivityPlugin 注册表返工修复。

本轮基线 `go test ./...`、`go build ./...` 均已通过；未发现 `test.md` 或 `commit.md` 中存在真实 FAIL / BLOCK 上下文，两个文件仍是模板内容。

最小修复点：

- `ActivityPlugin` 注册表拒绝重复 `pluginCode`，避免后注册插件静默覆盖前一个插件，满足“重复插件编码不得产生歧义”的验收口径。
- 新增注册表单元测试，固定重复注册返回错误且原插件仍可查找。

## Changed Files

- `internal/plugin/plugin.go`
  - `Registry.Register` 在写入前检查已存在编码，重复编码返回错误。
- `internal/plugin/plugin_test.go`
  - 新增 `TestRegistryRejectsDuplicatePluginCode`，覆盖重复注册、错误返回、原注册项不丢失。
- `tasks/2026-05-24-activityplugin/implementation.md`
  - 记录本轮实现、行为变化和验证结果。

## Behavior Changes

- 注册同一 `pluginCode` 的第二个插件会失败，不再覆盖已有插件。
- 未配置插件的普通活动行为不变。
- 已有插件生命周期接口、活动配置校验、字段路径校验、插件预检查拒绝写 `REJECTED` participation、模拟无副作用等行为保持原实现。

## Notes

已执行验证：

~~~bash
go test ./...
go build ./...
~~~

结果：均通过。

未执行 `go test -race ./...` 和 PostgreSQL smoke；本轮用户强制要求的最低命令为 `go test ./...` 和 `go build ./...`，已完成。后续交给 Test Agent 时，可按 `test-plan.md` 继续补充 race / PostgreSQL smoke。

需求：2026-05-24-activityplugin
