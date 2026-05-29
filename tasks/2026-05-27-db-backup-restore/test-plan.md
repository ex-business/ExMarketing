# Test Plan

## Agent 指令

你是 Test Plan Agent。请在 Review PASS 后根据 task.md、analysis.md、review.md 设计测试用例。

只允许写 test-plan.md。
禁止修改业务代码、测试代码、文档代码、配置文件或任何其他文件。
禁止执行测试命令、构建命令、格式化命令或 git 命令。
你的职责只是写清楚后续 Test Agent 应执行的测试用例和命令清单。

## Test Strategy

覆盖三层验证：

1. 基础 Go 验证：确认新增备份恢复命令、helper 和既有模块没有破坏编译与单测。
2. 工具行为验证：确认备份文件命名、非空检查、失败输出脱敏、恢复目标保护、schema/migration 校验和核心表 row count 输出。
3. PostgreSQL 演练验证：用本地 Docker PostgreSQL 生成源库数据，备份到文件，恢复到隔离库，在恢复库上执行 `validate` 和 Event In -> Outbox smoke。

## Test Cases

| ID | 场景 | 步骤 | 期望 |
| --- | --- | --- | --- |
| T1 | Go 全量测试 | 执行 `go test ./...` | 全部 PASS。 |
| T2 | Go 全量构建 | 执行 `go build ./...` | 全部 PASS。 |
| T3 | 备份 helper 单测 | 覆盖 `internal/dbbackup` 单测 | `pg_dump` 参数不泄露密码；备份路径含库名和 UTC 时间戳；失败时删除半成品并脱敏输出。 |
| T4 | 源库准备 | 启动 Docker PostgreSQL，设置源库 DSN，执行 migration apply 和 `cmd/engine-e2e-postgres` | 源库具备 schema_migrations 和核心业务数据。 |
| T5 | 生成备份 | 执行 `go run ./cmd/db-backup-restore --mode backup --output-dir backups/db-restore-drills` | 输出 `BACKUP PASS`、备份文件路径、size、sha256、createdUTC 和脱敏 source；文件非空。 |
| T6 | 恢复目标保护 | 将 `EX_MARKETING_RESTORE_DATABASE_URL` 指向源库或非空库，执行 restore 且不加 `--force` | 命令失败，阶段为 `RESTORE`，提示同库或目标非空，DSN 脱敏。 |
| T7 | 隔离恢复 | 创建空的 `exmarketing_restore` 库，执行 restore | 输出 `RESTORE PASS`，恢复目标为脱敏 DSN。 |
| T8 | 恢复库校验 | 执行 `go run ./cmd/db-backup-restore --mode validate` | 输出 `VALIDATE PASS`；migration checksum 无 drift；核心表 row count 可见。 |
| T9 | 恢复后 smoke | 执行 `go run ./cmd/db-backup-restore --mode smoke` | 输出 `PASS runId=...`，恢复库上生成 event inbox、participation、grant_order、outbox 链路证据。 |
| T10 | 缺失 DSN / 空备份异常 | 分别清空 DSN 或传入空文件执行 backup/restore/validate | 命令非 0 退出，阶段明确且不静默通过。 |

## Required Commands

~~~bash
go test ./...
go build ./...
~~~

可选 PostgreSQL 演练命令：

~~~bash
docker compose up -d postgres
export EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable'
go run ./cmd/db-migration-release --mode apply
go run ./cmd/engine-e2e-postgres
go run ./cmd/db-backup-restore --mode backup --output-dir backups/db-restore-drills
docker compose exec -T postgres dropdb -U exmarketing --if-exists exmarketing_restore
docker compose exec -T postgres createdb -U exmarketing exmarketing_restore
export EX_MARKETING_RESTORE_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing_restore?sslmode=disable'
go run ./cmd/db-backup-restore --mode restore --backup-file backups/db-restore-drills/<backup>.dump
go run ./cmd/db-backup-restore --mode validate
go run ./cmd/db-backup-restore --mode smoke
~~~

## Data / Smoke

- Source smoke uses `cmd/engine-e2e-postgres` to seed a unique `runId`.
- Restore smoke uses `cmd/db-backup-restore --mode smoke`, which points `EX_MARKETING_DATABASE_URL` at `EX_MARKETING_RESTORE_DATABASE_URL`.
- Test evidence must retain backup path, file size, SHA-256, restore target DSN redacted, validation row counts, and final smoke `runId`.
- No evidence should include database password, token, secret, Authorization, Cookie, or full sensitive DSN.

## Decision

~~~text
PASS
~~~

需求：2026-05-27-db-backup-restore
