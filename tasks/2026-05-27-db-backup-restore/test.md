# Test

## Scope

根据 `task.md`、`analysis.md`、`review.md`、`test-plan.md`、`implementation.md` 执行 Test Agent 验证。

本轮只写入本文件；未修改业务代码、测试代码、文档代码、配置文件或其他文件。

## Test Cases

| ID | 结果 | 说明 |
| --- | --- | --- |
| T1 | PASS | `go test ./...` 全量通过。 |
| T2 | PASS | `go build ./...` 全量通过。 |
| T3 | PASS | `go test -v ./internal/dbbackup ./internal/dbmigration ./cmd/db-backup-restore -count=1 -timeout=60s` 通过，覆盖备份 helper、恢复保护、脱敏、Docker fallback 参数和 migration 校验单测。 |
| T4 | PASS | Docker PostgreSQL 启动、migration apply 成功，源库 `cmd/engine-e2e-postgres` seed 通过；额外连续执行 3 次均通过，旧的跨秒幂等不稳定失败未复现。 |
| T5 | PASS | `cmd/db-backup-restore --mode backup` 通过；宿主机未发现 `pg_dump` / `pg_restore`，命令 fallback 到 Docker PostgreSQL client，生成非空 custom dump。 |
| T6 | PASS | 恢复目标与源库相同、空备份文件、非空目标库保护均按预期失败，输出阶段明确且 DSN 脱敏。 |
| T7 | PASS | 备份成功恢复到隔离库 `exmarketing_restore`。 |
| T8 | PASS | 恢复库 `validate` 通过，输出 migration/schema 校验和核心表 row count。 |
| T9 | PASS | 恢复库 `smoke` 通过，输出恢复后 Event In -> Outbox 链路 runId；smoke 后再次 validate，核心表 row count 增长。 |
| T10 | PASS | 缺失 DSN、空备份文件等负向用例按预期非 0 退出。 |

## Commands

```bash
go test ./...
go build ./...
docker --version && docker compose version
command -v pg_dump || true; command -v pg_restore || true
docker compose up -d postgres
docker compose ps postgres
docker compose exec -T postgres pg_isready -U exmarketing -d exmarketing
docker compose exec -T postgres pg_dump --version && docker compose exec -T postgres pg_restore --version
EX_MARKETING_DATABASE_URL='<source-dsn-redacted>' go run ./cmd/db-migration-release --mode apply
EX_MARKETING_DATABASE_URL='<source-dsn-redacted>' go run ./cmd/engine-e2e-postgres
for i in $(seq 1 3); do EX_MARKETING_DATABASE_URL='<source-dsn-redacted>' go run ./cmd/engine-e2e-postgres || exit 1; done
EX_MARKETING_DATABASE_URL='<source-dsn-redacted>' go run ./cmd/db-backup-restore --mode validate --restore-database-url '<source-dsn-redacted>'
EX_MARKETING_DATABASE_URL='<source-dsn-redacted>' go run ./cmd/db-backup-restore --mode backup --output-dir /tmp
EX_MARKETING_DATABASE_URL='<source-dsn-redacted>' EX_MARKETING_RESTORE_DATABASE_URL='<same-source-dsn-redacted>' go run ./cmd/db-backup-restore --mode restore --backup-file /tmp/exmarketing_20260529T125544Z.dump
docker compose exec -T postgres dropdb -U exmarketing --if-exists exmarketing_restore
EX_MARKETING_DATABASE_URL='<source-dsn-redacted>' EX_MARKETING_RESTORE_DATABASE_URL='<restore-dsn-redacted>' go run ./cmd/db-backup-restore --mode restore --backup-file /dev/null --force
docker compose exec -T postgres createdb -U exmarketing exmarketing_restore
EX_MARKETING_DATABASE_URL='<source-dsn-redacted>' EX_MARKETING_RESTORE_DATABASE_URL='<restore-dsn-redacted>' go run ./cmd/db-backup-restore --mode restore --backup-file /tmp/exmarketing_20260529T125544Z.dump
EX_MARKETING_DATABASE_URL='<source-dsn-redacted>' EX_MARKETING_RESTORE_DATABASE_URL='<restore-dsn-redacted>' go run ./cmd/db-backup-restore --mode restore --backup-file /tmp/exmarketing_20260529T125544Z.dump
EX_MARKETING_RESTORE_DATABASE_URL='<restore-dsn-redacted>' go run ./cmd/db-backup-restore --mode validate
EX_MARKETING_RESTORE_DATABASE_URL='<restore-dsn-redacted>' go run ./cmd/db-backup-restore --mode smoke
EX_MARKETING_RESTORE_DATABASE_URL='<restore-dsn-redacted>' go run ./cmd/db-backup-restore --mode validate
env -u EX_MARKETING_DATABASE_URL -u EX_MARKETING_RESTORE_DATABASE_URL go run ./cmd/db-backup-restore --mode backup --output-dir /tmp
env -u EX_MARKETING_DATABASE_URL -u EX_MARKETING_RESTORE_DATABASE_URL go run ./cmd/db-backup-restore --mode validate
go test -v ./internal/dbbackup ./internal/dbmigration ./cmd/db-backup-restore -count=1 -timeout=60s
```

