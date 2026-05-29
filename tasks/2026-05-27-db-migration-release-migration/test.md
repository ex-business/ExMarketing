# Test

## Agent 指令

你是 Test Agent。请根据 task.md、analysis.md、review.md、test-plan.md 和 implementation.md 执行完整测试。

最低要求：

~~~bash
go test ./...
go build ./...
~~~

## Test Cases

1. Go 基础验证
   - `go test ./...`
   - `go build ./...`

2. 空库发布前检查与一键初始化
   - `docker compose down -v`
   - `docker compose up -d postgres`
   - `EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go run ./cmd/db-migration-release --mode precheck`
   - `EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go run ./cmd/db-migration-release --mode apply`

3. 重复 apply 幂等与 drift 正常路径
   - 再次执行 `go run ./cmd/db-migration-release --mode apply`
   - 执行 `go run ./cmd/db-migration-release --mode precheck`
   - 执行 `go run ./cmd/db-migration-release --mode drift`
   - 查询 `schema_migrations` 和核心表清单。

4. 已有环境幂等升级与数据保留
   - 新建 `exmarketing_upgrade` 测试库。
   - 使用临时 migration 目录只包含 `001_marketing_core.sql`，先 apply 到旧版本。
   - 插入 `marketing_activity(id='upgrade_activity')` 代表性业务数据。
   - 使用完整 `db/migration` 再 apply 到最新版本，并再次重复 apply。
   - 查询升级前插入数据仍存在。

5. checksum drift 负向用例
   - 新建 `exmarketing_checksum` 测试库并 apply 到最新。
   - 手工将 `schema_migrations.version=1` 的 checksum 改为 64 个 `0`。
   - 执行 `--mode precheck`，期望失败并包含 `checksum drift`。

6. 核心表缺失 drift 负向用例
   - 新建 `exmarketing_missing` 测试库并 apply 到最新。
   - 删除 `marketing_fact_provider`。
   - 执行 `--mode drift`，期望失败并包含 `required tables missing: marketing_fact_provider`。

7. legacy schema 无 version 表负向用例
   - 新建 `exmarketing_legacy` 测试库。
   - 直接执行 `db/migration/001_marketing_core.sql`，制造已有业务表但无 migration 记录的历史库。
   - 执行 `--mode precheck`，期望失败并包含 `legacy schema detected without schema_migrations records`。

8. 失败 migration 停止与 failed 记录负向用例
   - 新建 `exmarketing_failure` 测试库。
   - 临时复制完整 migration 目录，追加 `004_bad_failure.sql` 和 `005_should_not_run.sql`。
   - `004_bad_failure.sql` 引用不存在的表，执行 apply 期望失败。
   - 查询 `schema_migrations`，确认 004 为 `failed`。
   - 查询 `to_regclass('public.migration_should_not_run')` 和 `schema_migrations.version=5`，确认 005 未执行。
   - 再次执行 `--mode precheck --dir <bad_dir>`，期望失败并包含 `migration 004 is failed`。

## Commands

已执行：

~~~bash
go test ./...
go build ./...

docker compose down -v
docker compose up -d postgres
docker compose exec -T postgres pg_isready -U exmarketing -d exmarketing

EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go run ./cmd/db-migration-release --mode precheck
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go run ./cmd/db-migration-release --mode apply
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go run ./cmd/db-migration-release --mode apply
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go run ./cmd/db-migration-release --mode precheck
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go run ./cmd/db-migration-release --mode drift

docker compose exec -T postgres psql -U exmarketing -d exmarketing -Atc "SELECT version, file_name, status FROM schema_migrations ORDER BY version;"
docker compose exec -T postgres psql -U exmarketing -d exmarketing -Atc "SELECT table_name FROM information_schema.tables WHERE table_schema='public' AND table_name IN ('marketing_activity','marketing_rule','marketing_reward','marketing_quota','marketing_event_inbox','marketing_participation','marketing_grant_order','marketing_outbox','marketing_audit','marketing_fact_provider') ORDER BY table_name;"
~~~

负向和升级场景使用独立测试库执行：

