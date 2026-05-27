# Implementation

## Summary

- 补齐操作审计来源上下文：HTTP 请求自动注入 method/path、requestId/traceId、脱敏 IP、userAgent 摘要；所有 engine 审计 payload 自动带 `source`。
- 对齐本任务 action/resource 口径：活动、审批、模拟、quota、grant failed、outbox DLQ/失败审计使用稳定大写 resource 和明确 action。
- 审计查询要求 `tenantId`，默认 limit 由仓储限制为 100，最大 500。
- 补充发奖失败回调 `GRANT_FAILED` 审计、Outbox 投递失败 `OUTBOX_DELIVERY_FAILED` 和进入 DLQ `OUTBOX_ENTER_DLQ` 审计。
- 更新测试和文档，补齐 `test-plan.md` 为明确 PASS。

## Changed Files

- `internal/marketing/audit_context.go`
- `internal/httpapi/handler.go`
- `internal/engine/service.go`
- `internal/worker/outbox_worker.go`
- `internal/marketing/memory_repository.go`
- `internal/postgres/repository.go`
- `internal/httpapi/handler_test.go`
- `internal/worker/outbox_worker_test.go`
- `docs/api.md`
- `docs/data-model.md`
- `tasks/2026-05-24-113929-9/test-plan.md`

## Behavior Changes

- `GET /admin/v1/audit` 缺少 `tenantId` 会返回 400 类错误。
- 审计 payload 统一包含 `source` 摘要，不记录 Authorization、Cookie、token 或 secret。
- 审批审计 action 使用 `APPROVAL_SUBMIT`、`APPROVAL_APPROVE`、`APPROVAL_REJECT`。
- 发奖失败回调会产生 `GRANT_FAILED` 审计，可按 `commandId` 查询。
- Outbox worker 投递失败会产生 `OUTBOX_DELIVERY_FAILED`，进入 DLQ 会产生 `OUTBOX_ENTER_DLQ`；人工 replay/close 审计保留并补充 source。

## Verification

~~~bash
go test ./...
go build ./...
~~~

`go test ./...` 已执行通过。`go build ./...` 已执行通过。

## Notes

- `test.md` 和 `commit.md` 仍是模板内容，没有实际 FAIL/BLOCK/FAILED 上下文可优先修复。
- PostgreSQL 仓储审计查询保留 JSON 文本过滤方式，按本任务只做最小实现，不扩展完整对账查询能力。

需求：2026-05-24-113929-9
