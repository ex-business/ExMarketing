# Test

## Scope

Test Agent 按 `task.md`、`analysis.md`、`review.md`、`test-plan.md`、`implementation.md` 执行验证。

本次仅写入本文件；未修改业务代码、测试代码、文档代码、配置文件或其他文件。

## Result

```text
PASS
```

理由：静态文档验收、告警规则到 runbook 映射、Prometheus 配置、告警规则样例、Alertmanager 配置、Go 测试、Go 构建、本地 `/metrics` smoke、PostgreSQL 表/聚合查询 smoke、PostgreSQL E2E 正常路径均通过。

## Static Validation

### PASS: operations 入口可发现

命令：

```bash
rg -n "SLI|SLO|SLA|演练|复盘|业务确认|runbook|mapping|映射" docs/operations.md docs/runbooks
sed -n '1,260p' docs/operations.md
```

结果摘要：

- `docs/operations.md` 明确指向 `docs/runbooks/exmarketing-alerts.md`。
- 可从运维手册定位告警公共字段与升级、告警 runbook、SLI/SLO/SLA、业务确认边界、演练与复盘模板、告警映射表。

### PASS: P1/P2 和核心告警 runbook 覆盖

命令：

```bash
rg -n "ExMarketing(DLQGrowing|OutboxBacklogHigh|CallbackFailRateHigh|EventIntakeErrorRateHigh|ClaimLatencyHigh|RewardRetrySpike|FactQueryFailRateHigh)" docs monitoring tasks/2026-05-29-alerts-runbook-slo-runbook-slo
rg -n "## ExMarketing|## DB Slow|## SLI|## Drill|## Incident|## Alert To Runbook|Business Confirmation|Escalation" docs/runbooks/exmarketing-alerts.md
```

结果摘要：

- P1/P2 告警均有 runbook 小节：`ExMarketingDLQGrowing`、`ExMarketingOutboxBacklogHigh`、`ExMarketingCallbackFailRateHigh`、`ExMarketingEventIntakeErrorRateHigh`、`ExMarketingClaimLatencyHigh`、`ExMarketingRewardRetrySpike`、`ExMarketingFactQueryFailRateHigh`。
- P0/P3 核心告警也有覆盖：`ExMarketingWorkerAllStopped`、`ExMarketingDBWriteFailure`、`ExMarketingQuotaOverDeductRisk`、`ExMarketingScrapeTargetDown`。
- 任务范围中的 DB 慢/错误通过 `DB Slow Or Error Scenario` 覆盖。

### PASS: 告警规则到 runbook 映射一致

命令：

```bash
ruby -e 'rules=File.read("monitoring/alerts-exmarketing.rules.yml").scan(/alert: (ExMarketing\w+)[\s\S]*?runbook: '\''([^'\'']+)'\''/).to_h; doc=File.read("docs/runbooks/exmarketing-alerts.md"); missing=rules.select{|a,r| !doc.include?("## #{a}") || !doc.include?(a.downcase)}; puts "alerts=#{rules.size}"; rules.each{|a,r| puts "#{a} -> #{r} -> doc_section=#{doc.include?("## #{a}")}"}; exit(missing.empty? ? 0 : 1)'
```

结果摘要：

- 共提取 11 个 alert。
- 11 个 alert 的 `runbook` label 都指向 `docs/runbooks/exmarketing-alerts.md#...`。
- 11 个 alert 在 runbook 中均有同名小节和映射表 anchor。

### PASS: SLO 指标口径和低流量边界

命令：

```bash
rg -n "exmarketing_(event_intake_total|reward_command_latency_seconds|reward_callback_total|outbox_dlq_count|outbox_dlq_total|quota_fail_total|db_operation_total|worker_claim_latency_seconds|fact_query_total)|rate\(|increase\(|minimum failure|最小失败|low-volume|low" docs/runbooks/exmarketing-alerts.md docs/operations.md
```

结果摘要：

- SLO 覆盖事件接入成功率、发奖指令投递延迟、callback 成功率、DLQ 清理时效、数据一致性、worker claim 延迟、Fact Query 成功率。
- Counter 口径使用 `rate` 或 `increase`。
- 文档说明低流量失败率需要最小失败数边界。

### PASS: 敏感信息与高基数字段约束

命令：

```bash
rg -n "token|Authorization|Cookie|secret|password|idempotencyKey|commandId|billNo|targetRef|payload" docs/operations.md docs/runbooks
```

结果摘要：

- `docs/runbooks/exmarketing-alerts.md` 明确禁止把 token、Authorization、Cookie、secret、password、完整 payload、完整 targetRef、eventId、bizId、subjectId、userId、activityId、billNo、commandId、idempotencyKey、原始 provider 错误文本写入指标标签或公开事故材料。
- 命中的 `docs/operations.md` 包含禁用说明、既有配置示例和排查章节；未发现本次 runbook 要求把敏感字段加入指标标签。

## Config Validation

### PASS: Prometheus config

命令：

```bash
promtool check config monitoring/prometheus-exmarketing.yml
```

结果摘要：

```text
SUCCESS: monitoring/prometheus-exmarketing.yml is valid prometheus config file syntax
SUCCESS: 11 rules found
```

### PASS: Prometheus alert rule tests

命令：

```bash
promtool test rules monitoring/alerts-exmarketing.test.yml
```

结果摘要：

```text
SUCCESS
```

### PASS: Alertmanager config

命令：

