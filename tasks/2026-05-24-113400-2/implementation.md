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

- 为活动创建补充统一配置模型基础 Schema 校验：activity `config`、rule `ruleConfig`、reward `rewardConfig` 均拒绝未知字段和非法类型。
- 将当前可创建奖励收紧到可执行边界：`BONUS` / `CASH` / `POINT`，`FIXED` / `RATIO`，`AUTO`，`HTTP` / `WEBHOOK`，并强制 `rewardConfig.targetRef` 为 `http(s)` URL。
- 补齐 reward `version` 字段的 API 模型、归一化、Postgres 表结构和读写。
- 更新创建活动示例和数据模型/API 文档，明确发奖边界。

## Changed Files

- `internal/marketing/model.go`
- `internal/engine/service.go`
- `internal/postgres/repository.go`
- `internal/httpapi/handler_test.go`
- `db/migration/001_marketing_core.sql`
- `README.md`
- `docs/api.md`
- `docs/data-model.md`
- `tasks/2026-05-24-113400-2/implementation.md`

## Behavior Changes

- 创建活动时，非法 `timezone`、`status`、重复或空 `triggerEvents`、未知 config 字段、非法 rule/reward schema、不可执行发奖配置会返回 `400 Bad Request`。
- `MQ`、`MANUAL_EXPORT`、`MANUAL`、缺失或非法 `targetRef` 不再允许保存成运行时稳定失败的活动。
- 创建成功后的 reward version 默认为 `1`，并可持久化读回。
- rule/reward 的 `enabled` 省略时默认启用，显式传 `false` 时保持禁用语义。

## Notes

- 已执行 `go test ./...`。
- 已执行 `go build ./...`。

需求：2026-05-24-113400-2