~~~bash
exmarketing_upgrade
exmarketing_checksum
exmarketing_missing
exmarketing_legacy
exmarketing_failure
~~~

## Result

~~~text
PASS
~~~

所有必跑命令通过。负向用例均按预期失败并输出可定位错误，不属于测试失败。

## Evidence

`go test ./...` 结果：

~~~text
PASS
ok github.com/ex-stack/ExMarketing/internal/dbmigration
ok github.com/ex-stack/ExMarketing/internal/httpapi
ok github.com/ex-stack/ExMarketing/internal/postgres
ok github.com/ex-stack/ExMarketing/internal/workflow
其余包为 cached 或 no test files
~~~

`go build ./...` 结果：

~~~text
PASS
~~~

空库 precheck：

~~~text
PRECHECK PASS
applied=0 pending=3
pending 001 001_marketing_core.sql
pending 002 002_audit_fact_outbox_indexes.sql
pending 003 003_activity_unique_constraint.sql
~~~

空库 apply：

~~~text
APPLY PASS
applied=3 pending=0
applied 001 001_marketing_core.sql success
applied 002 002_audit_fact_outbox_indexes.sql success
applied 003 003_activity_unique_constraint.sql success
~~~

重复 apply、precheck、drift：

~~~text
APPLY PASS
applied=3 pending=0
PRECHECK PASS
applied=3 pending=0
DRIFT PASS
applied=3 pending=0
~~~

`schema_migrations` 查询：

~~~text
1|001_marketing_core.sql|success
2|002_audit_fact_outbox_indexes.sql|success
3|003_activity_unique_constraint.sql|success
~~~

核心表查询：

~~~text
marketing_activity
marketing_audit
marketing_event_inbox
marketing_fact_provider
marketing_grant_order
marketing_outbox
marketing_participation
marketing_quota
marketing_reward
marketing_rule
~~~

已有环境升级与数据保留：

~~~text
APPLY PASS
applied=1 pending=0

APPLY PASS
applied=3 pending=0

APPLY PASS
applied=3 pending=0

SELECT count(*) FROM marketing_activity WHERE id='upgrade_activity';
1
~~~

checksum drift 负向用例：

~~~text
ERROR: migration 001 checksum drift: database=0000000000000000000000000000000000000000000000000000000000000000 local=395b5fc3c01034fe60089f1cb47c332e2d602d46fa92b94a9e5d4e14d55e78e8
EXPECTED checksum precheck failure
~~~

核心表缺失 drift 负向用例：

~~~text
ERROR: required tables missing: marketing_fact_provider
EXPECTED missing-table drift failure
~~~

legacy schema 无 version 表负向用例：

~~~text
ERROR: legacy schema detected without schema_migrations records: marketing_activity, marketing_event_inbox, marketing_grant_order, marketing_outbox, marketing_participation, marketing_quota, marketing_reward, marketing_rule; create a reviewed baseline before applying migrations
EXPECTED legacy precheck failure
~~~

失败 migration 停止负向用例：

~~~text
ERROR: apply migration 004_bad_failure.sql: ERROR: relation "missing_table_for_migration_failure" does not exist (SQLSTATE 42P01)
EXPECTED failing migration apply failure

1|001_marketing_core.sql|success|
2|002_audit_fact_outbox_indexes.sql|success|
3|003_activity_unique_constraint.sql|success|
4|004_bad_failure.sql|failed|ERROR: relation "missing_table_for_migration_failure" does not exist (SQLSTATE 4

to_regclass('public.migration_should_not_run') = <null>
to_regclass('public.migration_failure_marker') = <null>
schema_migrations version=5 count = 0

ERROR: migration 004 is failed: ERROR: relation "missing_table_for_migration_failure" does not exist (SQLSTATE 42P01)
EXPECTED failed-record precheck failure
~~~

## Notes

- 本轮只写入 `tasks/2026-05-27-db-migration-release-migration/test.md`。
- PostgreSQL smoke 使用 `docker compose down -v` 重建本地测试数据库，并额外创建独立测试库覆盖升级和负向场景。
- 未发现需要打回 Dev Agent 的真实失败。

需求：2026-05-27-db-migration-release-migration
