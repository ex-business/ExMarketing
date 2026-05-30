# 运维手册

## 本地启动

启动 PostgreSQL：

```bash
docker compose up -d postgres
```

确认数据库健康：

```bash
docker compose ps postgres
docker compose exec -T postgres pg_isready -U exmarketing -d exmarketing
```

运行服务：

```bash
export EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable'
go run ./cmd/exmarketing
```

指标接口：

```bash
curl http://localhost:8080/metrics
```

`/metrics` 输出 Prometheus text format，默认用于内网或本地运维采集，不建议直接公网暴露。Counter 是进程生命周期累计值，服务重启后从 0 重新开始；outbox backlog 和 DLQ Gauge 会在抓取时从当前仓储状态刷新。

Prometheus scrape 样例位于 `monitoring/prometheus-exmarketing.yml`，本地默认抓取 `localhost:8080/metrics`：

```bash
prometheus --config.file=monitoring/prometheus-exmarketing.yml
```

Prometheus 告警规则位于 `monitoring/alerts-exmarketing.rules.yml`，并已由
`monitoring/prometheus-exmarketing.yml` 的 `rule_files` 引入。Alertmanager
通知路由、抑制规则、升级接收组和消息模板位于：

- `monitoring/alertmanager-exmarketing.yml`
- `monitoring/alertmanager-exmarketing.tmpl`
- `docs/runbooks/exmarketing-alerts.md`

告警处置、SLO/SLA、演练记录、复盘模板和告警到 runbook 的映射表统一维护在
`docs/runbooks/exmarketing-alerts.md`。值班处理时优先打开 Alertmanager
告警中的 `runbook` label；若只从运维手册进入，使用以下入口：

| 入口 | 文档位置 | 用途 |
| --- | --- | --- |
| 告警公共字段与升级 | `docs/runbooks/exmarketing-alerts.md#common-fields`、`#escalation` | ack、升级、silence 和敏感信息约束。 |
| 告警 runbook | `docs/runbooks/exmarketing-alerts.md#alert-runbooks` | DLQ、outbox backlog、callback、quota、worker、DB、Fact Query 等逐项处置。 |
| SLI/SLO/SLA | `docs/runbooks/exmarketing-alerts.md#sli-slo-and-sla` | 可用性、延迟、错误率、数据一致性和 DLQ 清理时效。 |
| 业务确认边界 | `docs/runbooks/exmarketing-alerts.md#business-confirmation-policy` | 区分技术可直接处理和必须业务确认的动作。 |
| 演练与复盘模板 | `docs/runbooks/exmarketing-alerts.md#drill-record-template`、`#incident-review-template` | 演练记录、事故复盘和预防项跟踪。 |
| 生产告警演练手册 | `docs/runbooks/exmarketing-alert-drill.md` | 本地/测试环境故障注入、通知、恢复、清理和演练报告步骤。 |
| 告警映射表 | `docs/runbooks/exmarketing-alerts.md#alert-to-runbook-mapping` | alertname、severity、owner、dashboard、anchor、验证指标。 |

规则按 `P0/P1/P2/P3` 标注严重级别，并统一带上 `service`、`tenant`、
`channel`、`severity`、`summary`、`dashboard` 和 `runbook`。DLQ、outbox
backlog、worker claim 相关规则按 `tenant_id,target_channel` 聚合，不做全局
sum；callback 按 `tenant_id,grant_channel` 聚合；Fact Query 按
`tenant_id,fact_code` 聚合。DB 写失败会按 `service,env,tenant` 优先精确抑制
下游 outbox/callback/worker/event/Fact Query 泛洪告警，只有
`tenant="global"` 的 DB 根因才退化为 `service,env` 抑制。

告警静态校验和样例单测：

```bash
promtool check config monitoring/prometheus-exmarketing.yml
promtool test rules monitoring/alerts-exmarketing.test.yml
alertmanager --config.file=monitoring/alertmanager-exmarketing.yml --config.check
```

Grafana dashboard 位于 `monitoring/grafana-exmarketing-dashboard.json`。导入时将 `${DS_PROMETHEUS}` 绑定到实际 Prometheus 数据源即可；dashboard 不写死本机或生产地址。

