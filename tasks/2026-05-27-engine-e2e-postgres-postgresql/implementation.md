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

已实现真实 PostgreSQL 端到端验收命令：

```bash
export EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable'
go run ./cmd/engine-e2e-postgres
```

命令会拒绝未设置 `EX_MARKETING_DATABASE_URL` 的内存仓储模式，在进程内启动 API handler、Outbox worker 和 reward stub，并通过 HTTP API 串联创建活动、提交审批、两级审批、激活活动、创建 quota、上报标准事件、重复事件幂等、Outbox worker 发送、SUCCESS / FAILED callback、quota 确认 / 释放和 reconciliation 查询。所有业务状态写入真实 PostgreSQL。

## Changed Files

- `cmd/engine-e2e-postgres/main.go`
  - 新增一条命令式 PostgreSQL E2E runner。
  - 使用唯一 `runId` 作为 activityCode、eventId、bizId 前缀，可重复执行。
  - 成功分支 subjectId 固定为 `success_user`，失败分支 subjectId 固定为 `failed_user`，分别创建 USER_LIMIT / DAILY_LIMIT quota，避免单用户次数额度阻塞失败 callback 分支。
  - 失败时输出阶段、HTTP 请求/响应、环境摘要和 activity / quota / inbox / participation / grant_order / outbox / reservation / audit 数据库快照。
- `docs/operations.md`
  - 补充 PostgreSQL E2E 验收命令、运行边界和重复执行说明。
- `tasks/2026-05-27-engine-e2e-postgres-postgresql/implementation.md`
  - 记录本轮实现、行为变化和验证证据。

## Behavior Changes

- 新增可执行命令 `go run ./cmd/engine-e2e-postgres`。
- 命令默认使用租户 `site_001`，活动奖励 `E2E_PG_BONUS`，每次生成唯一 `e2e_pg_<timestamp>` runId。
- quota 覆盖：
  - `BUDGET + ACTIVITY + TOTAL = 20`
  - `STOCK + ACTIVITY + E2E_PG_BONUS = 2`
  - `USER_LIMIT + USER + success_user = 1`
  - `USER_LIMIT + USER + failed_user = 1`
  - `DAILY_LIMIT + USER + success_user:<UTC date> = 1`
  - `DAILY_LIMIT + USER + failed_user:<UTC date> = 1`
- 预期最终 quota：成功分支确认 `10` 金额和 1 次库存/次数；失败分支释放冻结额度，最终 frozen 全部为 `0`。

## Notes

- `test.md` 和 `commit.md` 当前为模板态，没有真实 FAIL / BLOCK 原因；本轮无提交失败上下文需要优先修复。
- `review.md` 为 PASS。`test-plan.md` 当前仍是模板态，但 Review 的 P2 要求 Dev 记录实际命令，本实现已在本文件和 `docs/operations.md` 写明。
- 已执行：
  - `go test ./...` PASS
  - `go build ./...` PASS
  - `EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go run ./cmd/engine-e2e-postgres` PASS，输出 runId `e2e_pg_20260527_151210_000000000`

需求：2026-05-27-engine-e2e-postgres-postgresql
