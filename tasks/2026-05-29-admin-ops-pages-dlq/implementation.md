# Implementation

## Agent 指令

你是 Dev Agent。只有 review.md 为 PASS 且 test-plan.md 为 PASS 时才能开发或修复。

必须按 task.md、analysis.md、review.md、test-plan.md 执行，不得扩大范围。
如果 test.md 已存在且 Result 为 FAIL，必须优先读取失败原因，做最小修复后再更新 implementation.md。
如果 commit.md 或提交日志存在 BLOCK / FAILED / 提交失败，必须优先读取提交失败上下文，做最小修复后再更新 implementation.md。

完成开发后必须先做基础验证：

~~~bash
go test ./...
go build ./...
~~~

## Summary

- 按 Test Agent 返工反馈完成最小修复：同步 Postgres 集成测试中的人工发奖 / 导出发奖审计 action 断言，移除旧 `MANUAL_GRANT_EXPORT_READY` / `MANUAL_GRANT_CONFIRM_SUCCESS` 期望，改为任务契约定义的 `MANUAL_GRANT_EXPORT_CREATE` / `MANUAL_GRANT_DECISION`。
- 按返工反馈完成最小修复：审计查询新增服务端 `startTime` / `endTime` / `traceId` / `requestId` 过滤，并补充 HTTP 参数解析、内存仓储和 Postgres 仓储过滤逻辑。
- 将人工发奖和导出发奖审计 action 收敛为需求定义的 `MANUAL_GRANT_DECISION`、`MANUAL_GRANT_EXPORT_CREATE`，并在 API 文档中固化 `MANUAL_GRANT_EXPORT_STATUS` 后续状态 action 契约。
- 更新 Admin 运维页面：审计页补齐过滤项；对账页明确是“按条件定位”；DLQ 页明确只展示 DLQ；新增人工发奖确认和导出创建表单。
- 补充 HTTP 层测试，覆盖审计时间 + trace/request 过滤、非法时间校验、manual decision/export 稳定审计 action。

## Changed Files

- `internal/marketing/model.go`
- `internal/marketing/memory_repository.go`
- `internal/postgres/repository.go`
- `internal/engine/service.go`
- `internal/httpapi/handler.go`
- `internal/httpapi/handler_test.go`
- `web/admin/index.html`
- `docs/api.md`
- `internal/postgres/repository_integration_test.go`
- `tasks/2026-05-29-admin-ops-pages-dlq/implementation.md`

## Behavior Changes

- `GET /admin/v1/audit` 新增查询参数：
  - `startTime`、`endTime`：RFC3339；服务端按 `createdAt >= startTime` 且 `createdAt < endTime` 过滤。
  - `traceId`、`requestId`：精确匹配 audit payload 中 `source.traceId` / `source.requestId`。
- `GET /admin/v1/audit` 对非法 RFC3339 时间返回 `400`，对 `startTime >= endTime` 返回 `400`。
- 人工发奖 decision 写 `MANUAL_GRANT_DECISION` 审计，payload 包含 decision/new status、operator、reason、billNo/commandId、idempotencyKey 和 quota reservation 摘要。
- 手工导出创建写 `MANUAL_GRANT_EXPORT_CREATE` 审计，payload 包含 exportId、fileFormat、rowCount、checksum、status、明细数量、operator 和 reason。
- DLQ replay/close 审计中的 `targetRef` 去除 query/fragment，并对明显 secret/token/password/authorization 内容脱敏。

## Notes

- 本轮返工优先处理 `test.md` 记录的 FAIL：`TestPostgresManualExportReadySuccessPersistsTraceAuditAndQuota` 仍断言旧审计 action。
- `commit.md` 当前仍为模板占位，未记录实际 BLOCK / FAILED / 提交失败上下文。
- `review.md` 为 PASS；`test-plan.md` 为 PASS。
- 已执行：

~~~bash
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./internal/postgres -run 'TestPostgresManualExportReadySuccessPersistsTraceAuditAndQuota$' -count=1 -v
go test ./...
go build ./...
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./...
~~~

- 结果：以上命令均通过。

需求：2026-05-29-admin-ops-pages-dlq
