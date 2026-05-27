# Test

## Test Cases

- PASS: 聚焦包回归覆盖 `internal/marketing`、`internal/httpapi`、`internal/worker`、`internal/postgres`。
- PASS: race 测试覆盖审计相关内存仓储、HTTP handler、worker 并发风险。
- PASS: 审计关键词筛选覆盖 `Audit`、`Quota`、`DLQ`、`Outbox`、`Postgres` 相关测试。
- PASS: 全量 Go 测试通过。
- PASS: 全量 Go build 通过。
- PASS: PostgreSQL smoke 已使用项目 `docker-compose.yml` 的 `postgres` 服务和 `EX_MARKETING_DATABASE_URL` 补跑，避免只依赖未设置环境变量时的 skip。

## Commands

~~~bash
go test ./internal/marketing ./internal/httpapi ./internal/worker ./internal/postgres
go test -race ./internal/marketing ./internal/httpapi ./internal/worker
go test -run 'Test.*Audit|Test.*Quota|Test.*DLQ|Test.*Outbox|Test.*Postgres' ./...
go test ./...
go build ./...
docker compose up -d postgres
docker compose exec -T postgres pg_isready -U exmarketing -d exmarketing
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./internal/postgres
~~~

## Result

~~~text
PASS
~~~

## Evidence

- `go test ./internal/marketing ./internal/httpapi ./internal/worker ./internal/postgres`
  - `ok github.com/ex-stack/ExMarketing/internal/marketing`
  - `ok github.com/ex-stack/ExMarketing/internal/httpapi`
  - `ok github.com/ex-stack/ExMarketing/internal/worker`
  - `ok github.com/ex-stack/ExMarketing/internal/postgres`
- `go test -race ./internal/marketing ./internal/httpapi ./internal/worker`
  - 全部通过，无 race 报告。
- `go test -run 'Test.*Audit|Test.*Quota|Test.*DLQ|Test.*Outbox|Test.*Postgres' ./...`
  - 全部匹配包通过；无失败测试。
- `go test ./...`
  - 全量通过；仅 `cmd/exmarketing`、`cmd/workflowctl`、`internal/mqtarget`、`internal/plugin` 显示 `[no test files]`。
- `go build ./...`
  - 通过，无输出。
- PostgreSQL smoke:
  - 初始环境中 `EX_MARKETING_DATABASE_URL` 未设置，Postgres integration tests 会 skip。
  - 已启动既有 `exmarketing-postgres` 容器，`pg_isready` 返回 `accepting connections`。
  - 设置 `EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable'` 后执行 `go test ./internal/postgres` 通过。

## Failure Details

无失败命令，无失败日志，无需打回 Dev Agent。

需求：2026-05-24-113929-9
