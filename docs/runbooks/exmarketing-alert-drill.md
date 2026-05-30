# ExMarketing Alert Production Drill

This drill is for local or test environments only. Do not stop production workers, break production database connectivity, create customer-visible DLQ, or mutate real quota state for this exercise.

## Scope

Use one isolated `drill_id`, tenant, channel, activity, source system, and operator for a whole run:

```text
drill_id=alerts-production-drill-YYYYMMDD-HHMM
tenant=drill_tenant
channel=drill_wallet
activity=drill_activity
source_system=drill_source
operator=alert-drill
```

Every scenario must record firing and resolved evidence from Prometheus or Alertmanager, plus cleanup evidence. Counter evidence must use `increase` or `rate`, not absolute counter differences.

## Prerequisites

1. Start the local drill stack. The `prometheus` service depends on the local metrics and webhook receiver harness, so the command below also exposes `http://localhost:8080/metrics` and records Alertmanager webhooks at `http://localhost:8080/drill/notifications`.

```bash
docker compose up -d postgres prometheus alertmanager
curl -fsS http://localhost:8080/metrics
curl -fsS http://localhost:9090/-/ready
curl -fsS http://localhost:9093/-/ready
```

If another local process already owns port `8080`, stop that process before running the acceptance command above. For ad hoc developer smoke only, the drill harness can be remapped with `EX_MARKETING_DRILL_HOST_PORT=18080 docker compose up -d postgres prometheus alertmanager` and `DRILL_URL=http://localhost:18080 scripts/exmarketing-alert-drill.sh --no-compose`. The `scripts/exmarketing-alert-drill.sh --start` helper keeps `8080` as the default acceptance path, but automatically falls back to `18080` when `8080` is already occupied by a non-drill local service.

2. In a second terminal, validate rules and routing before injecting faults:

```bash
promtool check config monitoring/prometheus-exmarketing.yml
promtool check config monitoring/prometheus-exmarketing.local.yml
promtool test rules monitoring/alerts-exmarketing.test.yml
alertmanager --config.file=monitoring/alertmanager-exmarketing.yml --config.check
alertmanager --config.file=monitoring/alertmanager-exmarketing.local.yml --config.check
```

3. For a full local live drill, run the harness script:

```bash
scripts/exmarketing-alert-drill.sh --start
```

The script serially triggers the six required scenarios, waits for firing and resolved webhook payloads, recovers each scenario, calls cleanup, and writes a Markdown report under `tasks/2026-05-29-alerts-production-drill/runs/`.

4. The local Alertmanager receiver is `cmd/exmarketing-alert-drill`. It records every webhook payload and exposes it through:

```bash
curl -fsS http://localhost:8080/drill/notifications
curl -fsS http://localhost:8080/drill/status
```

Each received alert must include `service`, `env`, `tenant`, `channel`, `severity`, `alertname`, `owner`, `summary`, `description`, `dashboard`, `runbook`, `first_seen`, `current_value`, and `threshold`.

5. Confirm there is no broad silence for `service="exmarketing"` that would hide target alerts.

```bash
curl -fsS http://localhost:9093/api/v2/silences
```

## Scenario Matrix

| Scenario | Target alert | Receiver | Trigger | Recovery | Cleanup |
| --- | --- | --- | --- | --- | --- |
| Worker stopped or not consuming | `ExMarketingWorkerAllStopped` P0 | `backend-oncall-p0` | Keep eligible outbox backlog present, set worker off or pause worker consumption, wait for oldest eligible age > 120s and zero successful claim rate for 5m. | Restore worker, verify claim success rate > 0, oldest eligible age falls, backlog drains, and resolved notification arrives. | Ensure worker config is restored and no drill backlog remains. |
| Outbox backlog high | `ExMarketingOutboxBacklogHigh` P1 | `backend-oncall` | Create more than 100 eligible `PENDING` or `FAILED` outbox items for the drill tenant/channel, then hold sender/worker path long enough for oldest eligible age > 60s for 5m. | Restore sender/worker path and verify backlog < 100, oldest eligible age falls, reward command p95 <= 120s, and no DLQ growth. | Drain or delete only drill outbox data. |
| DLQ growing | `ExMarketingDLQGrowing` P1 | `backend-oncall`, `business-ops` | Point the drill reward channel at a failing test provider until at least 5 drill commands enter DLQ over 10m. | Fix provider path, then replay or close only drill DLQ records after business confirmation. | Verify `exmarketing_outbox_dlq_count` is stable or lower and audit has `OUTBOX_DLQ_REPLAY` or `OUTBOX_DLQ_CLOSE`. |
| Callback failure rate high | `ExMarketingCallbackFailRateHigh` P1 | `backend-oncall`, `business-ops` | Send at least 5 invalid, rejected, failed, or conflicting callbacks in 5m while also sending enough accepted callbacks to prove the failure-rate denominator. | Restore valid callback payload/signature/config, verify accepted callbacks resume and failure rate drops below 5%. | Verify reconciliation terminal state and quota reservation confirm/release outcome for drill commands. |
| DB write failure or slow query | `ExMarketingDBWriteFailure` P0 for write failure; DB latency dashboard only for slow query | `dba-infra-p0` for P0 | For write failure, use an isolated DB or connection failure that causes a core write operation to fail. For slow query, run a test-only slow DB operation and observe `exmarketing_db_operation_latency_seconds`; do not expect the P0 unless a core write fails. | Restore DB connectivity or query performance, verify failed core writes stop for 5m and p95 latency returns to baseline. | Remove broken DSN/config, release locks, and record any inhibited downstream alerts. |
| Quota consistency risk | `ExMarketingQuotaOverDeductRisk` P0 | `backend-oncall-p0`, `business-ops` | Produce `exmarketing_quota_fail_total` with reason `quota_aggregate_conflict` or `quota_reservation_missing` on isolated drill data. Ordinary `action="reserve"` fail-rate or `quota_insufficient` is a dashboard check only and must not be treated as this P0. | Stop the consistency fault, reconcile aggregate used/frozen with reservations, and get business confirmation before timeout-release or business-impacting repair. | Verify no new consistency failures, reconciliation is clean, and audit contains repair actions if any. |

