# Test

## Scope

根据 `task.md`、`analysis.md`、`review.md`、`test-plan.md`、`implementation.md` 执行测试。

本轮只写入本文件，未修改业务代码、测试代码、文档代码、配置文件或其他文件。

## Result

```text
PASS
```

核心验收通过：

- Grafana dashboard JSON 可解析。
- Prometheus scrape 样例包含 `job_name`、`metrics_path`、`scrape_interval`、static target。
- dashboard / operations 文档覆盖 event、rule、reward、quota、outbox、DLQ、callback、worker、DB。
- event、quota、callback 失败率 Red 判断均使用 5 分钟窗口下的“失败率 + 最小失败数”双条件。
- dashboard 和 scrape 配置未直接使用禁止标签或敏感字段。
- `go test ./...` 通过。
- `go build ./...` 通过。
- PostgreSQL migration 与 E2E smoke 通过。
- 当前服务 `/metrics` 返回 Prometheus text format，并包含本任务要求的指标 family。
- `/metrics` 输出未发现禁止标签或敏感字段。

未完成项：

- 未执行真实 Prometheus target healthy / Grafana UI 导入验证。原因：本机未安装 `promtool`，且 `localhost:9090` 未运行 Prometheus。该项不判定为本轮阻塞，因为本任务非目标是不部署真实生产 Prometheus/Grafana，且已完成 scrape 配置静态检查与 `/metrics` 可抓取验证。

## Commands

```bash
python3 -m json.tool monitoring/grafana-exmarketing-dashboard.json >/dev/null
```

结果：PASS。Grafana dashboard JSON 可解析。

```bash
sed -n '1,120p' monitoring/prometheus-exmarketing.yml
```

结果：PASS。配置包含：

```text
global:
  scrape_interval: 15s
  evaluation_interval: 15s

scrape_configs:
  - job_name: exmarketing
    metrics_path: /metrics
    scheme: http
    static_configs:
      - targets:
          - localhost:8080
        labels:
          service: exmarketing
```

```bash
rg -n "activity_id=|activityId=|eventId=|subjectId=|userId=|bizId=|billNo|commandId=|idempotencyKey=|Authorization|Cookie|token|secret|password" monitoring/grafana-exmarketing-dashboard.json monitoring/prometheus-exmarketing.yml
```

结果：PASS。退出码 1，无匹配；dashboard 和 scrape 配置没有直接使用禁止标签或敏感字段。

```bash
rg -n "exmarketing_event_intake|exmarketing_rule|exmarketing_reward|exmarketing_quota|exmarketing_outbox|exmarketing_reward_callback|exmarketing_worker|exmarketing_db" monitoring/grafana-exmarketing-dashboard.json docs/operations.md
```

结果：PASS。dashboard / operations 文档覆盖 event、rule、reward、quota、outbox、callback、worker、DB 指标。

```bash
rg -n "rate\\(|increase\\(|histogram_quantile\\(|up\\{" monitoring/grafana-exmarketing-dashboard.json docs/operations.md
```

结果：PASS。dashboard / operations 文档包含 scrape up、rate、increase、histogram_quantile 查询。

```bash
python3 - <<'PY'
import json
p='monitoring/grafana-exmarketing-dashboard.json'
d=json.load(open(p))
print('title:', d.get('title'))
print('uid:', d.get('uid'))
print('panels:', len(d.get('panels', [])))
for panel in d.get('panels', []):
    print('-', panel.get('title'))
PY
```

结果：PASS。

```text
title: ExMarketing Activity Engine Health
uid: exmarketing-health
panels: 21
```

面板包含 Scrape Up、Event Failure Red、Quota Reserve Failure Red、DLQ、Outbox、DB、Event、Rule、Reward、Quota、Callback、Worker 等分组/能力。

```bash
python3 - <<'PY'
import json
p='monitoring/grafana-exmarketing-dashboard.json'
d=json.load(open(p))
for title in ['Event Failure Red','Quota Reserve Failure Red','Callback Failure Rate']:
    for panel in d.get('panels',[]):
        if panel.get('title') == title:
            print('\\nPANEL', title)
            for t in panel.get('targets',[]):
                print(t.get('expr',''))
            print('thresholds', panel.get('fieldConfig',{}).get('defaults',{}).get('thresholds'))
PY
```

结果：PASS。

关键输出摘要：

