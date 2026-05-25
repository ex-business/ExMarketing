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

实现模拟运行接口的结构化活动级预览结果，并补齐 quota 影响预览与无副作用测试。

## Changed Files

- `internal/marketing/model.go`
  - 扩展 `SimulationResult`，新增 `matchedActivities`、`unmatchedActivities`、`summary`、活动级预览、奖励预览和 quota 影响预览结构。
- `internal/engine/service.go`
  - `SimulateEvent` 输出活动命中/未命中分组、互斥拒绝原因、规则结果、奖励预览、quota 影响预览、participation / grant_order / outbox command 预览。
  - quota 不足时返回 `QUOTA_EXCEEDED` 风险，不展示可发奖成功的 grant/outbox 预览。
  - 模拟运行只读活动和 quota，不调用 inbox、participation、grant_order、outbox 或 quota reservation 写入路径。
- `internal/marketing/repository.go`
  - 新增 `ListActivityQuotas` 只读仓储方法。
- `internal/marketing/memory_repository.go`
  - 实现内存仓储 quota 只读查询。
- `internal/postgres/repository.go`
  - 实现 PostgreSQL quota 只读查询。
- `internal/httpapi/handler_test.go`
  - 增加模拟运行活动/奖励/quota 预览测试。
  - 增加模拟运行 quota 超限时不展示 grant/outbox 预览测试。
- `docs/api.md`
  - 更新模拟运行输出说明和无副作用/预览边界。

## Behavior Changes

- `POST /admin/v1/simulate` 保留原有 `participations`、`grantOrders`、`outboxMessages`、`ruleResults` 字段，同时新增活动级结构化输出。
- 正常命中且 quota 充足时，返回 reward、quota、participation、grant order、outbox command 预览。
- 活动未激活或被互斥组排除时，进入 `unmatchedActivities` 并带明确原因。
- quota 缺失按无限制展示；quota 不足展示 `wouldExceed=true` 和 `QUOTA_EXCEEDED`，且不返回可执行发奖预览。
- 模拟运行不写真实 `marketing_event_inbox`、`marketing_participation`、`marketing_grant_order`、`marketing_outbox`，也不扣减/冻结 quota。

## Notes

- `review.md` 为 PASS。
- `test-plan.md` 为 PASS；本次按最小可验收范围实现并覆盖核心自动化测试。
- 已执行：
  - `go test ./...`
  - `go build ./...`

需求：2026-05-24-113400-8
