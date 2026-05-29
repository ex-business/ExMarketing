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

已补齐 PostgreSQL 备份恢复演练入口 `cmd/db-backup-restore`，覆盖：

- `--mode backup`：使用 `pg_dump --format=custom --no-owner --no-acl` 生成 schema + data 备份，文件名包含数据库名和 UTC 时间戳，输出路径、大小、SHA-256 和脱敏源 DSN。
- `--mode restore`：使用 `pg_restore --clean --if-exists --no-owner --no-acl` 恢复到隔离目标库，默认拒绝源库与目标库相同、目标库非空、备份文件缺失或为空；只有显式 `--force` 才允许覆盖本地演练目标。
- `--mode validate`：复用 migration release drift 检查，校验 `schema_migrations` checksum 和核心表存在性，并输出核心表 row count。
- `--mode smoke`：将 `EX_MARKETING_DATABASE_URL` 指向恢复目标库并运行 `cmd/engine-e2e-postgres`，验证恢复后 Event In -> Outbox 核心链路。

修复过程中发现并修正脱敏 helper 的死循环问题：脱敏输出写入 `[redacted]` 后继续从当前位置向后扫描，避免反复匹配同一 marker。

## Rework Fix

根据 `test.md` 的 FAIL 反馈，本轮做最小修复：

### 2026-05-29 engine-e2e-postgres 稳定性返工

- `cmd/engine-e2e-postgres` 的重复事件验证原来会为同一个幂等 key 重新生成 `occurredAt`；当两次请求跨秒时，payload 变化会触发服务端正确返回 `INVALID_IDEMPOTENCY_KEY`。
- 本轮将每次 runner 的事件时间固定为 `eventTime`，同一 run 内首次事件和重复事件使用完全一致的 `occurredAt`，保持幂等 key 与 payload 一致。
- 修正失败诊断 SQL：每条诊断查询只传入自身需要的参数，避免 `expected ... arguments`；`marketing_audit` 字段改为实际存在的 `payload_json`；inbox/outbox 查询改用明确参数类型，避免诊断路径再报 SQL 错。

### 2026-05-29 pg_dump / pg_restore fallback 返工

- `cmd/db-backup-restore --mode backup` 仍优先调用宿主机 `pg_dump`。
- 当宿主机 PATH 找不到 `pg_dump` / `pg_restore` 时，自动 fallback 到本仓库 `docker compose exec -T postgres` 容器内执行对应 PostgreSQL client。
- fallback 执行时继续通过 `PGPASSWORD` 传递密码，不把 URL 密码放进命令参数。
- fallback 会把本地演练 DSN 中的 `localhost:54329` / `127.0.0.1:54329` 改写为容器内 `localhost:5432`。
- `pg_dump --file <host-path>` 在 fallback 中改为容器 stdout 流式写回宿主机备份文件，避免生成到容器内部。
- `pg_restore <host-backup-file>` 在 fallback 中改为从宿主机文件 stdin 输入给容器内 `pg_restore`，避免容器读取不到宿主机路径。

## Changed Files

- `cmd/db-backup-restore/main.go`
- `cmd/engine-e2e-postgres/main.go`
- `internal/dbbackup/backup.go`
- `internal/dbbackup/backup_test.go`
- `internal/dbmigration/migration.go`
- `README.md`
- `docs/operations.md`
- `tasks/2026-05-27-db-backup-restore/test-plan.md`
- `tasks/2026-05-27-db-backup-restore/implementation.md`

## Behavior Changes

- 新增数据库备份、恢复、校验、恢复后 smoke 的统一命令入口。
- `pg_dump` / `pg_restore` 命令参数不直接携带 URL 密码；URL 密码会转为 `PGPASSWORD` 环境变量。
- 失败输出按 `BACKUP`、`RESTORE`、`SCHEMA_CHECK`、`DATA_CHECK`、`EVENT_IN_SMOKE` 等阶段返回，且会脱敏 DSN、password、token、secret、Authorization。
- `internal/dbmigration.RequiredTables()` 导出核心表清单，供 migration drift 与恢复校验共用，避免两套验收表列表漂移。
- 本地缺少 `pg_dump` / `pg_restore` 时，备份恢复演练可复用 Docker Compose postgres 容器内置 client，解决 Test Agent 反馈的宿主机 PATH 缺失问题。
- PostgreSQL E2E helper 的重复事件请求在跨秒执行时不再产生 payload conflict。
- PostgreSQL E2E helper 失败时的数据库诊断不再因为参数数量、audit 字段名或参数类型错误而掩盖原始失败。

## Verification

已执行并通过：

~~~bash
go test ./internal/dbbackup -count=1
docker compose up -d postgres
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go run ./cmd/db-backup-restore --mode backup --output-dir /tmp
docker compose exec -T postgres dropdb -U exmarketing --if-exists exmarketing_restore
docker compose exec -T postgres createdb -U exmarketing exmarketing_restore
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' EX_MARKETING_RESTORE_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing_restore?sslmode=disable' go run ./cmd/db-backup-restore --mode restore --backup-file /tmp/exmarketing_20260529T124632Z.dump
EX_MARKETING_RESTORE_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing_restore?sslmode=disable' go run ./cmd/db-backup-restore --mode validate
go test -v ./internal/dbbackup ./internal/dbmigration ./cmd/db-backup-restore -count=1 -timeout=60s
~~~

随后按任务要求执行：

~~~bash
go test ./...
go build ./...
~~~

结果：全部 PASS。

关键验证结果：

- 原失败命令 `--mode backup --output-dir /tmp` 已输出 `BACKUP PASS`，生成 `/tmp/exmarketing_20260529T124632Z.dump`，size `39101`，sha256 `30ad137c9b9ecc30385758668533cffeb7b23cf548668449cbfe37f68056447b`。
- 使用同一备份恢复到 `exmarketing_restore` 输出 `RESTORE PASS`。
- 恢复库 `validate` 输出 `VALIDATE PASS`，核心表 row count 可见。
- `go test ./...` PASS。
- `go build ./...` PASS。

本轮返工后补充执行：

~~~bash
go test ./...
go build ./...
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go run ./cmd/db-migration-release --mode apply
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go run ./cmd/engine-e2e-postgres
for i in $(seq 1 3); do EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go run ./cmd/engine-e2e-postgres || exit 1; done
~~~

结果：全部 PASS。连续 E2E helper 输出的 runId 包括：

- `e2e_pg_20260529_125317_000000000`
- `e2e_pg_20260529_125327_000000000`
- `e2e_pg_20260529_125331_000000000`
- `e2e_pg_20260529_125335_000000000`

## Notes

- `test.md` 当前最新实际 FAIL 原因：`cmd/engine-e2e-postgres` 重复事件跨秒后同幂等 key payload 冲突，以及失败诊断 SQL 有误。本轮已优先按该原因修复。
- `commit.md` 当前仍为模板态，没有实际 `BLOCK` / `FAILED` 提交失败上下文。
- 完整 PostgreSQL 备份恢复演练仍需要 Docker PostgreSQL 可用；宿主机 PostgreSQL client 不再是本地演练硬依赖。

需求：2026-05-27-db-backup-restore