## Evidence Queries

Worker stopped:

```bash
curl -G http://localhost:9090/api/v1/query --data-urlencode 'query=sum by (tenant_id,target_channel) (exmarketing_outbox_backlog{send_status=~"PENDING|FAILED"})'
curl -G http://localhost:9090/api/v1/query --data-urlencode 'query=max by (tenant_id,target_channel) (exmarketing_outbox_oldest_eligible_age_seconds)'
curl -G http://localhost:9090/api/v1/query --data-urlencode 'query=sum by (tenant_id,target_channel) (rate(exmarketing_worker_claim_total{result="success"}[5m]))'
```

DLQ:

```bash
curl -G http://localhost:9090/api/v1/query --data-urlencode 'query=sum by (tenant_id,target_channel,reason) (increase(exmarketing_outbox_dlq_total[10m]))'
curl -G http://localhost:9090/api/v1/query --data-urlencode 'query=exmarketing_outbox_dlq_count'
```

Callback:

```bash
curl -G http://localhost:9090/api/v1/query --data-urlencode 'query=sum by (tenant_id,grant_channel,result,reason) (increase(exmarketing_reward_callback_total{result=~"rejected|failed|conflict"}[5m]))'
curl -G http://localhost:9090/api/v1/query --data-urlencode 'query=sum by (tenant_id,grant_channel) (rate(exmarketing_reward_callback_total{result=~"rejected|failed|conflict"}[5m])) / clamp_min(sum by (tenant_id,grant_channel) (rate(exmarketing_reward_callback_total[5m])), 0.001)'
```

DB:

```bash
curl -G http://localhost:9090/api/v1/query --data-urlencode 'query=sum by (operation,table,reason) (increase(exmarketing_db_operation_total{result="failed"}[5m]))'
curl -G http://localhost:9090/api/v1/query --data-urlencode 'query=histogram_quantile(0.95, sum by (operation,table,le) (rate(exmarketing_db_operation_latency_seconds_bucket[5m])))'
```

Quota:

```bash
curl -G http://localhost:9090/api/v1/query --data-urlencode 'query=sum by (tenant_id,quota_type,scope,reason) (increase(exmarketing_quota_fail_total{reason=~"quota_aggregate_conflict|quota_reservation_missing"}[5m]))'
curl -G http://localhost:9090/api/v1/query --data-urlencode 'query=sum by (tenant_id,quota_type,scope) (rate(exmarketing_quota_fail_total{action="reserve"}[5m])) / clamp_min(sum by (tenant_id,quota_type,scope) (rate(exmarketing_quota_attempt_total{action="reserve"}[5m])), 0.001)'
```

Alertmanager:

```bash
curl -fsS http://localhost:9093/api/v2/alerts | jq '.[] | {alertname: .labels.alertname, status: .status.state, receiver: .receivers, labels: .labels, annotations: .annotations}'
curl -fsS http://localhost:9093/api/v2/silences
curl -fsS http://localhost:8080/drill/notifications
```

## Local Fault Injection Commands

The local harness uses simulated metrics with the same metric names, labels, rules, Alertmanager routes, and `send_resolved` behavior as the ExMarketing alert configuration. It is intentionally isolated from production data and does not create real customer-visible outbox, DLQ, callback, DB, or quota mutations.

Trigger and recover one scenario manually:

```bash
curl -fsS -X POST http://localhost:8080/drill/scenario/worker_stopped/trigger
curl -fsS http://localhost:8080/metrics | rg 'exmarketing_outbox_backlog|exmarketing_outbox_oldest_eligible_age_seconds'
curl -fsS http://localhost:9093/api/v2/alerts
curl -fsS -X POST http://localhost:8080/drill/scenario/worker_stopped/recover
curl -fsS -X POST http://localhost:8080/drill/cleanup
```

Supported scenario names:

