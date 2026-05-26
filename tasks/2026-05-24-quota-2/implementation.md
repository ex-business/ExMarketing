# Implementation

需求：2026-05-24-quota-2

## Scope

- 修复不限额活动 callback：没有任何 quota 配置且没有 reservation 的 grant_order 可正常终结，不再误报 `QUOTA_RESERVATION_MISSING`。
- 修复 participation 级次数 quota 聚合：同一 participation 下多个 grant_order 时，任一 grant_order 成功则 participation 最终为 `GRANTED`，`USER_LIMIT` / `DAILY_LIMIT` 只确认一次；全部失败才释放一次。
- 新增 Admin/Operations 超时释放入口：`POST /admin/v1/quota-reservations/timeout-release`。
- 同步内存仓储、PostgreSQL 仓储、Engine service、HTTP handler、文档和测试。

## Code Changes

- `internal/marketing/model.go`
  - 新增 `QuotaTimeoutReleaseCommand`、`QuotaTimeoutReleaseResult`、`QuotaTimeoutReleaseItem`。
  - 校验 `tenantId`、`operator`、`reason` 必填，`limit` 不可为负数。
- `internal/marketing/repository.go`
  - 仓储接口新增 `ReleaseExpiredQuotaReservations`。
- `internal/engine/service.go`
  - 新增 `ReleaseExpiredQuotaReservations` 服务方法。
  - 默认 `olderThan = now - 2h`，默认 `limit = 100`。
  - 写入 `QUOTA_TIMEOUT_RELEASE_DRY_RUN` / `QUOTA_TIMEOUT_RELEASE` 审计。
- `internal/httpapi/handler.go`
  - 新增 `POST /admin/v1/quota-reservations/timeout-release`。
  - 映射 invalid request、reservation conflict、aggregate conflict 为机器可识别错误码。
- `internal/marketing/memory_repository.go`
  - callback 在无 reservation 时根据活动创建时是否已有 quota 区分“不限额兼容”和“应有 reservation 缺失”。
  - 超时释放支持 tenant、olderThan、limit、dryRun、operator、reason。
  - 实际释放将候选 grant_order 置为 `FAILED`，错误信息带 `TIMEOUT_RELEASE`，释放仍为 `FROZEN` 的 reservation。
- `internal/postgres/repository.go`
  - PostgreSQL callback 路径同步不限额兼容判断。
  - PostgreSQL 超时释放在事务中锁定候选 grant_order，释放 quota reservation 并更新 participation 聚合状态。
- `docs/api.md`、`docs/data-model.md`、`docs/architecture.md`、`docs/operations.md`
  - 同步冻结、确认、释放、不限额兼容和 Admin 超时释放入口。

## Tests Added

- `internal/marketing/memory_repository_test.go`
  - 不限额 callback 无 reservation 成功终结。
  - 多 grant_order 下任一成功时 participation 级次数 quota 只确认一次。
  - 超时释放 dry-run、实际释放、重复执行幂等。
- `internal/httpapi/handler_test.go`
  - Admin 超时释放入口 dry-run、实际释放和审计记录。

## Commands

```bash
go test ./...
go build ./...
```

## Result

```text
PASS
```

## Evidence

- `go test ./...` 通过。
- `go build ./...` 通过。
