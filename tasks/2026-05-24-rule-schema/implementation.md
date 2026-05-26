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

已按 `analysis.md` / `review.md` 的 PASS 口径完成最小修复：

- 补齐规则配置 Schema 校验边界，覆盖 `CONDITION / FORMULA / LIMIT / SCRIPT`。
- `CONDITION` 创建与上线前校验现在拒绝缺失 `logic`、空 `conditions`、未知字段、非法字段路径、非法 `op`、缺失或非法 `value`、空 `IN / NOT_IN`、布尔比较和负 `sort`。
- 标准事件字段 `eventId / eventType / sourceSystem / tenantId / subject.type / subject.id / biz.bizType / biz.bizId / occurredAt` 已作为合法字段路径保留；非标准顶层业务路径仍被拒绝。
- `enabled=false` 不再绕过 Schema 校验；禁用规则只影响运行时跳过。
- `FORMULA / LIMIT / SCRIPT` 会先做结构 Schema 校验，再因 evaluator 未注册被拒绝保存，避免默认通过或落库成可运行规则。

## Changed Files

- `internal/marketing/model.go`
- `internal/marketing/prelaunch_validation_test.go`
- `docs/api.md`
- `docs/data-model.md`
- `tasks/2026-05-24-rule-schema/implementation.md`

## Behavior Changes

- Admin 创建活动和 prelaunch validation 复用 `CreateActivityCommand.Validate`，因此都会阻断非法规则 Schema。
- `SCRIPT` 已加入规则类型枚举，但当前没有注册 evaluator，合法结构也会被保存前拒绝。
- `facts.<factCode>.<path>` 路径校验收紧为必须包含 factCode 和子路径。

## Notes

- `review.md` 为 PASS；`test-plan.md` 当前仍是模板内容，未包含可执行用例细节，但本轮根据 `analysis.md` 的验收标准补了针对性自动化测试。
- 本轮没有新增规则更新接口；现有 `PUT /admin/v1/activities/{id}` 仍只更新活动主配置。
- 已执行 `go test ./internal/marketing ./internal/httpapi ./internal/rule` 和 `go test ./internal/engine`，均通过。
- 已执行 `go test ./...`，通过。
- 已执行 `go build ./...`，通过。

需求：2026-05-24-rule-schema
