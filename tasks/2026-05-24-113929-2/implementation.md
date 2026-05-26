# Implementation

## Summary

完成活动展示与入口配置的最小实现：活动 `config/configJson/config_json` 可保存通用 `display` 与 `entry` 字段，创建/更新阶段校验类型、URL、字符串数组和租户边界；引擎在 MATCH 阶段按通用入口配置过滤候选活动，并在模拟运行和正式事件处理中输出结构化拒绝原因。

## Changed Files

- `internal/marketing/model.go`
  - 扩展 activity config schema：`display.visible/description/tags`、`entry.enabled/entryCodes/sourceSystems/tenantIds/audience`。
  - `entry.tenantIds` 必须匹配活动自身 `tenantId`，不能扩大租户范围。
  - 字符串数组元素不能为空且不可重复。
- `internal/engine/service.go`
  - 在活动运行态和互斥组判断之间增加入口配置预检。
  - 支持 `ENTRY_DISABLED`、`ENTRY_CHANNEL_NOT_ALLOWED`、`ENTRY_SOURCE_SYSTEM_NOT_ALLOWED`、`ENTRY_TENANT_NOT_ALLOWED`、`ENTRY_SUBJECT_TYPE_NOT_ALLOWED`、`ENTRY_SEGMENT_NOT_ALLOWED`、`ENTRY_RISK_LEVEL_NOT_ALLOWED` 等 MATCH 阶段拒绝原因。
  - `display.visible=false` 不参与引擎候选过滤。
- `internal/httpapi/handler_test.go`
  - 增加展示/入口配置校验、入口拒绝、隐藏展示不阻断发奖的测试。
  - 调整事件测试样例，提供通用 `attributes.channel` 以匹配已有 `entry.channels` 配置。
- `docs/api.md`
  - 补充展示与入口配置字段、默认语义、拒绝原因和兼容字段说明。
- `tasks/2026-05-24-113929-2/test-plan.md`
  - 从模板补齐为明确 PASS 的测试计划。

## Behavior Changes

- 创建/更新活动时，非法展示与入口配置返回 `400 Bad Request`。
- 旧活动缺失 `entry` 或未配置 `entry.enabled` 时保持兼容，不增加限制。
- `entry.enabled=false` 会阻断 MATCH 候选，不生成 participation、grant_order 或 outbox。
- `display.visible=false` 只影响展示语义，不阻断正式事件处理、模拟运行、奖励计算或 quota 预览。
- 入口匹配只读取活动通用 config、标准事件字段和 `attributes`，不引用具体业务页面或外部业务库。

## Verification

```bash
go test ./...
go build ./...
```

两条命令均已通过。

需求：2026-05-24-113929-2