核心指标：

| 指标 | 类型 | 标签 | 说明 |
| --- | --- | --- | --- |
| `exmarketing_event_intake_total` | Counter | `tenant_id`, `source_system`, `event_type`, `result`, `reason` | Event API 成功、失败、重复、鉴权失败、quota 拒绝等结果。 |
| `exmarketing_event_intake_latency_seconds` | Histogram | `tenant_id`, `source_system`, `event_type`, `result` | Event API 处理耗时。 |
| `exmarketing_rule_execution_total` | Counter | `tenant_id`, `event_type`, `phase`, `result`, `reason` | Rule 各阶段执行、跳过、拒绝和错误计数。 |
| `exmarketing_rule_stage_latency_seconds` | Histogram | `tenant_id`, `event_type`, `phase`, `result` | Rule 阶段耗时。 |
| `exmarketing_reward_execution_total` | Counter | `tenant_id`, `reward_type`, `calculate_type`, `grant_channel`, `result`, `reason` | Reward 计算成功、跳过和失败计数。 |
| `exmarketing_reward_stage_latency_seconds` | Histogram | `tenant_id`, `reward_type`, `calculate_type`, `grant_channel`, `result` | Reward 计算耗时。 |
| `exmarketing_quota_attempt_total` | Counter | `tenant_id`, `quota_type`, `scope`, `action`, `result`, `reason` | Quota 操作尝试。当前 fail rate 只用 `action="reserve"` 作为分母，避免 confirm/release 稀释 reserve 拒绝率。 |
| `exmarketing_quota_fail_total` | Counter | `tenant_id`, `quota_type`, `scope`, `action`, `reason` | Quota 失败计数。 |
| `exmarketing_quota_amount_total` | Counter | `tenant_id`, `quota_type`, `scope`, `action`, `result` | Quota reserve 冻结量、confirm 确认量、release 释放量。 |
| `exmarketing_outbox_backlog` | Gauge | `tenant_id`, `target_channel`, `send_status` | 当前 outbox 状态数量，覆盖 `PENDING`, `FAILED`, `SENDING`, `DLQ`, `DLQ_CLOSED`。 |
| `exmarketing_outbox_oldest_eligible_age_seconds` | Gauge | `tenant_id`, `target_channel` | 最老可投递 outbox 等待秒数。未到期 retry 不抬高该值。 |
| `exmarketing_outbox_dlq_count` | Gauge | `tenant_id`, `target_channel` | 当前未关闭 DLQ 数量。 |
| `exmarketing_outbox_dlq_total` | Counter | `tenant_id`, `target_channel`, `reason` | 新进入 DLQ 的累计数量。 |
| `exmarketing_reward_callback_total` | Counter | `tenant_id`, `grant_channel`, `callback_status`, `result`, `reason` | Reward callback 成功、重复、冲突、拒绝和失败。 |
| `exmarketing_reward_callback_latency_seconds` | Histogram | `tenant_id`, `grant_channel`, `result` | Callback 接收到业务状态更新完成耗时。 |
| `exmarketing_worker_claim_total` | Counter | `worker_id`, `tenant_id`, `target_channel`, `result` | Worker claim 成功、空批次、失败和冲突。 |
| `exmarketing_worker_claim_latency_seconds` | Histogram | `tenant_id`, `target_channel` | outbox 从 eligible 到被 claim 的延迟。 |
| `exmarketing_worker_inflight` | Gauge | `worker_id`, `target_channel` | Worker 当前处理中 outbox 数。 |
| `exmarketing_reward_command_total` | Counter | `tenant_id`, `target_channel`, `result`, `reason` | Reward command 投递 sent/retry/dlq 结果。 |
| `exmarketing_reward_command_latency_seconds` | Histogram | `tenant_id`, `target_channel`, `result` | outbox 创建到投递终态耗时，不代表最终发奖成功。 |
| `exmarketing_reward_command_delivery_latency_seconds` | Histogram | `tenant_id`, `target_channel`, `result` | 单次外部投递调用耗时。 |
| `exmarketing_db_operation_total` | Counter | `operation`, `table`, `result`, `reason` | DB 操作成功和失败计数；当前覆盖 metrics 刷新依赖的 outbox/DLQ 查询。 |
| `exmarketing_db_operation_latency_seconds` | Histogram | `operation`, `table`, `result` | DB 操作耗时。 |
| `exmarketing_fact_query_total` | Counter | `tenant_id`, `fact_code`, `result`, `reason` | Fact Query 成功、缓存命中、拒绝、超时、provider 失败和 mapping 失败计数。 |
| `exmarketing_fact_query_latency_seconds` | Histogram | `tenant_id`, `fact_code`, `result` | Fact Query 从接收请求到缓存命中、成功返回或失败的耗时。 |

