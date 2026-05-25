# Test

## Agent 指令

你是 Test Agent。已根据 `task.md`、`analysis.md`、`review.md`、`test-plan.md` 和 `implementation.md` 执行测试。

本轮只写入本文件，未修改业务代码、测试代码、文档代码或配置文件。

## Environment

- Workspace: `/Users/tong/Documents/GitHub/OracleBetX-Projects/ExMarketing`
- Go: `go version go1.25.4 darwin/arm64`
- PostgreSQL: Docker Compose service `postgres`
- PostgreSQL DSN: `postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable`

PostgreSQL 说明：

- 首次手工检查发现本地 Docker volume 是旧 schema，`marketing_activity` 缺少本需求新增的 `plugin_code` 列。
- 已按项目本地测试环境重建 PostgreSQL volume，使 `db/migration/001_marketing_core.sql` 重新初始化 schema。
- 重建后 `plugin_code` 字段 smoke 通过。

## Commands

```bash
docker compose up -d postgres
docker compose exec -T postgres pg_isready -U exmarketing -d exmarketing
go test ./...
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./...
go build ./...
go test -count=1 ./internal/engine -run 'TestPlugin|TestActivityPlugin' -v
go test -count=1 ./internal/marketing -run 'TestPrelaunch|TestValidate|Plugin' -v
go test -count=1 ./internal/rule ./internal/reward -v
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test -count=1 ./internal/postgres -run 'TestPostgres' -v
docker compose down -v
docker compose up -d postgres
docker compose exec -T postgres pg_isready -U exmarketing -d exmarketing
docker compose exec -T postgres psql -v ON_ERROR_STOP=1 -U exmarketing -d exmarketing -c "BEGIN; INSERT INTO marketing_activity (id, tenant_id, activity_code, activity_name, activity_type, status, trigger_events, start_time, end_time, timezone, priority, exclusive_group, stackable, plugin_code, version, config_json, created_at, updated_at) VALUES ('pg_plugin_column_smoke_act', 'tenant_pg_plugin_smoke', 'PG_PLUGIN_COLUMN_SMOKE', 'PG Plugin Column Smoke', 'LOTTERY', 'ACTIVE', '[\"DEPOSIT_SUCCESS\"]', now(), now() + interval '1 day', 'UTC', 1, '', false, 'LOTTERY_TEST', 1, '{}', now(), now()); SELECT id, plugin_code FROM marketing_activity WHERE id='pg_plugin_column_smoke_act'; ROLLBACK;"
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test -count=1 ./internal/postgres -run 'TestPostgres' -v
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test -count=1 ./...
go build ./...
```

## Evidence

基础测试：

```text
go test ./...
PASS
ok github.com/ex-stack/ExMarketing/internal/engine
ok github.com/ex-stack/ExMarketing/internal/httpapi
ok github.com/ex-stack/ExMarketing/internal/marketing
ok github.com/ex-stack/ExMarketing/internal/postgres
ok github.com/ex-stack/ExMarketing/internal/reward
ok github.com/ex-stack/ExMarketing/internal/rule
ok github.com/ex-stack/ExMarketing/internal/worker
ok github.com/ex-stack/ExMarketing/internal/workflow
```

PostgreSQL 全量测试：

```text
EX_MARKETING_DATABASE_URL=... go test -count=1 ./...
PASS
ok github.com/ex-stack/ExMarketing/internal/engine 1.041s
ok github.com/ex-stack/ExMarketing/internal/httpapi 1.316s
ok github.com/ex-stack/ExMarketing/internal/marketing 1.343s
ok github.com/ex-stack/ExMarketing/internal/postgres 2.003s
ok github.com/ex-stack/ExMarketing/internal/reward 2.059s
ok github.com/ex-stack/ExMarketing/internal/rule 1.725s
ok github.com/ex-stack/ExMarketing/internal/worker 1.589s
ok github.com/ex-stack/ExMarketing/internal/workflow 2.536s
```

构建：

```text
go build ./...
PASS
```

插件专项：

```text
TestActivityPluginContextFeedsRulesRewardsAndSnapshot PASS
TestActivityPluginUnknownCodeRejectedAtCreateAndRuntime PASS
TestValidatePrelaunchBlocksUnknownPluginCode PASS
TestValidatePrelaunchBlocksIncompatiblePluginCode PASS
TestValidatePrelaunchAllowsRegisteredCompatiblePlugin PASS
```

规则和奖励专项：

```text
go test -count=1 ./internal/rule ./internal/reward -v
PASS
```

PostgreSQL smoke：

```text
TestPostgresAtomicGrantRollsBackOnStockExceeded PASS
TestPostgresCreateInboxDetectsDuplicateAndConflict PASS
TestPostgresParticipationAndGrantIdempotencyUniqueConstraints PASS
TestPostgresAtomicGrantRollsBackWhenGrantOrderCreateFails PASS
TestPostgresAtomicGrantRollsBackWhenOutboxCreateFails PASS
```

PostgreSQL `plugin_code` 字段 smoke：

```text
BEGIN
INSERT 0 1
id                           | plugin_code
pg_plugin_column_smoke_act    | LOTTERY_TEST
ROLLBACK
```

架构边界检查：

```text
db/migration/001_marketing_core.sql only adds plugin_code to marketing_activity.
No plugin_state table was found.
No plugin state repository was found.
ActivityPlugin code path exposes context/intents through service snapshots and does not add a dedicated reward/outbox path.
```

## Coverage Mapping

- 普通无插件链路：`go test ./...`、HTTP/API/engine/reward/rule 现有套件通过。
- 插件上下文参与规则和奖励预览：`TestActivityPluginContextFeedsRulesRewardsAndSnapshot` 通过。
- 未知插件配置和运行时兜底：`TestActivityPluginUnknownCodeRejectedAtCreateAndRuntime` 通过。
- 上线前未知/不兼容/兼容插件校验：`internal/marketing` prelaunch 插件测试通过。
- 规则字段白名单和非法字段拒绝：`internal/rule`、`internal/marketing` 测试通过。
- PostgreSQL 事务、幂等、quota 回滚、grant/outbox 回滚：`internal/postgres` smoke 通过。
- PostgreSQL schema 持久化字段：`marketing_activity.plugin_code` 手工事务 smoke 通过。
- 无插件状态持久化：代码/迁移检查未发现插件状态表或插件状态仓储。

## Result

```text
PASS
```

需求：2026-05-24-113400-9