```bash
alertmanager --config.file=monitoring/alertmanager-exmarketing.yml --config.check
amtool check-config monitoring/alertmanager-exmarketing.yml
```

结果摘要：

```text
SUCCESS
Found: global config, route, 3 inhibit rules, 6 receivers, 1 templates
```

## Go Validation

### PASS: go test

命令：

```bash
go test ./...
```

结果摘要：

- 通过。
- 覆盖包包括 `internal/httpapi`、`internal/engine`、`internal/postgres`、`internal/reward`、`internal/worker`、`internal/workflow`、`internal/fact` 等。

### PASS: go build

命令：

```bash
go build ./...
```

结果摘要：

- 通过，无错误输出。

## API Smoke

### PASS: 当前代码 `/healthz` 和 `/metrics`

8080 已有旧 `exmarketing` 进程监听，因此按 `test-plan.md` 使用 18080 启动当前代码：

```bash
EX_MARKETING_ADDR=:18080 go run ./cmd/exmarketing
curl -fsS --max-time 2 http://127.0.0.1:18080/healthz
curl -fsS --max-time 2 http://127.0.0.1:18080/metrics | head -n 40
curl -fsS --max-time 2 http://127.0.0.1:18080/metrics | rg "exmarketing_(event_intake_total|reward_command_latency_seconds|reward_callback_total|outbox_dlq|quota_fail_total|worker_claim_latency_seconds|fact_query_total)"
```

结果摘要：

- `/healthz` 返回 `{"status":"ok"}`。
- `/metrics` 暴露 Prometheus text format。
- 关键 SLO 指标名均存在：`exmarketing_event_intake_total`、`exmarketing_reward_command_latency_seconds`、`exmarketing_reward_callback_total`、`exmarketing_outbox_dlq_count`、`exmarketing_outbox_dlq_total`、`exmarketing_quota_fail_total`、`exmarketing_worker_claim_latency_seconds`、`exmarketing_fact_query_total`。
- 临时 18080 服务已停止。

## PostgreSQL Smoke

### PASS: migration apply

命令：

```bash
export EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable'
go run ./cmd/db-migration-release --mode apply
```

结果摘要：

```text
APPLY PASS
applied=3 pending=0
```

### PASS: runbook 引用表和聚合查询

命令：

```bash
docker compose exec -T postgres psql -U exmarketing -d exmarketing -c "\dt"
docker compose exec -T postgres psql -U exmarketing -d exmarketing -c "select send_status, count(*) from marketing_outbox group by send_status order by send_status;"
docker compose exec -T postgres psql -U exmarketing -d exmarketing -c "select grant_status, count(*) from marketing_grant_order group by grant_status order by grant_status;"
docker compose exec -T postgres psql -U exmarketing -d exmarketing -c "select action, count(*) from marketing_audit where created_at > now() - interval '1 day' group by action order by action;"
docker compose exec -T postgres psql -U exmarketing -d exmarketing -c "select quota_type, scope, count(*) from marketing_quota_reservation group by quota_type, scope order by quota_type, scope limit 20;"
```

结果摘要：

- 核心表存在：`marketing_activity`、`marketing_audit`、`marketing_event_inbox`、`marketing_fact_provider`、`marketing_grant_order`、`marketing_outbox`、`marketing_participation`、`marketing_quota`、`marketing_quota_reservation`、`marketing_reward`、`marketing_rule` 等。
- `marketing_outbox` 可按 `send_status` 聚合，当前本地测试库含 `DLQ`、`FAILED`、`SENT`。
- `marketing_grant_order` 可按 `grant_status` 聚合，当前本地测试库含 `FAILED`、`RETRY`、`SUCCESS`。
- `marketing_audit` 可按 `action` 聚合，当前本地测试库含 `EVENT_IN`、`OUTBOX_ENTER_DLQ`、`GRANT_FAILED` 等。
- `marketing_quota_reservation` 可按 `quota_type`、`scope` 聚合，当前本地测试库含 `BUDGET/ACTIVITY`、`STOCK/ACTIVITY`、`USER_LIMIT/USER`、`DAILY_LIMIT/USER`。

### PASS: PostgreSQL E2E 正常路径

命令：

```bash
export EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable'
go run ./cmd/engine-e2e-postgres
```

结果摘要：

```text
PASS runId=e2e_pg_20260530_100536_000000000 tenantId=site_001_e2e_pg_20260530_100536_000000000 activityId=act_c2b852b2ac665b41e0f0d16f activityCode=e2e_pg_20260530_100536_000000000 successCommand=<redacted> failedCommand=<redacted>
```

该 E2E 覆盖本地测试库上的活动创建、quota 创建、事件接入、重复事件检查、outbox worker、reward callback success/failed、quota 状态校验和审计证据。

## Not Executed

- 未执行真实 DLQ `replay` / `close` API。原因：本任务交付为 runbook/SLO 文档，且用户明确禁止修改除本文件外的其他文件；对现有本地测试库中的任意 DLQ 做 replay/close 会改变业务状态，未作为本轮必要测试执行。
- 未执行 `go run ./cmd/engine-load-fault-tests --scenario all`。原因：该命令会写入 `tasks/2026-05-27-engine-load-fault-tests/runs/...` 运行日志，超出本轮“只允许写本任务 test.md”的约束。相关并发、quota、重复 callback、DLQ 逻辑已由 `go test ./...` 和 `cmd/engine-e2e-postgres` 提供基础覆盖。

## Failed Commands

```text
None.
```

需求：2026-05-29-alerts-runbook-slo-runbook-slo
