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

已实现后台上线前配置校验的最小闭环：

- 新增 `marketing.ValidatePrelaunch`，返回逐项校验结果、总体结论、是否允许上线、是否需要二级审批、高危配置列表和审计摘要。
- 新增后台接口 `POST /admin/v1/activities/prelaunch-validation`，用于提交活动配置快照、quota、已有活动冲突上下文和审批快照并返回可展示校验结果。
- Engine 层新增 `ValidatePrelaunch` 并写入 `PRELAUNCH_VALIDATE` 审计记录。
- 覆盖概率非法阻断、自动资金类高危审批、审批版本绑定、互斥组冲突和 API 结构化返回测试。

## Changed Files

- `internal/marketing/prelaunch_validation.go`
- `internal/marketing/prelaunch_validation_test.go`
- `internal/engine/service.go`
- `internal/httpapi/handler.go`
- `internal/httpapi/handler_test.go`

## Behavior Changes

- 合法但高危的自动资金类奖励、未配置预算、随机/概率/奖池类奖励会返回 `APPROVAL_REQUIRED`，审批通过前 `allowGoLive=false`。
- 概率、规则、奖励、URL、Decimal、枚举等基础 Schema 错误会返回 `BLOCK`。
- 预算暴露超过 `BUDGET + ACTIVITY + TOTAL` 会返回 `BLOCK`；缺少预算但存在自动资金类奖励会标记高危。
- 概率/奖池类奖励缺少库存或库存为 0 会阻断；普通非库存型奖励缺少库存只返回 `WARN`。
- 同 tenant、同互斥组、时间窗口和触发事件重叠、不可叠加且优先级相同会返回 `BLOCK`。
- 二级审批必须与当前活动版本一致，且审批人不能等于提交人；拒绝或版本不匹配会阻断。

## Notes

- 为避免破坏现有大量测试夹具中直接创建 `ACTIVE` 活动的行为，本次未强改 `POST /admin/v1/activities` 的创建语义；上线阻断由新增预上线校验接口的 `allowGoLive=false` 表达。
- 当前审批以请求中的 `ApprovalSnapshot` 表示，未新增审批表或持久化审批流；后续如需要真实审批状态机，可在此结果模型上扩展。
- 已执行：
  - `go test ./...`
  - `go build ./...`

需求：2026-05-24-113400-7
