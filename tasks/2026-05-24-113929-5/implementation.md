# Implementation

## Summary

- 修复 DLQ 人工操作与发奖单状态同步：`replay` 现在要求关联 `grant_order` 仍为 `RETRY`；`close` 会将关联 `grant_order` 置为 `CLOSED`，并阻止已终态发奖单被覆盖。
- 修复 DLQ close 的 quota/participation 收尾：关闭投递层 DLQ 时释放仍冻结的 grant quota reservation，并在参与记录所有发奖单终结后把 participation 更新为失败终态。
- 保持 Outbox 投递层和 callback 结果层边界：`SENT` 仍只表示投递成功；真实 `SUCCESS/FAILED` 由 callback 处理，DLQ replay 不用于已终结发奖单。

## Changed Files

- `internal/marketing/memory_repository.go`
  - `ReplayOutboxDLQ` 校验关联 command 的 grant_order 必须为 `RETRY`。
  - `CloseOutboxDLQ` 同步关闭关联 grant_order、释放冻结 quota、更新 participation，并保留审计。
  - participation 终态聚合将 `CLOSED` 视为失败类终态。
- `internal/postgres/repository.go`
  - PostgreSQL DLQ replay/close 补齐与内存仓储一致的 grant_order 校验、关闭和 quota 释放事务语义。
  - participation 终态聚合将 `CLOSED` 视为失败类终态。
- `internal/marketing/memory_repository_test.go`
  - 新增 DLQ close 关闭 grant_order、释放 quota、更新 participation 的单元测试。
- `internal/postgres/repository_integration_test.go`
  - 新增 PostgreSQL DLQ close 事务语义集成测试；未设置 `EX_MARKETING_DATABASE_URL` 时按现有约定跳过。
- `internal/httpapi/handler_test.go`
  - 调整 DLQ replay/close API 测试数据，使其包含符合验收语义的 `RETRY` grant_order。

## Behavior Changes

- `DLQ` outbox 的 replay 只允许关联发奖单处于 `RETRY`，避免对已终结或不一致发奖单重新投递。
- `DLQ` outbox 的 close 会把关联发奖单终结为 `CLOSED`，释放仍冻结额度，并把全失败/关闭的 participation 汇总为失败态。
- 已为 `SUCCESS/FAILED/CLOSED` 的 grant_order 不会被 DLQ close/replay 覆盖。

## Verification

```bash
go test ./internal/marketing ./internal/httpapi ./internal/worker ./internal/postgres
go test ./internal/marketing -run TestMemoryCloseOutboxDLQClosesGrantAndReleasesQuota -count=1
go test ./...
go build ./...
```

Result: all commands passed.

需求：2026-05-24-113929-5