`ExMarketingDBWriteFailure` 只允许基于核心写路径启用，默认匹配以下
`exmarketing_db_operation_total.operation`：`event_inbox_write`、
`participation_write`、`grant_order_write`、`quota_reserve_write`、
`quota_finalize_write`、`outbox_write`、`outbox_dlq_write`、`callback_write`。
查询类 DB 失败只能进入 DB operation/metrics 健康排查，不能冒充核心写失败 P0。

建议告警阈值：

| 告警 | 窗口 | 阈值 |
| --- | --- | --- |
| Event intake failed rate | 5 分钟 | 失败率 > 5% 且失败数 >= 20。 |
| Event intake internal error | 5 分钟 | `reason="internal_error"` >= 1。 |
| Quota reserve fail rate | 5 分钟 | `action="reserve"` 失败率 > 20% 且失败数 >= 10。 |
| Outbox backlog count | 5 分钟 | 单 tenant/channel 可投递 backlog > 100。 |
| Outbox backlog age | 5 分钟 | `exmarketing_outbox_oldest_eligible_age_seconds` > 60。 |
| DLQ count | 即时或 5 分钟 | 未关闭 DLQ > 0；单 tenant/channel >= 10 升级 High。 |
| Callback fail rate | 5 分钟 | `result=~"rejected|failed|conflict"` 失败率 > 5% 且失败数 >= 5。 |
| Worker claim latency P95 | 5 分钟 | P95 > 60 秒。 |
| Reward command latency P95 | 5 分钟 | P95 > 120 秒。 |
| DB operation error | 5 分钟 | `exmarketing_db_operation_total{result="failed"}` 持续增长。 |
| DB operation latency P95 | 5 分钟 | P95 明显超过当前仓储操作基线，测试环境建议先用 1 秒作为排查阈值。 |

失败率类面板必须同时满足失败率和最小失败数才判 Red，避免低流量单次失败造成误报：

```promql
sum(increase(exmarketing_event_intake_total{result=~"failed|rejected"}[5m])) >= 20
and
(
  sum(rate(exmarketing_event_intake_total{result=~"failed|rejected"}[5m]))
  / clamp_min(sum(rate(exmarketing_event_intake_total[5m])), 0.001)
) > 0.05
```

```promql
sum(increase(exmarketing_quota_fail_total{action="reserve"}[5m])) >= 10
and
(
  sum(rate(exmarketing_quota_fail_total{action="reserve"}[5m]))
  / clamp_min(sum(rate(exmarketing_quota_attempt_total{action="reserve"}[5m])), 0.001)
) > 0.20
```

```promql
sum(increase(exmarketing_reward_callback_total{result=~"rejected|failed|conflict"}[5m])) >= 5
and
(
  sum(rate(exmarketing_reward_callback_total{result=~"rejected|failed|conflict"}[5m]))
  / clamp_min(sum(rate(exmarketing_reward_callback_total[5m])), 0.001)
) > 0.05
```

指标标签禁止包含 token、Authorization、Cookie、secret、password、完整 payload、完整 targetRef、eventId、bizId、subjectId、userId、activityId、activity_id、billNo、commandId、idempotencyKey 或原始错误文本；这些信息只允许进入审计、对账查询或已脱敏日志。`tenant_id`、`source_system`、`event_type`、`result`、`reason`、`quota_type`、`scope`、`action`、`target_channel`、`send_status`、`grant_channel`、`callback_status`、`worker_id`、`operation`、`table` 和 `phase` 是当前允许的低基数标签。若需要定位单个活动、事件、用户或订单，使用审计、对账查询、DB 查询或脱敏日志，不通过 Prometheus 标签承载。

