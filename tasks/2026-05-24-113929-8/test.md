# Test

## Agent 指令

你是 Test Agent。请根据 task.md、analysis.md、review.md、test-plan.md 和 implementation.md 执行完整测试。

最低要求：

~~~bash
go test ./...
go build ./...
~~~

## Test Cases

- 全量 Go 单元/集成测试：覆盖当前仓库所有包，包含活动审批、事件处理、quota、幂等、审计、PostgreSQL repository 跳过/缓存路径等现有测试。
- 构建验证：覆盖所有 Go command/package 的编译。
- PostgreSQL smoke：启动 `docker compose` 中的 `postgres`，使用真实 `EX_MARKETING_DATABASE_URL` 执行 `internal/postgres` 及审批/活动/事件/幂等/quota/并发/Postgres 相关测试。
- 精确审批回归：执行 `internal/marketing`、`internal/httpapi`、`internal/engine` 中与审批、上线门禁、SCHEDULED 事件语义、审计、幂等、quota、事件处理相关的测试。

## Commands

~~~bash
go test ./...
go build ./...
docker compose up -d postgres
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./internal/postgres -run 'TestPostgres|Approval|Activity' -count=1
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./... -run 'Approval|Activity|Event|Idempot|Quota|Concurrent|Postgres' -count=1
go test ./internal/marketing -run 'Approval|ActivityApproval|ConcurrentApproval|IdempotentApproval' -count=1
go test ./internal/httpapi -run 'Approval|ActivityApproval|Audit|Scheduled|Idempotency' -count=1
go test ./internal/engine -run 'Approval|Scheduled|Quota|Idempotency|Event' -count=1
~~~

## Result

~~~text
PASS
~~~

## Evidence

- `go test ./...`：通过。关键包结果包括 `internal/engine`、`internal/httpapi`、`internal/marketing`、`internal/mqconsumer`、`internal/postgres`、`internal/reward`、`internal/rule`、`internal/worker`、`internal/workflow` 均为 `ok`。
- `go build ./...`：通过，无编译错误输出。
- `docker compose up -d postgres`：通过，`exmarketing-postgres` 状态为 `Running`。
- `EX_MARKETING_DATABASE_URL=... go test ./internal/postgres -run 'TestPostgres|Approval|Activity' -count=1`：通过，`ok github.com/ex-stack/ExMarketing/internal/postgres 0.926s`。
- `EX_MARKETING_DATABASE_URL=... go test ./... -run 'Approval|Activity|Event|Idempot|Quota|Concurrent|Postgres' -count=1`：通过。相关包 `internal/engine`、`internal/httpapi`、`internal/marketing`、`internal/mqconsumer`、`internal/postgres`、`internal/rule` 均为 `ok`；部分无匹配测试包显示 `[no tests to run]`。
- `go test ./internal/marketing -run 'Approval|ActivityApproval|ConcurrentApproval|IdempotentApproval' -count=1`：通过，`ok github.com/ex-stack/ExMarketing/internal/marketing 0.168s`。
- `go test ./internal/httpapi -run 'Approval|ActivityApproval|Audit|Scheduled|Idempotency' -count=1`：通过，`ok github.com/ex-stack/ExMarketing/internal/httpapi 0.551s`。
- `go test ./internal/engine -run 'Approval|Scheduled|Quota|Idempotency|Event' -count=1`：通过，`ok github.com/ex-stack/ExMarketing/internal/engine 0.253s`。

## Failure Details

无失败命令、失败日志或复现步骤。本轮测试未发现需要打回 Dev Agent 的问题。

需求：2026-05-24-113929-8
