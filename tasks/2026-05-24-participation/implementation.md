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

- 按 `analysis.md` / `review.md` / `test-plan.md` 的 PASS 口径完成最小修复。
- 修复真实事件进入候选活动后被 entry/MATCH 配置拒绝时只返回 ruleResult、未落 `REJECTED` participation 的问题。
- 复用统一 rejected participation 创建逻辑，使 entry 拒绝和互斥组失败都带 participation snapshot、reject reason 和 participation 幂等键。

## Changed Files

- `internal/engine/service.go`
- `internal/httpapi/handler_test.go`
- `tasks/2026-05-24-participation/implementation.md`

## Behavior Changes

- `ProcessEventFrom` 对已进入候选活动且 `activityEntryAllowed` 拒绝的真实事件，会写入 `REJECTED` participation。
- entry 拒绝仍不生成 grant order/outbox，不扣减 quota。
- participation snapshot 保留 `activity`、`event`、`ruleResult`、`rewards` 等审计上下文；entry 拒绝场景的 `rejectReason` 形如 `ENTRY_DISABLED: activity entry is disabled`。
- 完全未进入候选或时间窗/状态未通过的活动仍按 analysis 默认最小口径不强制落 participation。

## Notes

- `tasks/2026-05-24-participation/test.md` 和 `commit.md` 当前仍是模板内容，没有实际 `FAIL`、`BLOCK` 或提交失败上下文可优先修复。
- `test-plan.md` 当前 Decision 为 `PASS`。
- 已执行并通过：

~~~bash
go test ./...
go build ./...
~~~

需求：2026-05-24-participation