本地验证：

```bash
go run ./cmd/exmarketing
curl -fsS http://localhost:8080/metrics | tee /tmp/exmarketing.metrics
rg 'exmarketing_(event_intake|rule_execution|reward_execution|quota_attempt|quota_amount|outbox_backlog|reward_callback|worker_claim|db_operation)' /tmp/exmarketing.metrics
rg 'activity_id=|activityId=|eventId=|subjectId=|userId=|bizId=|commandId=|idempotencyKey=' /tmp/exmarketing.metrics && exit 1 || true
promtool check config monitoring/prometheus-exmarketing.yml
```

开启 Outbox worker：

```bash
export EX_MARKETING_OUTBOX_WORKER=true
```

运行 PostgreSQL 端到端验收：

```bash
export EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable'
go run ./cmd/engine-e2e-postgres
```

该命令会在进程内启动 API handler、Outbox worker 和 reward callback stub，但所有活动、quota、event inbox、participation、grant_order、outbox、callback 和对账状态都写入 `EX_MARKETING_DATABASE_URL` 指向的 PostgreSQL。命令使用唯一 `runId` 隔离数据，可重复执行；未设置 PostgreSQL DSN 时会失败。

## PostgreSQL 备份恢复演练

本地/测试环境灾备演练使用 `cmd/db-backup-restore`。该工具只支持
PostgreSQL，默认读取 `EX_MARKETING_DATABASE_URL` 作为源库，并读取
`EX_MARKETING_RESTORE_DATABASE_URL` 作为恢复目标库。命令输出会脱敏 DSN；
`pg_dump`/`pg_restore` 执行时不会把 URL 密码放进命令参数。如果宿主机
PATH 缺少 PostgreSQL client，工具会 fallback 到本仓库 `docker compose`
的 `postgres` 服务内执行，并把 `localhost:54329` 改写为容器内的
`localhost:5432`。

准备源库并应用 migration：

```bash
docker compose up -d postgres
export EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable'
go run ./cmd/db-migration-release --mode apply
go run ./cmd/engine-e2e-postgres
```

生成 schema + data 备份：

```bash
go run ./cmd/db-backup-restore --mode backup --output-dir backups/db-restore-drills
```

成功输出包含备份文件路径、大小、SHA-256、UTC 创建时间和脱敏源 DSN。备份文件
使用 PostgreSQL custom format，文件名包含数据库名与 UTC 时间戳，避免覆盖旧备份。

创建隔离恢复库：

```bash
docker compose exec -T postgres createdb -U exmarketing exmarketing_restore
export EX_MARKETING_RESTORE_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing_restore?sslmode=disable'
```

恢复备份：

```bash
go run ./cmd/db-backup-restore --mode restore --backup-file backups/db-restore-drills/<backup>.dump
```

`restore` 默认拒绝以下情况：

- 未提供恢复目标 DSN。
- 恢复目标与源库相同。
- 备份文件为空或不可读。
- 恢复目标库已经存在 public base table。

如果确认目标库只是本地演练库并且可以覆盖，才允许加 `--force`：

```bash
go run ./cmd/db-backup-restore --mode restore --backup-file backups/db-restore-drills/<backup>.dump --force
```

校验恢复库 schema、migration checksum 和核心表 row count：

```bash
go run ./cmd/db-backup-restore --mode validate
```

`validate` 会执行与 migration release 一致的 drift 检查，并统计核心表：
`marketing_activity`、`marketing_rule`、`marketing_reward`、`marketing_quota`、
`marketing_event_inbox`、`marketing_participation`、`marketing_grant_order`、
`marketing_outbox`、`marketing_audit` 和 `marketing_fact_provider`。

在恢复库上跑 Event In 到 Outbox smoke：

```bash
go run ./cmd/db-backup-restore --mode smoke
```

