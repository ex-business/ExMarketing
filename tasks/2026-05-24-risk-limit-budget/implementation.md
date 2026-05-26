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

本轮按返工反馈做最小修复：

- 补全 `test-plan.md`，Decision 更新为 `PASS`，测试范围覆盖 RISK/LIMIT/BUDGET、互斥、幂等、预算、库存、USER_LIMIT、DAILY_LIMIT 和模拟运行。
- 在原子发奖命令中加入 `DAILY_LIMIT + USER + subject.id:activityLocalDate` 扣减，活动本地日按 activity timezone 从事件 `occurredAt` 计算。
- `ProcessEvent` 对同一事件互斥落选活动写 `REJECTED` participation，`rejectReason=EXCLUSIVE_GROUP_LOST...`，不生成 grant_order/outbox，不扣 quota。
- 模拟运行 quota preview 同步展示 `USER_LIMIT` 和 `DAILY_LIMIT` 风险。
- 同步 API、架构和数据模型文档中 DAILY_LIMIT 已支持的说明。

## Changed Files

- `internal/marketing/model.go`
- `internal/marketing/memory_repository.go`
- `internal/postgres/repository.go`
- `internal/engine/service.go`
- `internal/engine/service_test.go`
- `docs/api.md`
- `docs/architecture.md`
- `docs/data-model.md`
- `tasks/2026-05-24-risk-limit-budget/test-plan.md`
- `tasks/2026-05-24-risk-limit-budget/implementation.md`

## Behavior Changes

- 成功发奖链路现在会在同一 activity 级原子边界内同时检查并扣减 `USER_LIMIT` 和 `DAILY_LIMIT`；缺少 DAILY_LIMIT quota 行仍按无限制处理。
- DAILY_LIMIT scopeKey 格式为 `subject.id:YYYY-MM-DD`，日期使用活动 timezone。
- DAILY_LIMIT 不足返回既有结构化 quota 错误：`QUOTA_EXCEEDED`，`quotaType=DAILY_LIMIT`，`scope=USER`，`scopeKey=subject.id:YYYY-MM-DD`。
- 同一事件互斥落选活动会留下可审计 participation 解释，不会静默跳过。

## Verification

~~~bash
go test ./...
go build ./...
~~~

结果：全部通过。

## Notes

- 未实现 IP / Device quota 扣减；按 analysis/review/test-plan，本轮仅保留它们作为普通 CONDITION 字段。
- 未实现跨历史 participation 的活动期互斥；本轮只处理同一事件候选活动裁剪。

需求：2026-05-24-risk-limit-budget
