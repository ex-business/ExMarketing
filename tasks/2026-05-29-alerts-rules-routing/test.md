# Test

## Scope

根据 `task.md`、`analysis.md`、`review.md`、`test-plan.md`、`implementation.md` 执行测试。

本轮只写入本文件：`tasks/2026-05-29-alerts-rules-routing/test.md`。未修改业务代码、测试代码、文档代码、配置文件或其他文件。

本次重点复测 `implementation.md` 中声明已修复的 PostgreSQL outbox smoke 失败点，并覆盖告警规则、Alertmanager 路由、字段完整性、敏感/高基数字段、Go 全量测试、构建和业务 smoke。

## Commands

```bash
docker run --rm --entrypoint promtool -v "$PWD:/work:ro" -w /work prom/prometheus:v2.55.1 check rules monitoring/alerts-exmarketing.rules.yml
docker run --rm --entrypoint promtool -v "$PWD:/work:ro" -w /work/monitoring prom/prometheus:v2.55.1 test rules alerts-exmarketing.test.yml
docker run --rm --entrypoint amtool -v "$PWD/monitoring:/etc/alertmanager:ro" prom/alertmanager:v0.27.0 check-config /etc/alertmanager/alertmanager-exmarketing.yml
go test ./...
go build ./...
ruby -e 'require "yaml"; ARGV.each { |f| YAML.load_file(f); puts "ok #{f}" }' monitoring/prometheus-exmarketing.yml monitoring/alerts-exmarketing.rules.yml monitoring/alertmanager-exmarketing.yml monitoring/alerts-exmarketing.test.yml
ruby -ryaml -e '<alert coverage / required field / forbidden label static assertions>'
ruby -ryaml -e '<alertmanager route / receiver / inhibition static assertions>'
go test ./internal/fact -run 'Test.*Metrics|Test.*Fact' -v
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go run ./cmd/db-migration-release --mode apply
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./internal/postgres -run 'Test.*Integration|Test.*Postgres' -count=1
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go run ./cmd/engine-e2e-postgres
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go run ./cmd/engine-load-fault-tests --profile local --scenario pg-smoke
go test ./internal/engine ./internal/marketing -run 'Test.*(Quota|Idempot|Duplicate|Concurrent|Concurrency|Outbox|Callback|Fact)' -count=1 -v
```

## Passed

- Prometheus rule 静态校验通过：
  - `SUCCESS: 11 rules found`
- Prometheus rule unit test 通过：
  - `docker run --rm --entrypoint promtool ... test rules alerts-exmarketing.test.yml`
  - 输出 `SUCCESS`
- Alertmanager 配置校验通过：
  - `SUCCESS`
  - found global config, route, 3 inhibit rules, 6 receivers, 1 templates.
- `go test ./...` 通过。
- `go build ./...` 通过。
- YAML 解析通过：
  - `monitoring/prometheus-exmarketing.yml`
  - `monitoring/alerts-exmarketing.rules.yml`
  - `monitoring/alertmanager-exmarketing.yml`
  - `monitoring/alerts-exmarketing.test.yml`
- 告警规则静态断言通过：
  - 11 条必需告警均存在：worker 全停、DB write failure、quota risk、DLQ growing、outbox backlog、callback fail rate、event intake、claim latency、reward retry、Fact Query fail rate、scrape target down。
  - 所有告警均包含 `service`、`severity`、`owner`、`tenant`、`channel`、`dashboard`、`runbook`。
  - 所有告警均包含 `summary`、`description`、`current_value`、`threshold`。
  - 未发现禁用敏感/高基数字段文本：`token`、`Authorization`、`Cookie`、`secret`、`password`、`targetRef`、`eventId`、`bizId`、`subjectId`、`userId`、`activityId`、`activity_id`、`billNo`、`commandId`、`idempotencyKey`。
- Alertmanager 静态断言通过：
  - 6 个预期 receiver 均存在。
  - 6 条 child route 引用的 receiver 均存在。
  - 3 条 inhibition 规则存在。
  - 包含 tenant-scoped DB P0 下游抑制、global DB P0 fallback、P0/P1 抑制同维度 P2/P3。
- Fact Query 指标测试通过：
  - `TestQueryRecordsPrometheusMetrics`
- DB migration apply 通过：
  - `APPLY PASS`
  - `applied=3 pending=0`
- PostgreSQL integration subset 通过：
  - `ok github.com/ex-stack/ExMarketing/internal/postgres 1.230s`
- PostgreSQL E2E smoke 通过：
  - `PASS runId=e2e_pg_20260530_093502_000000000`
  - success command 和 failed command 均完成。
- PostgreSQL load fault `pg-smoke` 通过：
  - `PASS runId=load_fault_20260530_093502_879546000`
  - `total=6 success=6 retry=0 dlq=0`
  - consistency: `event_inbox=6 grant_order=6 outbox=6 participation=6 quota_drift=0 result=PASS`
  - 前次失败的 `outbox sent=0, want=6 counts=map[SENDING:6]` 未复现。
- Quota / idempotency / duplicate / concurrency / outbox / callback / fact focused Go tests 通过，包含：
  - `TestMemoryAtomicGrantDoesNotDoubleReserveOnDuplicateInbox`
  - `TestMemoryCallbackFinalizesConfiguredLimitReservations`
  - `TestMemoryRepositoryRejectsDuplicateParticipationAndGrantIdempotencyKeys`
  - `TestMemoryConcurrentDuplicateGrantRequestOnlyProcessesOnce`
  - `TestMemoryConcurrentQuotaCompetitionDoesNotOverReserve`
  - `TestMemoryConcurrentQuotaCompetitionCoversAllQuotaTypes`
  - `TestSimulateEventQueriesFactsAndReportsCache`
  - `TestSimulateEventFactQueryFailureIsExplainedAndIsolated`

## Failed

无。

## Notes

- 本次复测覆盖了上轮 `test.md` 的两个失败命令：
  - `go run ./cmd/engine-e2e-postgres`
  - `go run ./cmd/engine-load-fault-tests --profile local --scenario pg-smoke`
- 两个命令均已通过，PostgreSQL outbox 卡在 `SENDING` 的失败现象未复现。
- `pg-smoke` 输出的复现命令：

```bash
EX_MARKETING_DATABASE_URL='<dsn>' EX_MARKETING_BASE_URL='' go run ./cmd/engine-load-fault-tests --profile local --scenario pg-smoke --run-id 'load_fault_20260530_093502_879546000' --seed '1780133702879583000'
```

## Result

```text
PASS
```

需求：2026-05-29-alerts-rules-routing