该模式会把 `EX_MARKETING_DATABASE_URL` 临时指向恢复目标 DSN，并调用
`go run ./cmd/engine-e2e-postgres`。通过证据包括唯一 `runId`、tenant、
activity、event inbox、participation、grant_order、outbox 和 callback/reconciliation
状态。若只需要截止到 Outbox 的最小证据，也可以直接运行恢复库上的
`go run ./cmd/engine-e2e-postgres` 并保留输出。

失败诊断按阶段输出：`BACKUP`、`RESTORE`、`SCHEMA_CHECK`、`DATA_CHECK` 或
`EVENT_IN_SMOKE`。错误信息会包含脱敏 DSN、命令退出摘要和 stderr 摘要，不应包含
数据库密码、token、secret、Authorization 或 Cookie。

MQ reward commands use `targetRef` values such as `kafka://reward.commands`, `rabbitmq://reward-exchange/reward.commands`, `rabbitmq:///reward.commands`, or `pulsar://tenant/rewards/commands`. Broker endpoints and credentials belong in runtime sender configuration, not in activity config, outbox payload, audit payload, or logs.

MQ sender runtime config:

```bash
export EX_MARKETING_MQ_SENDERS_JSON='{
  "providers": {
    "kafka": {
      "enabled": true,
      "endpoints": ["kafka-1:9092", "kafka-2:9092"],
      "authRef": "secret/exmarketing/kafka",
      "tls": true,
      "caFile": "/run/secrets/kafka-ca.pem",
      "clientCertFile": "/run/secrets/kafka-client.pem",
      "clientKeyFile": "/run/secrets/kafka-client.key",
      "sendTimeoutMillis": 5000,
      "destinationAllowlist": ["reward.commands"]
    },
    "rabbitmq": {
      "enabled": false,
      "endpoints": []
    },
    "pulsar": {
      "enabled": false,
      "endpoints": []
    }
  }
}'
```

Rules:

- Provider keys are `kafka`, `rabbitmq`, or `pulsar`.
- Enabled providers require at least one endpoint.
- Endpoints must not include userinfo or `password=`, `token=`, `secret=` query material; use `authRef` or mounted secret files instead.
- `destinationAllowlist` is optional. When set, a MQ outbox destination must match exactly or the worker keeps the outbox retryable and eventually moves it to DLQ.
- A configured provider is not enough to mark a message `SENT`; the registered provider transport must return only after Kafka ack, RabbitMQ publisher confirm, or Pulsar send ack.
- Missing provider config, disabled provider, allowlist refusal, timeout, auth failure, or missing transport adapter keeps the original outbox payload and follows retry/DLQ handling with a redacted error summary.

配置 Fact Query allowlist：

```bash
export EX_MARKETING_FACT_ALLOWLIST='member.example.com,profile.example.com'
```

## 停止服务

停止 Go 服务：

```bash
Ctrl-C
```

停止 PostgreSQL：

```bash
docker compose down
```

删除本地数据卷：

```bash
docker compose down -v
```

## Migration 发布流程

正式发布使用 forward-only migration。已成功发布的 `db/migration/NNN_name.sql`
不得修改；修复已发布 schema 必须新增更高版本 SQL。流程使用数据库内的
`schema_migrations` 表记录版本、文件名、SHA-256 checksum、执行状态、开始/完成时间和脱敏错误摘要。

本地 PostgreSQL：

```bash
docker compose up -d postgres
export EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable'
```

发布前检查：

```bash
go run ./cmd/db-migration-release --mode precheck
```

应用未执行 migration：

```bash
go run ./cmd/db-migration-release --mode apply
```

检测 schema drift：

```bash
go run ./cmd/db-migration-release --mode drift
```

查看状态：

```bash
go run ./cmd/db-migration-release --mode status
```

`precheck` 会验证 PostgreSQL 可连接、migration 目录可读、文件名和版本唯一、
已执行版本的 checksum 与本地文件一致、没有失败未处理记录，并打印待执行版本。
空库会自动创建 `schema_migrations` 后执行全部 migration；重复执行 `apply` 会跳过
已成功版本。已有业务表但没有 `schema_migrations` 记录的历史环境会被阻止自动升级，
必须先人工确认基线后再继续，避免重跑初始化 SQL 破坏数据。

