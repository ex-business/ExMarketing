# Test

## Agent 指令

你是 Test Agent。已根据 task.md、analysis.md、review.md、test-plan.md 和 implementation.md 执行测试。

本轮只修改本文件。

## Test Cases

- TP-01 Review Gate: PASS。`review.md` Decision 为 PASS，且无 P0/P1 Finding。
- TP-02 威胁模型覆盖完整性: PASS。`docs/security-threat-model.md` 覆盖 OpenAPI、Admin、Webhook Receiver、Reward Callback、Fact Provider、MQ、Outbox、DLQ、导出文件、备份文件。
- TP-03 高危资产覆盖完整性: PASS。资产清单覆盖 activity config、quota/reservation、grant_order、outbox/DLQ、audit、OpenAPI token、webhook/callback secret、Fact Provider authRef、MQ credential/TLS refs、export、backup、metrics/logs/runbooks。
- TP-04 数据流和信任边界: PASS。Mermaid 图和边界说明覆盖公网入口、管理入口、服务、DB、MQ、Fact Provider、外部发奖系统、导出/备份、监控。
- TP-05 STRIDE 风险矩阵字段: PASS。风险矩阵包含入口/资产、STRIDE、攻击路径、影响、现有控制、缺口、严重度、可能性、风险、补救/证据。
- TP-06 P0/P1 后续任务: PASS。所有 P0/P1 风险均在 Follow-Up Security Tasks 中拆出可执行任务和验收口径。
- TP-07 Review P2 补强项: PASS。文档明确 nonce 未文档化时的 timestamp/idempotency/终态冲突替代控制，并覆盖 redirect、DNS rebinding、proxy env SSRF 子场景。
- TP-08/09/10/11/12 回归与 PostgreSQL smoke: PASS。全量 Go 测试、构建、干净 PostgreSQL race、migration、engine e2e smoke 均通过。
- TP-13 敏感信息和低基数指标: PASS。后台启动本地服务，健康检查通过，`/metrics` 包含预期指标族，未匹配 forbidden sensitive/high-cardinality 字段。
- TP-14 Admin / DLQ 高危操作审计: PASS by existing tests/docs review。现有测试覆盖 DLQ 状态限制、callback/tenant/quota 并发；威胁模型要求 operator/reason/audit/business confirmation。
- TP-15 MQ / targetRef / Fact Provider 边界: PASS by existing package tests/docs review。`internal/mqtarget`、`internal/fact`、`internal/worker` 测试通过；威胁模型补充生产 allowlist/SSRF 后续任务。

## Commands

~~~bash
go test ./...
# PASS

go build ./...
# PASS

command -v promtool || true
command -v alertmanager || true
command -v docker || true
# /opt/homebrew/bin/promtool
# /opt/homebrew/bin/alertmanager
# /usr/local/bin/docker

promtool check config monitoring/prometheus-exmarketing.yml
# PASS: valid prometheus config, 1 rule file, 11 rules found

promtool test rules monitoring/alerts-exmarketing.test.yml
# SUCCESS

alertmanager --config.file=monitoring/alertmanager-exmarketing.yml --config.check
# SUCCESS

docker compose up -d postgres
docker compose exec -T postgres pg_isready -U exmarketing -d exmarketing
# /var/run/postgresql:5432 - accepting connections

EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' \
  go run ./cmd/db-migration-release --mode apply
# APPLY PASS, applied=3, pending=0

EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' \
  go test -count=1 ./internal/postgres
# PASS

EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' \
  go run ./cmd/engine-e2e-postgres
# PASS runId=e2e_pg_20260530_124048_000000000 tenantId=site_001_e2e_pg_20260530_124048_000000000 activityId=act_ac60f9eab6ddb275f68a1915 activityCode=e2e_pg_20260530_124048_000000000 successCommand=cmd_3cc5d9fd067a26b2e65b1239 failedCommand=cmd_e06bc6888f8f826af755ba31

