# Test Plan

## Agent 指令

你是 Test Plan Agent。请在 Review PASS 后根据 task.md、analysis.md、review.md 设计测试用例。

只允许写 test-plan.md。
禁止修改业务代码、测试代码、文档代码、配置文件或任何其他文件。
禁止执行测试命令、构建命令、格式化命令或 git 命令。
你的职责只是写清楚后续 Test Agent 应执行的测试用例和命令清单。

## Test Strategy

本任务为高风险 PostgreSQL migration 发布流程。测试分三层：

- Go 单元测试：验证 migration 文件解析、排序、重复版本、checksum drift、失败状态和 pending 计算。
- 基础构建测试：必须执行 `go test ./...` 和 `go build ./...`。
- PostgreSQL 手工 smoke：在本地 Docker PostgreSQL 上验证 `precheck`、`apply`、重复 apply、`drift` 和关键失败场景。

## Test Cases

1. Migration 文件解析与校验
   - 执行 `go test ./internal/dbmigration -count=1 -v`。
   - 期望：按版本升序读取；非法文件名失败；重复版本失败；checksum drift、failed 记录和缺失本地文件均会 BLOCK。

2. 新环境一键初始化
   - 执行 `docker compose down -v` 后 `docker compose up -d postgres`。
   - 设置 `EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable'`。
   - 执行 `go run ./cmd/db-migration-release --mode apply`。
   - 期望：命令 PASS，`schema_migrations` 存在，所有 migration 记录为 `success`，核心业务表存在。

3. 重复执行幂等
   - 在 Case 2 后再次执行 `go run ./cmd/db-migration-release --mode apply`。
   - 期望：命令 PASS，pending 为 0，不重复执行已 success 的 migration。

4. 发布前检查和 drift 检测
   - 执行 `go run ./cmd/db-migration-release --mode precheck`。
   - 执行 `go run ./cmd/db-migration-release --mode drift`。
   - 期望：两者 PASS，输出 applied 数量和 pending=0。

5. checksum drift 失败
   - 在临时目录复制 `db/migration`，修改一个已应用 migration 文件内容。
   - 执行 `go run ./cmd/db-migration-release --mode precheck --dir <临时目录>`。
   - 期望：命令 FAIL，错误包含 `checksum drift`。

6. 核心表缺失 drift 失败
   - 在临时测试库或可丢弃库中删除一个核心表，例如 `DROP TABLE marketing_fact_provider;`。
   - 执行 `go run ./cmd/db-migration-release --mode drift`。
   - 期望：命令 FAIL，错误包含 `required tables missing` 和缺失表名。

7. 历史环境无 version 表 BLOCK
   - 在可丢弃库中只用旧 Docker init SQL 建出业务表，确保没有 `schema_migrations` 记录。
   - 执行 `go run ./cmd/db-migration-release --mode precheck`。
   - 期望：命令 FAIL，错误包含 `legacy schema detected without schema_migrations records`。

8. 失败 migration 停止
   - 在临时目录复制 migration，并追加一个更高版本的失败 SQL。
   - 执行 `go run ./cmd/db-migration-release --mode apply --dir <临时目录>`。
   - 期望：命令 FAIL，`schema_migrations` 记录失败版本为 `failed`，再次 `precheck` 会 BLOCK，不继续执行更高版本。

## Required Commands

~~~bash
go test ./...
go build ./...
~~~

建议 Test Agent 在有本地 PostgreSQL 时补跑：

~~~bash
docker compose up -d postgres
export EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable'
go run ./cmd/db-migration-release --mode precheck
go run ./cmd/db-migration-release --mode apply
go run ./cmd/db-migration-release --mode apply
go run ./cmd/db-migration-release --mode drift
~~~

## Data / Smoke

核心表 smoke 查询：

~~~bash
docker compose exec -T postgres psql -U exmarketing -d exmarketing -Atc "SELECT version, file_name, status FROM schema_migrations ORDER BY version;"
docker compose exec -T postgres psql -U exmarketing -d exmarketing -Atc "SELECT table_name FROM information_schema.tables WHERE table_schema='public' AND table_name IN ('marketing_activity','marketing_rule','marketing_reward','marketing_quota','marketing_event_inbox','marketing_participation','marketing_grant_order','marketing_outbox','marketing_audit','marketing_fact_provider') ORDER BY table_name;"
~~~

## Decision

~~~text
PASS
~~~

需求：2026-05-27-db-migration-release-migration
