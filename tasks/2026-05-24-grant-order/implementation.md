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

- 补齐 grant order callback 状态机约束：`PENDING` 不再允许直接通过 callback 落 `SUCCESS` / `FAILED`；callback 只允许命中 `SENDING` / `RETRY` 的非终态单据。
- 保持终态幂等与冲突规则：相同终态重复 callback 返回 duplicate；相反终态或不同 `externalGrantId` 的重复 `SUCCESS` 返回冲突。
- outbox worker 在投递 reward command 前将 grant order 推进为 `SENDING`，投递失败或进入 DLQ 时将 grant order 推进为 `RETRY` 并递增 `retry_count`。
- 补齐测试计划 PASS 内容，并覆盖 review 提到的不同 `externalGrantId` 重复 SUCCESS callback 冲突场景。

## Changed Files

- `internal/marketing/memory_repository.go`
- `internal/postgres/repository.go`
- `internal/worker/outbox_worker.go`
- `internal/worker/outbox_worker_test.go`
- `internal/httpapi/handler_test.go`
- `tasks/2026-05-24-grant-order/test-plan.md`
- `tasks/2026-05-24-grant-order/implementation.md`

## Behavior Changes

- `ApplyRewardCallback` 现在拒绝 `PENDING` grant order callback，返回 `CALLBACK_STATUS_CONFLICT`。
- `UpdateGrantOrderByCommandID` 现在只允许：
  - `PENDING/RETRY -> SENDING`
  - `SENDING -> RETRY`
- `SENDING -> RETRY` 会累计 grant order 维度 `retry_count`。
- worker 对无法推进 grant order 状态的 reward command 不再继续投递，避免终态单被 outbox 重放反向改写。

## Verification

~~~bash
go test ./internal/httpapi ./internal/worker ./internal/marketing ./internal/postgres
go test ./...
go build ./...
~~~

## Notes

- 本轮未修改提交文件，也未执行 git commit。
- `test.md` 和 `commit.md` 中没有实际 FAIL / BLOCK 上下文；本轮按 review P2 和 analysis 状态机约束做最小修复。

需求：2026-05-24-grant-order