DB=exmarketing_test_security_$(date +%Y%m%d%H%M%S)
docker compose exec -T postgres createdb -U exmarketing "$DB"
EX_MARKETING_DATABASE_URL="postgres://exmarketing:exmarketing@localhost:54329/$DB?sslmode=disable" \
  go run ./cmd/db-migration-release --mode apply
EX_MARKETING_DATABASE_URL="postgres://exmarketing:exmarketing@localhost:54329/$DB?sslmode=disable" \
  go test -race ./internal/postgres
# PASS

EX_MARKETING_DATABASE_URL="postgres://exmarketing:exmarketing@localhost:54329/$DB?sslmode=disable" \
  go test -race ./...
# PASS

EX_MARKETING_ADDR='127.0.0.1:18080' \
EX_MARKETING_DATABASE_URL="postgres://exmarketing:exmarketing@localhost:54329/$DB?sslmode=disable" \
  go run ./cmd/exmarketing >/tmp/exmarketing-security-service.log 2>&1 &
PID=$!
echo "$PID" >/tmp/exmarketing-security-service.pid
trap 'kill "$PID" 2>/dev/null || true; wait "$PID" 2>/dev/null || true' EXIT
curl -fsS http://127.0.0.1:18080/healthz
curl -fsS http://127.0.0.1:18080/metrics >/tmp/exmarketing-security.metrics
rg 'exmarketing_(event_intake|rule_execution|reward_execution|quota_attempt|quota_amount|outbox_backlog|reward_callback|worker_claim|db_operation)' /tmp/exmarketing-security.metrics
rg 'Authorization|Cookie|token|secret|password|payload|targetRef|eventId|bizId|subjectId|userId|activityId|billNo|commandId|idempotencyKey' /tmp/exmarketing-security.metrics && exit 1 || true
# PASS, service_pid=904, metrics_file=/tmp/exmarketing-security.metrics, 88 lines

docker compose exec -T postgres dropdb -U exmarketing --if-exists "$DB"
docker compose stop postgres
# Cleanup complete
~~~

## Non-Blocking Notes

Initial `go test -race ./...` before starting PostgreSQL failed because `localhost:54329` was not accepting connections for `internal/postgres` integration tests.

After starting PostgreSQL against the shared default `exmarketing` database, one race run failed in an outbox concurrent claim assertion:

~~~text
--- FAIL: TestPostgresOutboxStaleRecoveryConcurrentClaim
repository_integration_test.go:561: expected one stale recovery, got [outbox_1928d20abf841f38362c1486 outbox_089e03817fce0a2b962d0d51]
~~~

A later all-package race run against the same shared database failed similarly:

~~~text
--- FAIL: TestPostgresOutboxConcurrentClaimSingleMessage
repository_integration_test.go:442: expected one worker to claim the outbox once, got [pg_callback_tenant_a_outbox pg_outbox_claim_single_outbox]
~~~

Failure cause: the shared local PostgreSQL database contained records from prior tests/smoke, causing outbox claim tests that assume an isolated database to observe extra eligible outbox rows. Reproduction: run PostgreSQL integration/e2e smoke against the default `exmarketing` DB, then run `EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test -race ./...`.

Control verification: creating a fresh temporary database, applying migrations, then running `go test -race ./internal/postgres` and `go test -race ./...` both passed. This is recorded as a local test isolation issue, not a Dev Agent code failure for this documentation-only task.

## Result

~~~text
PASS
~~~

## Evidence

- Threat model document inspected: `docs/security-threat-model.md`
- Metrics capture: `/tmp/exmarketing-security.metrics`
- Service PID during metrics smoke: `/tmp/exmarketing-security-service.pid` recorded `904`; process was cleaned by trap.
- PostgreSQL temp DB used for clean race: `exmarketing_test_security_*`; dropped after test.
- Docker PostgreSQL started for tests and stopped at end.

需求：2026-05-29-security-threat-model
