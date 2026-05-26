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

- 按 `analysis.md` / `review.md` 的本期边界修正 DLQ close 行为：`close` 只关闭同一 outbox 为 `DLQ_CLOSED` 并写审计，不再联动修改 `grant_order`、`participation` 或 `quota_reservation`。
- 补强 DLQ 操作审计 payload，记录 reason、beforeStatus、afterStatus、targetChannel、targetRef、retryCount、errorMessage、commandId，便于复核失败原因和人工操作上下文。
- 补齐 DLQ 查询入口的 tenant 必填校验，避免空 tenant 查询跨租户 DLQ。
- 更新 MemoryRepository / PostgreSQL repository 测试，覆盖 close 后 grant、participation、quota reservation 保持不变。

## Changed Files

- `internal/marketing/memory_repository.go`
- `internal/postgres/repository.go`
- `internal/engine/service.go`
- `internal/httpapi/handler.go`
- `internal/httpapi/handler_test.go`
- `internal/marketing/memory_repository_test.go`
- `internal/postgres/repository_integration_test.go`
- `docs/api.md`
- `docs/data-model.md`
- `tasks/2026-05-24-dlq/implementation.md`

## Behavior Changes

- `POST /admin/v1/outbox/{id}/close`：
  - 仅允许当前 outbox 为 `DLQ`。
  - 成功后同一 outbox 进入 `DLQ_CLOSED`。
  - 不修改发奖单终态、不释放冻结 quota、不更新 participation 状态。
  - 审计 `OUTBOX_DLQ_CLOSE` 包含操作原因、前后状态和失败上下文。
- `POST /admin/v1/outbox/{id}/replay`：
  - 保持同一 outbox 回到 `PENDING`，payload 不变。
  - 审计 `OUTBOX_DLQ_REPLAY` 包含 target、retry、error、commandId。
- `GET /admin/v1/outbox/dlq`：
  - `tenantId` 为空时返回参数错误，不执行全租户 DLQ 查询。

## Notes

- 本轮未处理生产 Admin 鉴权 / RBAC；仍按 review 中的最小口径由请求体传入 operator。
- close 后 grant_order / quota 悬挂由 callback、对账或 quota timeout release 等后续流程承接，符合本期 analysis 边界。
- 验证命令：
  - `go test ./...`：PASS
  - `go build ./...`：PASS

需求：2026-05-24-dlq
