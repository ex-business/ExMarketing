# Test

## Agent 指令

Test Agent 已按 `task.md`、`analysis.md`、`review.md`、`test-plan.md`、`implementation.md` 执行验证。

本轮只写入本文件：`tasks/2026-05-27-rule-formula-script-limit/test.md`。

## Test Cases

已执行测试覆盖：

- `FORMULA` evaluator 核心单元测试：Decimal 公式、Boolean 缺省 compare、运行时错误、除以 0、不安全表达式和结构化拒绝。
- `marketing` 创建 / 上线前校验：安全 `FORMULA` 允许，非法字段、函数调用、缺失 compare、未支持 resultType、Boolean 非法 op、scale 越界拒绝。
- `engine` / `httpapi` 相邻链路回归：规则执行、活动处理、HTTP handler、幂等 / quota / duplicate 相关现有用例。
- 全仓库回归：所有 Go package 测试、race 定向测试和全量 build。
- PostgreSQL smoke：使用 `EX_MARKETING_DATABASE_URL=postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable` 验证 postgres 仓储、HTTP 和 engine 测试；额外执行 `-tags=integration` 全量测试。

说明：

- `docker compose up -d postgres` 执行时发现 `exmarketing-postgres` 已经是运行且 healthy 状态，容器已存在 2 天。
- 因该 PostgreSQL 容器是测试前已存在的用户环境，本轮未执行 `docker compose down`，避免停止已有服务。

## Commands

~~~bash
go test ./internal/rule
go test ./internal/marketing
go test ./internal/engine
go test ./internal/httpapi
go test ./internal/decimal
go test ./...
go test ./... -run 'Formula|FORMULA|Script|SCRIPT|Limit|LIMIT|Evaluator|Rule'
go test ./... -run 'Decimal|Boolean|Compare|Field|Provider|Plugin|Unsafe|Divide|Idempot|Duplicate|Quota|Budget|Stock|Concurrent'
go test -race ./internal/rule ./internal/engine ./internal/httpapi
go build ./...
docker compose up -d postgres
docker compose ps postgres
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./internal/postgres
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test -count=1 ./internal/httpapi ./internal/engine
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test -tags=integration ./...
~~~

## Result

~~~text
PASS
~~~

## Evidence

关键结果摘要：

- `go test ./internal/rule`: PASS
- `go test ./internal/marketing`: PASS
- `go test ./internal/engine`: PASS
- `go test ./internal/httpapi`: PASS
- `go test ./internal/decimal`: PASS
- `go test ./...`: PASS
- FORMULA / SCRIPT / LIMIT / Evaluator / Rule 定向筛选: PASS
- Decimal / Boolean / Compare / Field / Provider / Plugin / Unsafe / Divide / Idempot / Duplicate / Quota / Budget / Stock / Concurrent 定向筛选: PASS
- `go test -race ./internal/rule ./internal/engine ./internal/httpapi`: PASS
- `go build ./...`: PASS
- `docker compose ps postgres`: `exmarketing-postgres` healthy, port `54329->5432`
- PostgreSQL smoke `go test ./internal/postgres`: PASS
- PostgreSQL smoke `go test -count=1 ./internal/httpapi ./internal/engine`: PASS
- Integration tag `go test -tags=integration ./...`: PASS

未发现失败命令、失败日志或需打回 Dev Agent 的问题。

需求：2026-05-27-rule-formula-script-limit
