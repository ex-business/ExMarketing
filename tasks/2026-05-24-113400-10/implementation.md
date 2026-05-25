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

已完成审计记录与对账的最小实现补齐：

- 审计查询支持按 `tenantId`、`actor`、`action`、`resource`、`resourceId`、`activityId`、`eventId`、`commandId` 和 `limit` 过滤。
- 创建活动、创建 quota、预上线校验、事件入站、模拟运行、quota 不足审计 payload 统一包含 `beforeSnapshot` / `afterSnapshot` 或失败上下文。
- 模拟运行新增 `SIMULATION_RUN` 审计，但仍不创建真实 `event_inbox`、`participation`、`grant_order`、`outbox`，也不扣减 quota。
- 发奖对账接口扩展为可按活动、用户、事件、幂等键、发奖单查询，并返回 event inbox、outbox、quota reservations、audit timeline。
- PostgreSQL migration 增加 audit tenant/time 和 resource 查询索引。
- 未扩展完整 RBAC、审批流引擎、外部系统对账或导出能力。

## Changed Files

- `internal/marketing/model.go`
- `internal/marketing/repository.go`
- `internal/marketing/memory_repository.go`
- `internal/postgres/repository.go`
- `internal/engine/service.go`
- `internal/httpapi/handler.go`
- `internal/httpapi/handler_test.go`
- `internal/worker/outbox_worker_test.go`
- `db/migration/002_audit_fact_outbox_indexes.sql`
- `docs/api.md`
- `README.md`
- `tasks/2026-05-24-113400-10/test-plan.md`

## Behavior Changes

- `GET /admin/v1/audit` 现在接受筛选参数，默认仍返回最多 100 条。
- `POST /admin/v1/activities` 和 `POST /admin/v1/quotas` 可选传 `operator`；缺省时审计 actor 使用 `system`，保持兼容。
- `POST /admin/v1/quotas` 可选传 `tenantId` 用于 quota 创建审计租户归属；旧请求仍兼容。
- `GET /admin/v1/reward/reconciliation` 新增 `grantOrderId`、`activityId`、`subjectType`、`subjectId`、`eventId`、`idempotencyKey` 查询参数。
- 对账响应在原字段基础上追加 `eventInbox`、`outboxMessages`、`auditTimeline`。

## Notes

验证已完成：

~~~bash
go test ./...
go build ./...
~~~

两条命令均通过。

需求：2026-05-24-113400-10
