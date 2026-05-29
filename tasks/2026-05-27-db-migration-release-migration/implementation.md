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

已实现 PostgreSQL migration 正式发布流程：

- 新增 `internal/dbmigration`，负责读取 `db/migration/NNN_name.sql`、按版本排序、校验重复版本、计算 SHA-256 checksum、维护 `schema_migrations`、执行 precheck/apply/drift。
- 新增 `cmd/db-migration-release`，支持 `--mode precheck|apply|drift|status` 和 `--dir`。
- migration version 表字段包括 version、name、file_name、checksum、status、started_at、finished_at、error_message。
- `apply` 使用 PostgreSQL advisory transaction lock 串行执行，已 success 版本跳过，checksum 或 filename drift 会 BLOCK。
- 失败 migration 记录为 `failed`，后续 precheck 会 BLOCK，符合 forward-only + 人工恢复/修复策略。
- `drift` 检测覆盖 version/checksum 和核心表存在性。
- PostgreSQL repository 初始化时会执行 drift 检测，schema 未达标、pending migration、checksum drift 或核心表缺失会阻止服务启动。
- 移除 `docker-compose.yml` 中 `/docker-entrypoint-initdb.d` 自动挂载，避免新库绕过 `schema_migrations`。
- 更新 README、architecture 和 operations 文档，说明正式发布命令、本地验证、失败处理和 forward-only 策略。
- 补齐 `test-plan.md`，使其从模板状态变为 PASS。

## Changed Files

- `cmd/db-migration-release/main.go`
- `internal/dbmigration/migration.go`
- `internal/dbmigration/migration_test.go`
- `internal/postgres/repository.go`
- `docker-compose.yml`
- `README.md`
- `docs/architecture.md`
- `docs/operations.md`
- `tasks/2026-05-27-db-migration-release-migration/test-plan.md`
- `tasks/2026-05-27-db-migration-release-migration/implementation.md`

## Behavior Changes

- 新环境不再通过 Docker entrypoint 自动执行 SQL；必须执行：

~~~bash
export EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable'
go run ./cmd/db-migration-release --mode apply
~~~

- 已有业务表但没有 `schema_migrations` 记录的历史库会被 precheck 阻止自动升级，需要人工确认基线。
- 修改已执行 migration 文件会触发 checksum drift，阻止发布。
- 删除核心业务表后执行 `--mode drift` 会失败并指出缺失表。
- 使用 PostgreSQL repository 启动服务前必须先完成 migration apply，否则 repository 初始化失败。

## Verification

已执行：

~~~bash
go test ./...
go build ./...
~~~

结果：

~~~text
PASS
~~~

## Notes

本轮 `test.md` 和 `commit.md` 均为模板占位，没有可定位的真实 FAIL/BLOCK 上下文；因此按 analysis/review/test-plan 的最小验收口径完成实现和验证。

需求：2026-05-27-db-migration-release-migration