## Evidence

基础验证：

```text
go test ./...: PASS
go build ./...: PASS
go test -v ./internal/dbbackup ./internal/dbmigration ./cmd/db-backup-restore -count=1 -timeout=60s: PASS
```

Docker/PostgreSQL：

```text
Docker version 28.4.0, build d8eb465
Docker Compose version v2.39.2-desktop.1
host pg_dump: not found
host pg_restore: not found
container pg_dump: pg_dump (PostgreSQL) 16.11
container pg_restore: pg_restore (PostgreSQL) 16.11
exmarketing-postgres: Up, healthy, 0.0.0.0:54329->5432/tcp
pg_isready: /var/run/postgresql:5432 - accepting connections
```

Migration：

```text
APPLY PASS
applied=3 pending=0
applied 001 001_marketing_core.sql success
applied 002 002_audit_fact_outbox_indexes.sql success
applied 003 003_activity_unique_constraint.sql success
```

Source E2E seed：

```text
PASS runId=e2e_pg_20260529_125513_000000000 tenantId=site_001_e2e_pg_20260529_125513_000000000 activityId=act_f9fa81c0ec388c4a69b910df activityCode=e2e_pg_20260529_125513_000000000 successCommand=cmd_b64d33c7d1521702ac5d6120 failedCommand=cmd_a2826340bb5b98f50fc6444c
```

Source E2E repeat stability:

```text
PASS runId=e2e_pg_20260529_125522_000000000 ...
PASS runId=e2e_pg_20260529_125526_000000000 ...
PASS runId=e2e_pg_20260529_125531_000000000 ...
```

Source validate：

```text
VALIDATE PASS
marketing_activity rows=12
marketing_rule rows=12
marketing_reward rows=12
marketing_quota rows=72
marketing_event_inbox rows=23
marketing_participation rows=23
marketing_grant_order rows=23
marketing_outbox rows=23
marketing_audit rows=169
marketing_fact_provider rows=0
database=postgres://exmarketing:%5Bredacted%5D@localhost:54329/exmarketing?sslmode=disable
```

Backup：

```text
BACKUP PASS
path=/tmp/exmarketing_20260529T125544Z.dump
sizeBytes=64924
sha256=8e8e89129bbceaf55969d09e6a4818e2aaa7a698013e524a944fabb05d1227e2
createdUTC=2026-05-29T12:55:44Z
source=postgres://exmarketing:%5Bredacted%5D@localhost:54329/exmarketing?sslmode=disable
```

Restore safety checks:

```text
same source/target: FAIL as expected, RESTORE source and restore database are the same, DSN redacted
empty backup /dev/null: FAIL as expected, RESTORE backup file is empty
non-empty target: FAIL as expected, RESTORE target database is not empty, DSN redacted
```

Restore:

```text
RESTORE PASS
backupFile=/tmp/exmarketing_20260529T125544Z.dump
target=postgres://exmarketing:%5Bredacted%5D@localhost:54329/exmarketing_restore?sslmode=disable
force=false
```

Restore validate before smoke:

```text
VALIDATE PASS
marketing_activity rows=12
marketing_rule rows=12
marketing_reward rows=12
marketing_quota rows=72
marketing_event_inbox rows=23
marketing_participation rows=23
marketing_grant_order rows=23
marketing_outbox rows=23
marketing_audit rows=169
marketing_fact_provider rows=0
database=postgres://exmarketing:%5Bredacted%5D@localhost:54329/exmarketing_restore?sslmode=disable
```

Restore smoke:

```text
PASS runId=e2e_pg_20260529_125613_000000000 tenantId=site_001_e2e_pg_20260529_125613_000000000 activityId=act_4283a98599cb7d2bb2739aae activityCode=e2e_pg_20260529_125613_000000000 successCommand=cmd_6deeb2f81b092a9c8271cade failedCommand=cmd_98773a7a327a839b8341a54e
```

Restore validate after smoke:

```text
VALIDATE PASS
marketing_activity rows=13
marketing_rule rows=13
marketing_reward rows=13
marketing_quota rows=78
marketing_event_inbox rows=25
marketing_participation rows=25
marketing_grant_order rows=25
marketing_outbox rows=25
marketing_audit rows=183
marketing_fact_provider rows=0
database=postgres://exmarketing:%5Bredacted%5D@localhost:54329/exmarketing_restore?sslmode=disable
```

Negative DSN checks:

```text
missing source backup: FAIL as expected, BACKUP EX_MARKETING_DATABASE_URL is required
missing validate DSN: FAIL as expected, SCHEMA_CHECK connect : EX_MARKETING_DATABASE_URL is required
```

## Failure

本轮测试未发现失败。

上一轮 `cmd/engine-e2e-postgres` 重复事件跨秒导致的 `INVALID_IDEMPOTENCY_KEY` 失败，本轮通过首次 seed 和连续 3 次重复执行验证，未复现。

## Result

```text
PASS
```

需求：2026-05-27-db-backup-restore