`drift` 至少检查 version/checksum 与核心表存在性，核心表包括
`marketing_activity`、`marketing_rule`、`marketing_reward`、`marketing_quota`、
`marketing_event_inbox`、`marketing_participation`、`marketing_grant_order`、
`marketing_outbox`、`marketing_audit` 和 `marketing_fact_provider`。

失败处理：

- migration SQL 失败时流程停止，不会继续执行后续版本。
- 失败版本会记录为 `failed`，错误摘要会截断并脱敏。
- 共享环境和生产环境不执行自动 down 回滚；优先从备份恢复或新增更高版本修复 migration。
- 如果失败发生在未成功版本且事务已回滚，修复同一未成功 migration 后，需要先人工清理或确认失败记录，再重新执行 `precheck` 和 `apply`。

本地验证建议：

```bash
docker compose down -v
docker compose up -d postgres
export EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable'
go run ./cmd/db-migration-release --mode apply
go run ./cmd/db-migration-release --mode apply
go run ./cmd/db-migration-release --mode drift
go test ./...
go build ./...
```

查看表：

```bash
docker compose exec -T postgres psql -U exmarketing -d exmarketing -c '\dt'
```

## 清空本地测试数据

```bash
docker compose exec -T postgres psql -U exmarketing -d exmarketing -c "
TRUNCATE
  marketing_outbox,
  marketing_grant_order,
  marketing_participation,
  marketing_event_inbox,
  marketing_reward,
  marketing_rule,
  marketing_quota,
  marketing_audit,
  marketing_activity;
"
```

## 常用查询

活动：

```sql
SELECT id, tenant_id, activity_code, status, start_time, end_time
FROM marketing_activity
ORDER BY created_at DESC;
```

事件 Inbox：

```sql
SELECT id, tenant_id, event_type, source_system, biz_type, biz_id, process_status, error_message
FROM marketing_event_inbox
ORDER BY created_at DESC
LIMIT 20;
```

参与记录：

```sql
SELECT id, activity_id, subject_type, subject_id, biz_type, biz_id, status, reject_reason
FROM marketing_participation
ORDER BY created_at DESC
LIMIT 20;
```

发奖单：

```sql
SELECT id, tenant_id, bill_no, reward_type, reward_amount, grant_status, grant_channel, error_message
FROM marketing_grant_order
ORDER BY created_at DESC
LIMIT 20;
```

Quota：

```sql
SELECT activity_id, quota_type, scope, scope_key, total_value, used_value, frozen_value, version
FROM marketing_quota
ORDER BY updated_at DESC;
```

长期冻结排查：

```sql
SELECT tenant_id, command_id, activity_id, quota_type, scope, scope_key, reserved_amount, created_at
FROM marketing_quota_reservation
WHERE reservation_status = 'FROZEN'
ORDER BY created_at ASC;
```

超时释放先 dry-run，再实际执行：

```bash
curl -X POST http://localhost:8080/admin/v1/quota-reservations/timeout-release \
  -H 'Content-Type: application/json' \
  -d '{"tenantId":"site_001","limit":100,"dryRun":true,"operator":"ops-a","reason":"provider timeout"}'

curl -X POST http://localhost:8080/admin/v1/quota-reservations/timeout-release \
  -H 'Content-Type: application/json' \
  -d '{"tenantId":"site_001","limit":100,"operator":"ops-a","reason":"provider timeout"}'
```

Outbox：

```sql
SELECT id, tenant_id, message_type, target_channel, target_ref, send_status, retry_count, next_retry_at, error_message
FROM marketing_outbox
ORDER BY created_at DESC
LIMIT 20;
```

DLQ：

```sql
SELECT id, tenant_id, target_channel, target_ref, retry_count, error_message, updated_at, send_status
FROM marketing_outbox
WHERE send_status IN ('DLQ', 'DLQ_CLOSED')
ORDER BY updated_at DESC;
```

DLQ 管理 API：

