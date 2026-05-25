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

已完成标准事件模型 Schema 校验最小实现：

- `ActivityEvent` JSON 解码阶段显式校验 8 个顶层字段，只允许 `eventId`、`eventType`、`sourceSystem`、`tenantId`、`subject`、`biz`、`occurredAt`、`attributes`。
- `subject` 只允许 `type`、`id`；`biz` 只允许 `bizType`、`bizId`。
- 必填字符串字段必须是非空字符串，`occurredAt` 必须是带时区的 RFC3339 时间。
- `attributes` 必须存在且必须是 object；允许 `{}`、嵌套 object/array 和任意业务字段。
- 正式事件和模拟运行均返回 `INVALID_EVENT`，并保留字段路径或原因；幂等键错误继续返回 `INVALID_IDEMPOTENCY_KEY`。

## Changed Files

- `internal/marketing/model.go`
  - 为 `ActivityEvent` 增加自定义 `UnmarshalJSON`。
  - 增强 `Validate`，拒绝空白字符串和 nil `attributes`。
  - 增加标准事件字段、对象和时间校验 helper。
- `internal/httpapi/handler.go`
  - `POST /openapi/v1/events` 解码失败时返回具体 `INVALID_EVENT` 原因。
  - `POST /admin/v1/simulate` 对标准事件 Schema 错误返回 `INVALID_EVENT`。
- `internal/httpapi/handler_test.go`
  - 补充缺字段、未知字段、非法 subject/biz/time/attributes、空和嵌套 attributes、模拟运行 Schema 错误测试。
  - 将原先接受 missing/null attributes 的测试改为拒绝。
- `docs/api.md`
  - 补充标准事件字段表、subject/biz 子字段、时间示例、错误响应示例和 Fact Query 对齐说明。
- `tasks/2026-05-24-schema/test-plan.md`
  - 补齐测试计划并标记 PASS。

## Behavior Changes

- `attributes` 缺失或为 `null` 不再自动归一为空对象，改为 `400 INVALID_EVENT`。
- 顶层、`subject`、`biz` 中出现业务字段会被拒绝，错误消息包含字段路径，例如 `subject.vipLevel` 或 `biz.amount`。
- `occurredAt` 缺少时区或不可解析时返回 `400 INVALID_EVENT`。
- 合法的空 `attributes: {}` 和嵌套业务字段仍可通过。

## Verification

~~~bash
go test ./...
go build ./...
~~~

结果：均通过。

## Notes

- 未新增业务枚举、source allowlist、字段字符集或长度限制，遵循 review.md 的 P2 约束。
- 未改变幂等键格式和重复事件处理语义。

需求：2026-05-24-schema