```text
PANEL Event Failure Red
sum(increase(exmarketing_event_intake_total{tenant_id=~"$tenant_id",result=~"failed|rejected"}[5m])) >= 20 and (...) > 0.05
threshold red value = 1

PANEL Quota Reserve Failure Red
sum(increase(exmarketing_quota_fail_total{tenant_id=~"$tenant_id",action="reserve"}[5m])) >= 10 and (...) > 0.20
threshold red value = 1

PANEL Callback Failure Rate
sum(increase(exmarketing_reward_callback_total{tenant_id=~"$tenant_id",result=~"rejected|failed|conflict"}[5m])) >= 5 and (...) > 0.05
threshold red value = 1
```

上一轮 `Callback Failure Rate` 缺少最小失败数门槛的问题已修复。

```bash
go test ./...
```

结果：PASS。

```text
ok github.com/ex-stack/ExMarketing/internal/httpapi 0.443s
ok github.com/ex-stack/ExMarketing/internal/workflow 0.492s
其他包为 PASS / cached / no test files
```

```bash
go build ./...
```

结果：PASS。

```bash
docker compose exec -T postgres pg_isready -U exmarketing -d exmarketing
```

结果：PASS。

```text
/var/run/postgresql:5432 - accepting connections
```

```bash
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go run ./cmd/db-migration-release --mode apply
```

结果：PASS。

```text
APPLY PASS
applied=3 pending=0
applied 001 001_marketing_core.sql success
applied 002 002_audit_fact_outbox_indexes.sql success
applied 003 003_activity_unique_constraint.sql success
```

```bash
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go run ./cmd/engine-e2e-postgres
```

结果：PASS。

```text
PASS runId=e2e_pg_20260530_085603_000000000 tenantId=site_001_e2e_pg_20260530_085603_000000000 activityId=act_8ee50cc8397e7cceda13a3b9 activityCode=e2e_pg_20260530_085603_000000000 successCommand=cmd_9033ad70f20e60dd0de7ce03 failedCommand=cmd_7114ecde716780a239ecb27c
```

```bash
EX_MARKETING_ADDR=':18080' EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go run ./cmd/exmarketing
curl -fsS http://localhost:18080/metrics | head -n 140
curl -fsS http://localhost:18080/metrics | rg -n "activity_id=|activityId=|eventId=|subjectId=|userId=|bizId=|billNo|commandId=|idempotencyKey=|Authorization|Cookie|token|secret|password"
curl -fsS http://localhost:18080/metrics | rg -n "exmarketing_(event_intake|rule_execution|reward_execution|quota_attempt|quota_fail|quota_amount|outbox_backlog|outbox_dlq|reward_callback|worker_claim|worker_inflight|db_operation)"
```

结果：PASS。

- `/metrics` 返回 Prometheus text format。
- 禁止标签和敏感字段扫描退出码 1，无匹配。
- 输出包含 event、rule、reward、quota、outbox、DLQ、callback、worker、DB 指标 family。
- 18080 验证进程已停止。

指标输出摘要：

```text
# HELP exmarketing_event_intake_total Event API intake results.
# HELP exmarketing_rule_execution_total Activity rule execution results.
# HELP exmarketing_reward_execution_total Activity reward calculation results.
# HELP exmarketing_quota_attempt_total Quota reserve, confirm, and release attempts.
# HELP exmarketing_quota_fail_total Quota failed attempts.
# HELP exmarketing_quota_amount_total Quota amount reserved, confirmed, or released.
# HELP exmarketing_outbox_backlog Current outbox backlog by status.
# HELP exmarketing_outbox_dlq_count Current open DLQ outbox count.
# HELP exmarketing_reward_callback_total Reward callback results.
# HELP exmarketing_worker_claim_total Outbox worker claim attempts.
# HELP exmarketing_worker_inflight Outbox messages currently processed by worker.
# HELP exmarketing_db_operation_total Database operation results.
# HELP exmarketing_db_operation_latency_seconds Database operation latency in seconds.
```

```bash
command -v promtool || true
curl -fsS http://localhost:9090/api/v1/targets
```

结果：环境限制，未完成真实 Prometheus target smoke。

日志摘要：

```text
promtool: command not found
curl: (7) Failed to connect to localhost port 9090 after 0 ms: Couldn't connect to server
```

影响：无法在本机确认 Prometheus `up{job="exmarketing"}`，但 scrape 配置静态检查通过，且 `/metrics` endpoint 已用 `curl` 验证可抓取。

## Findings

未发现阻塞问题。

需求：2026-05-29-alerts-prometheus-grafana-prometheus-grafana