```bash
curl 'http://localhost:8080/admin/v1/outbox/dlq?tenantId=site_001&includeClosed=true'

curl -X POST http://localhost:8080/admin/v1/outbox/{outboxId}/replay \
  -H 'content-type: application/json' \
  -d '{"tenantId":"site_001","operator":"ops","reason":"target recovered"}'

curl -X POST http://localhost:8080/admin/v1/outbox/{outboxId}/close \
  -H 'content-type: application/json' \
  -d '{"tenantId":"site_001","operator":"ops","reason":"business rejected"}'
```

`replay` 和 `close` 只允许当前状态为 `DLQ` 的记录；`DLQ_CLOSED` 不允许 replay。操作会写入 `marketing_audit`。

审计：

```sql
SELECT actor, action, resource, resource_id, created_at
FROM marketing_audit
ORDER BY created_at DESC
LIMIT 50;
```

## 常见问题

### quota exceeded

含义：

```text
活动预算不足，或用户次数限制已达上限。
```

排查：

```sql
SELECT quota_type, scope, scope_key, total_value, used_value
FROM marketing_quota
WHERE activity_id = '<activity_id>';
```

处理：

```text
确认配置是否正确。
如需提高预算或次数，调整 total_value。
不要直接降低 used_value，除非明确是在清理本地测试数据。
```

### Idempotency-Key does not match event payload

含义：

```text
请求头 Idempotency-Key 与事件 payload 计算出的幂等键不一致。
```

计算规则：

```text
tenantId:sourceSystem:eventType:bizType:bizId
```

### OpenAPI Event API auth and size limit

生产环境必须开启事件接入鉴权：

```bash
export EX_MARKETING_REQUIRE_OPENAPI_AUTH=true
export EX_MARKETING_OPENAPI_TOKENS='token-a:site_001:payment'
```

`EX_MARKETING_OPENAPI_TOKENS` 格式：

```text
token:tenantId[,tenantId...]:sourceSystem[,sourceSystem...];token2:tenantId2:sourceSystem2
```

规则：

- `EX_MARKETING_REQUIRE_OPENAPI_AUTH=true` 且未配置有效 token 时服务启动失败。
- `POST /openapi/v1/events` 必须携带 `Authorization: Bearer <token>`。
- token 只允许上报配置内的 `tenantId` 和 `sourceSystem`。
- 未设置或为 `false` 时仅用于本地开发，服务启动日志会提示 OpenAPI Event auth disabled。
- `EX_MARKETING_EVENT_MAX_BODY_BYTES` 可覆盖默认 1 MiB 请求体限制。
- 日志和错误响应不得记录完整 token 或完整 Authorization header。

### fact endpoint host is not allowlisted

含义：

```text
Fact Query endpoint host 不在 EX_MARKETING_FACT_ALLOWLIST 中。
```

处理：

```bash
export EX_MARKETING_FACT_ALLOWLIST='member.example.com'
```

### fact endpoint resolves to private or local address

含义：

```text
Fact endpoint 解析到了 private/local IP，被 SSRF 防护拦截。
```

处理：

```text
只能使用合法公网 provider，或后续设计内网 provider 的专用安全通道。
```

### outbox target refused

含义：

```text
Outbox worker 投递 HTTP/Webhook/MQ 失败。
```

排查：

```sql
SELECT id, target_ref, send_status, retry_count, error_message
FROM marketing_outbox
ORDER BY updated_at DESC;
```

处理：

```text
确认 targetRef 可达。
确认 rewardConfig.targetRef 配置正确。
失败超过重试次数后会进入 DLQ。
```

### duplicate event

含义：

```text
相同 idempotency_key 的事件已经处理过。
```

排查：

```sql
SELECT id, process_status, payload_json
FROM marketing_event_inbox
WHERE idempotency_key = '<key>';
```

## 生产前检查

生产部署前至少确认：

```text
Admin API 已加鉴权或只在内网暴露
OpenAPI Event In 已加鉴权
Reward callback 已加签名或 token 校验
Fact Query allowlist 已配置
Outbox worker 实例数和 claim 机制验证通过
Quota 原子扣减压测通过
DLQ 有告警和人工处理流程
审计表有保留策略
数据库 migration 有正式发布流程
```
