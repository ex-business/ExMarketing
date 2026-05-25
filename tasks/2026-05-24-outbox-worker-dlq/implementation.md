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

已完成 HTTP / WEBHOOK Outbox Worker 重试与 DLQ 最小实现。

本次实现内容：

- Worker 只 claim `HTTP` / `WEBHOOK` outbox，`MQ` 不投递、不进入本任务 retry / DLQ 流程。
- HTTP 2xx 标记 `SENT`。
- HTTP 408 / 429 / 5xx、网络错误、超时等按可重试失败处理。
- HTTP 400 / 401 / 403 / 404 / 409 / 422、非法 `target_ref`、非法 JSON payload 直接进入 `DLQ`。
- 最大失败次数调整为 5 次。
- 重试时间按 30s 初始延迟、2 倍指数退避、30m 上限、正负 20% jitter 计算。
- HTTP 429 可解析 `Retry-After` 时优先使用该值。
- 失败写入 `retry_count`、`next_retry_at`、`error_message`、`updated_at`；进入 DLQ 时更新最终 `retry_count` 并停止自动重试。
- 增加 `DLQ_CLOSED` 状态。
- 增加 tenant 范围 DLQ 查询、DLQ replay、DLQ close。
- replay / close 只允许当前状态为 `DLQ`，缺少 `tenantId` / `operator` / `reason` 会失败。
- replay 后同一 outbox 回到 `PENDING`，不创建新的 participation、grant_order 或 outbox，payload 保持不变。
- close 后同一 outbox 进入 `DLQ_CLOSED`，`DLQ_CLOSED` 不允许 replay。
- replay / close 写 `marketing_audit`，记录 operator、reason、前后状态和操作时间。

## Changed Files

- `internal/worker/outbox_worker.go`
- `internal/worker/outbox_worker_test.go`
- `internal/marketing/model.go`
- `internal/marketing/repository.go`
- `internal/marketing/memory_repository.go`
- `internal/postgres/repository.go`
- `internal/engine/service.go`
- `internal/httpapi/handler.go`
- `internal/httpapi/handler_test.go`
- `README.md`
- `docs/api.md`
- `docs/operations.md`
- `tasks/2026-05-24-outbox-worker-dlq/test-plan.md`
- `tasks/2026-05-24-outbox-worker-dlq/implementation.md`

## Behavior Changes

Worker 行为：

- `HTTP` / `WEBHOOK` 才会被自动 claim。
- `MQ` 保持待发送，不作为本期自动投递、retry、DLQ、replay、close 验收项。
- 可重试失败进入 `FAILED` 并等待 `next_retry_at` 到期后再次 claim。
- 不可重试失败直接进入 `DLQ`。
- 第 5 次失败进入 `DLQ`，自动投递停止。

Admin API：

- `GET /admin/v1/outbox/dlq?tenantId=site_001&includeClosed=true`
- `POST /admin/v1/outbox/{id}/replay`
- `POST /admin/v1/outbox/{id}/close`

Replay / close 请求体：

~~~json
{
  "tenantId": "site_001",
  "operator": "ops",
  "reason": "target recovered"
}
~~~

## Verification

已执行：

~~~bash
go test ./...
go build ./...
~~~

结果：PASS。

## Notes

- 未实现 MQ sender / MQ retry / MQ DLQ / MQ replay / MQ close，符合本任务非目标。
- 未实现 `SENDING` 悬挂恢复阈值，analysis.md 已列为非阻塞后续问题。
- 本次没有提交 git commit。

需求：2026-05-24-outbox-worker-dlq