| Scenario name | Purpose |
| --- | --- |
| `worker_stopped` | Eligible backlog and oldest eligible age without successful claims. |
| `outbox_backlog` | More than 100 eligible `PENDING` outbox items. |
| `dlq_growing` | At least 5 new DLQ entries over the rule window. |
| `callback_failures` | At least 5 failed callbacks and failure rate above 5%. |
| `db_write_failure` | Core DB write failures for `grant_order_write`. |
| `db_slow_query` | DB latency observation only; it does not trigger DB P0. |
| `quota_consistency` | `quota_aggregate_conflict` consistency failures for quota P0. |
| `quota_insufficient` | Ordinary reserve insufficient quota path; it must not trigger quota P0. |

## Drill Report Template

```markdown
# ExMarketing Alert Drill Report

- Drill ID:
- Environment:
- Started at:
- Finished at:
- Participants:
- Isolated tenant/channel/activity/runId:
- Prometheus config checked: yes/no, evidence:
- Alertmanager route checked: yes/no, evidence:
- Existing silences reviewed: yes/no, evidence:

| Scenario | Alert | Triggered at | Receiver | Resolved at | Cleanup evidence | Result |
| --- | --- | --- | --- | --- | --- | --- |
| Worker stopped | `ExMarketingWorkerAllStopped` |  |  |  |  | PASS/FAIL |
| Outbox backlog | `ExMarketingOutboxBacklogHigh` |  |  |  |  | PASS/FAIL |
| DLQ growing | `ExMarketingDLQGrowing` |  |  |  |  | PASS/FAIL |
| Callback failures | `ExMarketingCallbackFailRateHigh` |  |  |  |  | PASS/FAIL |
| DB write failure or slow query | `ExMarketingDBWriteFailure` or DB latency |  |  |  |  | PASS/FAIL |
| Quota consistency | `ExMarketingQuotaOverDeductRisk` |  |  |  |  | PASS/FAIL |

## Timeline

- T+00m:
- T+05m:
- T+10m:

## Notification Evidence

- Firing payload evidence:
- Resolved payload evidence:
- Inhibited alerts, if any:

## Data Cleanup Evidence

- Outbox backlog:
- DLQ:
- Callback/reconciliation:
- Quota reservations/aggregates:
- Audit:
- Worker/DB/provider config restored:
- Active silences:

## Issues And Follow-Up

| Issue | Severity | Owner | Due date | Retest condition |
| --- | --- | --- | --- | --- |
```

## Repeatable Local Drill Records

### Promtool Rule Fixture Record

This record documents the repeatable local rule simulation using the Prometheus rule test fixture. It verifies rule expressions, labels, annotations, and the quota distinction called out in review. It does not claim real Alertmanager delivery behavior.

| Field | Value |
| --- | --- |
| Drill ID | `alerts-production-drill-promtool-20260530` |
| Environment | Local simulated metrics via `monitoring/alerts-exmarketing.test.yml` |
| Evidence command | `promtool test rules monitoring/alerts-exmarketing.test.yml` |
| Covered alerts | `ExMarketingWorkerAllStopped`, `ExMarketingOutboxBacklogHigh`, `ExMarketingDLQGrowing`, `ExMarketingCallbackFailRateHigh`, `ExMarketingDBWriteFailure`, `ExMarketingQuotaOverDeductRisk` |
| Quota fail-rate check | `quota_insufficient` reserve failures are asserted not to trigger `ExMarketingQuotaOverDeductRisk`; they remain dashboard-only evidence. |
| Result | PASS when the promtool rule test passes. |
| Residual risk | DB/provider real failure modes still require a connected test environment. Local notification delivery and resolved payloads are covered by the compose harness below. |

### Local Compose Harness Record

This record is produced by `scripts/exmarketing-alert-drill.sh --start`. It uses the same alert rule file and Alertmanager route semantics, a local metrics harness for safe fault injection, and a webhook receiver that records firing and resolved payloads.

| Field | Value |
| --- | --- |
| Drill ID | Generated as `alerts-production-drill-YYYYMMDDTHHMMSSZ` unless `DRILL_ID` is set. |
| Environment | Local Docker Compose: `postgres`, `exmarketing-alert-drill`, `prometheus`, `alertmanager`. |
| Bring-up command | `docker compose up -d postgres prometheus alertmanager` |
| Evidence command | `scripts/exmarketing-alert-drill.sh --start` |
| Report output | `tasks/2026-05-29-alerts-production-drill/runs/${DRILL_ID}-live-report.md` |
| Covered alerts | `ExMarketingWorkerAllStopped`, `ExMarketingOutboxBacklogHigh`, `ExMarketingDLQGrowing`, `ExMarketingCallbackFailRateHigh`, `ExMarketingDBWriteFailure`, `ExMarketingQuotaOverDeductRisk` |
| Quota fail-rate check | `quota_insufficient` is available as a manual negative-control scenario and must not be treated as `ExMarketingQuotaOverDeductRisk`. |
| Cleanup evidence | The script calls each scenario recover endpoint and final `/drill/cleanup`; final state is written to the report. |
