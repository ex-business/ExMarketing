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

已完成活动审批流最小闭环：

- 新增活动审批状态 `NONE / PENDING_L1 / PENDING_L2 / APPROVED / REJECTED / WITHDRAWN` 和活动 `approval` 快照。
- 新增提交审批、审批通过、审批驳回、查询审批记录接口。
- 普通活动一级审批通过后可上线；高危活动一级通过后进入二级审批，二级通过后才可上线。
- 上线门禁阻止未审批、审批未完成、被驳回或版本不一致的活动进入 `SCHEDULED` / `ACTIVE`。
- 正式事件匹配继续只允许 `ACTIVE` 活动，审批通过但处于 `SCHEDULED` 的活动不会生成 participation、grant_order 或 outbox。
- 审批动作和上线阻断写入 `marketing_audit`，审批记录查询从审计记录生成。

## Changed Files

- `internal/marketing/model.go`
- `internal/marketing/prelaunch_validation.go`
- `internal/engine/service.go`
- `internal/httpapi/handler.go`
- `internal/httpapi/handler_test.go`
- `internal/engine/service_test.go`
- `internal/engine/plugin_test.go`
- `internal/mqconsumer/consumer_test.go`
- `docs/api.md`
- `docs/data-model.md`

## Behavior Changes

- `POST /admin/v1/activities` 创建 `SCHEDULED` / `ACTIVE` 活动会返回审批门禁错误；应先创建 `DRAFT`，完成审批后再更新状态。
- `PUT /admin/v1/activities/{id}` 更新到 `SCHEDULED` / `ACTIVE` 时必须已有当前版本 `APPROVED` 审批。
- 配置类更新会推进活动版本并清空旧审批有效性，要求重新提交审批。
- 高危判定采用保守口径：资金类奖励、自动外部发奖、随机/概率/奖池分摊奖励、`RISK` / `LIMIT` 规则或外部 facts/state 引用都需要二级审批。
- 审批通过不代表正式事件可命中；`SCHEDULED` 仍按非 ACTIVE 处理。

## Notes

- 已执行 `go test ./...`，通过。
- 已执行 `go build ./...`，通过。
- `test.md` 和 `commit.md` 中的失败反馈为模板占位，没有实际 FAIL/BLOCK 原因；本轮按需求做最小实现并补充回归测试。

需求：2026-05-24-113929-8
