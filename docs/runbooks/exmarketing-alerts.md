# ExMarketing Alert Runbook

## Common Fields

Every alert includes `service`, `tenant`, `channel`, `severity`, `summary`, `dashboard`, `runbook`, `owner`, `current_value`, and `threshold`. Alert labels must stay low-cardinality; use audit, reconciliation, admin queries, or redacted logs for event, activity, subject, order, command, or idempotency-key lookup.

Maintenance silences should match `service="exmarketing"` plus the narrowest safe combination of `env`, `tenant`, `channel`, `severity`, and `alertname`. Planned maintenance silences require owner, reason, start/end time, expected impact, rollback contact, and a default maximum duration of 4 hours unless approved by oncall and business operations.

Do not paste token, Authorization, Cookie, secret, password, full payload, full targetRef, eventId, bizId, subjectId, userId, activityId, billNo, commandId, idempotencyKey, or raw provider error text into metrics labels or public incident material. Use redacted counts and references.

## Escalation

P0 alerts page backend oncall or DBA/infra immediately. If unacknowledged for 10 minutes, escalate to backend and infra leads; if unresolved after 20 minutes, notify business operations leadership.

P1 alerts notify backend oncall and the relevant business or DBA receiver. If unacknowledged for 30 minutes, escalate to the module owner; if unresolved after 60 minutes, escalate to the backend lead.

P2 alerts go to the backend work queue/oncall channel and are reviewed within 1 hour. Repeated P2 alerts across two windows should be treated as a P1 candidate.

P3 alerts go to platform observability and are handled in the next working window. If observability loss masks P0/P1 symptoms, record observability as impaired in the incident.

## Business Confirmation Policy

Technical oncall may restart workers, restore database connectivity, fix scrape configuration, inspect metrics, query redacted audit/reconciliation data, and remove an expired maintenance silence without business approval.

Business operations confirmation is required before closing DLQ records, replaying DLQ or callback records after a customer-visible provider incident, pausing or resuming affected activities, releasing quota reservations through timeout-release, changing activity configuration to bypass a source/provider issue, or declaring customer impact. Provider-wide reward/source/fact outages require the relevant provider or source owner before changing production behavior.

Never bypass callback signatures, SSRF protections, provider allowlists, quota consistency checks, tenant isolation, or idempotency checks as a stopgap.

## Alert Runbooks

Each runbook follows the same shape: symptoms, impact, first checks, stopgap, recovery, verification, escalation, and review notes. Replace `$TENANT`, `$CHANNEL`, `$FACT_CODE`, and `$OUTBOX_ID` with values from alert labels or admin queries.

## ExMarketingWorkerAllStopped

Symptoms: P0 when eligible outbox backlog exists for a tenant/channel, oldest eligible age is above 120s, and `exmarketing_worker_claim_total{result="success"}` has no successful claim for 5 minutes.

Impact: reward commands stop moving from `PENDING` or `FAILED` to terminal sender states. User-visible reward delivery can be delayed; callback success is not expected until commands are delivered.

First checks:

```bash
curl -fsS http://localhost:8080/metrics | rg 'exmarketing_(outbox_backlog|outbox_oldest_eligible_age_seconds|worker_claim_total|worker_inflight)'
curl -G http://localhost:9090/api/v1/query --data-urlencode 'query=sum by (tenant_id,target_channel) (exmarketing_outbox_backlog{send_status=~"PENDING|FAILED"})'
curl -G http://localhost:9090/api/v1/query --data-urlencode 'query=sum by (tenant_id,target_channel) (rate(exmarketing_worker_claim_total{result="success"}[5m]))'
```

Open the alert dashboard, then check process manager logs for `cmd/exmarketing`, recent deploys, `EX_MARKETING_OUTBOX_WORKER`, sender configuration, and database P0 status.

Stopgap: restart or rescale workers only after excluding `ExMarketingDBWriteFailure` and planned maintenance. If one reward channel is isolated, route to that provider owner and consider pausing only the affected activity after business confirmation.

Recovery: restore worker process/config, verify successful claims resume, and let backlog drain. Do not manually mark outbox as sent.

Verification: `rate(exmarketing_worker_claim_total{result="success"}[5m]) > 0`, oldest eligible age trends down, `exmarketing_outbox_backlog{send_status=~"PENDING|FAILED"}` returns below threshold, and no new DLQ surge appears.

Escalation/review: P0 escalation applies. Record whether delay was customer-visible and whether reward provider callbacks caught up.

## ExMarketingDBWriteFailure

Symptoms: P0 when core write operations such as `event_inbox_write`, `participation_write`, `grant_order_write`, `quota_reserve_write`, `quota_finalize_write`, `outbox_write`, `outbox_dlq_write`, or `callback_write` fail.

Impact: event intake, grant order creation, quota movement, outbox transition, or callback finalization may be partially unavailable. Downstream outbox/callback/worker alerts can be noise from this root cause.

First checks:

```bash
curl -G http://localhost:9090/api/v1/query --data-urlencode 'query=sum by (operation,table,reason) (increase(exmarketing_db_operation_total{result="failed"}[5m]))'
curl -G http://localhost:9090/api/v1/query --data-urlencode 'query=histogram_quantile(0.95, sum by (operation,table,le) (rate(exmarketing_db_operation_latency_seconds_bucket[5m])))'
docker compose ps postgres
docker compose exec -T postgres pg_isready -U exmarketing -d exmarketing
```

Check PostgreSQL health, connection pool exhaustion, locks, migration drift, disk, and recent deploys. Use DB logs or managed DB console for root-cause detail.

Stopgap: restore DB availability first. Keep downstream flood alerts inhibited where configured, but do not silence this DB P0 or quota consistency alerts.

Recovery: after DB is healthy, resume services, verify writes succeed, and inspect reconciliation for affected tenant/channel/activity windows.

Verification: no increase in failed core writes for 5 minutes, DB latency returns to baseline, event intake and callback success recover, and pending outbox drains without unexplained DLQ.

Escalation/review: DBA/infra owns the alert; backend oncall provides impact and reconciliation data. Every unresolved or customer-visible P0 requires post-incident review.

## ExMarketingQuotaOverDeductRisk

Symptoms: P0 when quota aggregate conflicts or missing reservations are observed.

Impact: budget or user quota can be over-deducted, frozen incorrectly, or left inconsistent. Replay can amplify the issue if performed too early.

First checks:

```bash
curl -G http://localhost:9090/api/v1/query --data-urlencode 'query=sum by (tenant_id,quota_type,scope,reason) (increase(exmarketing_quota_fail_total{reason=~"quota_aggregate_conflict|quota_reservation_missing"}[5m]))'
curl 'http://localhost:8080/admin/v1/reward/reconciliation?tenantId=$TENANT&activityId=<redacted_activity>&limit=50'
curl 'http://localhost:8080/admin/v1/audit?tenantId=$TENANT&action=QUOTA_TIMEOUT_RELEASE&limit=100'
```

Inspect quota reservation rows, aggregate quota rows, callback statuses, and recent activity configuration changes for the affected tenant/quota type/scope.

Stopgap: pause affected rollout or freeze operations that can expand the inconsistency after business confirmation. Do not replay callback or DLQ and do not hand-edit used/frozen values until reconciliation identifies the root cause.

Recovery: reconcile reservations against grant orders and callbacks. Use `POST /admin/v1/quota-reservations/timeout-release` only for confirmed timeout cases with operator and reason.

Verification: zero new consistency failures, aggregate used/frozen matches reservations, reconciliation returns expected terminal status, and audit contains the repair actions.

Escalation/review: P0 escalation applies; business operations must be included. Every case requires review with prevention items.

## ExMarketingDLQGrowing

Symptoms: P1 when new outbox DLQ messages increase for a tenant/channel.

Impact: reward commands have exhausted retry and are no longer being delivered automatically. Depending on provider state, users can see delayed or failed rewards.

First checks:

```bash
curl -G http://localhost:9090/api/v1/query --data-urlencode 'query=sum by (tenant_id,target_channel,reason) (increase(exmarketing_outbox_dlq_total[10m]))'
curl 'http://localhost:8080/admin/v1/outbox/dlq?tenantId=$TENANT&includeClosed=false'
curl 'http://localhost:8080/admin/v1/reward/reconciliation?tenantId=$TENANT&outboxId=$OUTBOX_ID&limit=10'
curl 'http://localhost:8080/admin/v1/audit?tenantId=$TENANT&action=OUTBOX_ENTER_DLQ&limit=100'
```

Check sender configuration, provider health, payload validation, retry count, DB P0 status, and whether the failure is isolated to one channel.

Stopgap: do not bulk replay while root cause is active. If provider is down, pause affected activities only after business confirmation. If messages are invalid and should never be sent, get business confirmation before close.

Recovery: after root cause is resolved, replay current `DLQ` only:

```bash
curl -X POST "http://localhost:8080/admin/v1/outbox/$OUTBOX_ID/replay" -H 'Content-Type: application/json' -d '{"operator":"ops","reason":"root cause fixed"}'
```

Close only with business confirmation:

```bash
curl -X POST "http://localhost:8080/admin/v1/outbox/$OUTBOX_ID/close" -H 'Content-Type: application/json' -d '{"operator":"ops","reason":"business confirmed no replay"}'
```

Verification: `exmarketing_outbox_dlq_count` decreases or stabilizes, no new `exmarketing_outbox_dlq_total` growth, replayed messages return to `PENDING` then terminal sender state, and audit shows `OUTBOX_DLQ_REPLAY` or `OUTBOX_DLQ_CLOSE`.

Escalation/review: P1 escalation applies. Include business decision, affected counts, and replay/close evidence.

## ExMarketingOutboxBacklogHigh

Symptoms: P1 when eligible `PENDING|FAILED` backlog exceeds 100 and the oldest eligible age is above 60s for a tenant/channel.

Impact: reward command delivery latency SLO is at risk. User reward status can lag even if event intake succeeded.

First checks:

```bash
curl -G http://localhost:9090/api/v1/query --data-urlencode 'query=sum by (tenant_id,target_channel,send_status) (exmarketing_outbox_backlog{send_status=~"PENDING|FAILED|SENDING"})'
curl -G http://localhost:9090/api/v1/query --data-urlencode 'query=max by (tenant_id,target_channel) (exmarketing_outbox_oldest_eligible_age_seconds)'
curl -G http://localhost:9090/api/v1/query --data-urlencode 'query=histogram_quantile(0.95, sum by (tenant_id,target_channel,le) (rate(exmarketing_reward_command_latency_seconds_bucket[5m])))'
```

Check worker claim success, sender delivery latency, provider status, retry schedule, and DB latency.

Stopgap: fix DB P0 first if active. Scale workers only when DB and provider are healthy. For isolated provider degradation, involve provider owner and business operations before pausing activity.

Recovery: restore worker/sender/provider path, allow backlog to drain, and replay DLQ only if messages moved there after root cause is fixed.

Verification: backlog and oldest eligible age fall below threshold, reward command p95 is <= 120s, and DLQ does not grow.

Escalation/review: P1 escalation applies; attach SLO impact for reward command latency.

## ExMarketingCallbackFailRateHigh

Symptoms: P1 when callback failures, rejections, or conflicts are >= 5 and failure rate is > 5% over 5 minutes.

Impact: reward terminal state and quota finalize/release can be delayed or inconsistent. `SENT` outbox is not proof of real reward success.

First checks:

```bash
curl -G http://localhost:9090/api/v1/query --data-urlencode 'query=sum by (tenant_id,grant_channel,result,reason) (increase(exmarketing_reward_callback_total{result=~"rejected|failed|conflict"}[5m]))'
curl 'http://localhost:8080/admin/v1/reward/reconciliation?tenantId=$TENANT&commandId=<redacted_command>&limit=10'
curl 'http://localhost:8080/admin/v1/audit?tenantId=$TENANT&limit=100'
```

Check signature/timestamp failures, duplicate callbacks, terminal status conflicts, command locator mismatches, provider status codes, quota finalize DB failures, and tenant mismatch.

Stopgap: never disable signature validation as the default stopgap. If provider is sending bad payloads, coordinate with provider owner; pause provider callbacks or activities only with business confirmation.

Recovery: fix provider/auth/config/root DB issue. Use reconciliation to identify affected commands. Use quota timeout-release only for confirmed failed/timeout states with business approval where it changes business outcome.

Verification: callback failure rate falls below 5%, `exmarketing_reward_callback_total{result="accepted"}` resumes, reconciliation shows expected terminal states, and quota reservations are confirmed or released correctly.

Escalation/review: P1 escalation applies. Review any terminal conflicts or missing reservation as consistency incidents.

## ExMarketingEventIntakeErrorRateHigh

Symptoms: P2 when event intake failed/rejected count is >= 20 and error rate is > 5% over 5 minutes.

Impact: source events may not create participation, grant orders, quota reservations, or outbox commands.

First checks:

```bash
curl -G http://localhost:9090/api/v1/query --data-urlencode 'query=sum by (tenant_id,source_system,event_type,result,reason) (increase(exmarketing_event_intake_total{result=~"failed|rejected"}[5m]))'
curl -X POST http://localhost:8080/admin/v1/simulate -H 'Content-Type: application/json' -d '<redacted representative payload>'
curl 'http://localhost:8080/admin/v1/activities?tenantId=$TENANT'
```

Check source auth, schema changes, event type mapping, activity status, quota rejection, and recent configuration releases. Use redacted samples only.

Stopgap: reject bad source traffic normally; do not weaken auth or schema checks. If valid customer events are blocked, notify business operations and source owner.

Recovery: restore source contract/config/activity state and ask source owner to retry if their platform supports replay.

Verification: intake success rate returns to SLO, failed/rejected reasons stop increasing, and reconciliation/audit confirms representative events reached expected state.

Escalation/review: P2 within 1 hour; two repeated windows become a P1 candidate.

## ExMarketingClaimLatencyHigh

Symptoms: P2 when worker eligible-to-claim p95 is above 60s for 5 minutes.

Impact: reward command latency can degrade before backlog count crosses P1 threshold.

First checks:

```bash
curl -G http://localhost:9090/api/v1/query --data-urlencode 'query=histogram_quantile(0.95, sum by (tenant_id,target_channel,le) (rate(exmarketing_worker_claim_latency_seconds_bucket[5m])))'
curl -G http://localhost:9090/api/v1/query --data-urlencode 'query=sum by (worker_id,target_channel) (exmarketing_worker_inflight)'
curl -G http://localhost:9090/api/v1/query --data-urlencode 'query=sum by (tenant_id,target_channel,result) (rate(exmarketing_worker_claim_total[5m]))'
```

Check worker saturation, DB latency, retry scheduling, queue eligibility, and provider slowness.

Stopgap: scale workers if DB and provider are healthy and inflight/concurrency is the bottleneck. Avoid creating sender pressure during provider incidents.

Recovery: tune worker capacity or fix DB/provider root cause.

Verification: claim p95 <= 60s, successful claim rate is nonzero, and reward command latency recovers.

Escalation/review: P2 handling applies; customer-visible delay requires business notification.

## ExMarketingRewardRetrySpike

Symptoms: P2 when reward command retries are >= 20 in 5 minutes for a tenant/channel.

Impact: provider delivery is unstable and may turn into outbox backlog or DLQ.

First checks:

```bash
curl -G http://localhost:9090/api/v1/query --data-urlencode 'query=sum by (tenant_id,target_channel,reason) (increase(exmarketing_reward_command_total{result="retry"}[5m]))'
curl -G http://localhost:9090/api/v1/query --data-urlencode 'query=histogram_quantile(0.95, sum by (tenant_id,target_channel,le) (rate(exmarketing_reward_command_delivery_latency_seconds_bucket[5m])))'
```

Check sender timeout, auth, allowlist, TLS/DNS, provider status, and recent sender config changes.

Stopgap: do not close or replay messages while retry root cause is active. If provider is down, coordinate with provider owner and business operations.

Recovery: fix sender/provider path and let retry schedule drain naturally.

Verification: retry increase stops, sent rate recovers, backlog/DLQ do not grow, and command delivery latency returns to baseline.

Escalation/review: P2 handling applies; provider-wide incidents should be tracked with provider owner.

## ExMarketingFactQueryFailRateHigh

Symptoms: P2 when Fact Query failures are >= 10 and failure rate is > 10% over 5 minutes for a tenant/fact code.

Impact: activities depending on `facts.*` may reject or fail events. Safety failures are intentional and must not be bypassed.

First checks:

```bash
curl -G http://localhost:9090/api/v1/query --data-urlencode 'query=sum by (tenant_id,fact_code,result,reason) (increase(exmarketing_fact_query_total{result=~"failed|rejected|timeout|mapping_failed|field_missing|provider_unavailable"}[5m]))'
curl -G http://localhost:9090/api/v1/query --data-urlencode 'query=histogram_quantile(0.95, sum by (tenant_id,fact_code,le) (rate(exmarketing_fact_query_latency_seconds_bucket[5m])))'
curl 'http://localhost:8080/admin/v1/activities?tenantId=$TENANT'
```

Check endpoint availability, timeout rate, mapping failures, missing fields, activities referencing the fact code, allowlist/SSRF rejections, and recent provider schema changes.

Stopgap: do not relax SSRF, redirect, proxy, or allowlist restrictions. If provider-wide, coordinate with fact provider owner and business operations before disabling affected activities or rules.

Recovery: restore provider/config/mapping or roll back the activity configuration causing invalid fact references.

Verification: Fact Query success plus cache-hit rate returns to SLO, failure reasons stop increasing, and representative simulation/event paths work.

Escalation/review: P2 handling applies; provider-wide or security-related failures need provider/security owner notes in review.

## ExMarketingScrapeTargetDown

Symptoms: P3 when Prometheus `up{job="exmarketing"} == 0` for 2 minutes.

Impact: observability is degraded; business can still be healthy. Other business alerts may be blind or stale.

First checks:

```bash
curl -fsS http://localhost:8080/healthz
curl -fsS http://localhost:8080/metrics | head
promtool check config monitoring/prometheus-exmarketing.yml
```

Check service process, scrape target address, network route, Prometheus config reload, and Grafana datasource health.

Stopgap: restore scraping or add a narrow maintenance silence only when this is a planned metrics outage.

Recovery: fix target/config/network and reload Prometheus.

Verification: target returns to `up == 1`, `/metrics` emits ExMarketing families, and dashboard panels refresh.

Escalation/review: P3 handling applies.

## DB Slow Or Error Scenario

DB slow operations without a core write failure are handled through the DB latency dashboard and `exmarketing_db_operation_latency_seconds`. Treat sustained p95 above baseline as P2 unless it causes write failures or customer-visible degradation, then escalate according to the resulting alert.

```bash
curl -G http://localhost:9090/api/v1/query --data-urlencode 'query=histogram_quantile(0.95, sum by (operation,table,le) (rate(exmarketing_db_operation_latency_seconds_bucket[5m])))'
```

Fix query plan, lock, pool, disk, or migration issues with DBA/infra. Verify latency returns to baseline and no core write failures occurred during the window.

## SLI, SLO, And SLA

These are internal operational objectives unless a company-wide SLA says otherwise. They do not define customer compensation.

| Objective | SLI | SLO | Query or data source |
| --- | --- | --- | --- |
| Event intake availability | Successful intake divided by total intake excluding duplicates. | Monthly >= 99.5%. | `sum(increase(exmarketing_event_intake_total{result="success"}[30d])) / clamp_min(sum(increase(exmarketing_event_intake_total{result!="duplicate"}[30d])), 1)` |
| Reward command delivery latency | p95 from outbox creation to terminal sender result. | 5m p95 <= 120s; monthly compliant windows >= 99%. | `histogram_quantile(0.95, sum by (tenant_id,target_channel,le) (rate(exmarketing_reward_command_latency_seconds_bucket[5m])))` |
| Callback processing success | Accepted callback count divided by total callback count. | Monthly >= 99.0%; 5m failure rate must not stay > 5%. | `sum(increase(exmarketing_reward_callback_total{result="accepted"}[30d])) / clamp_min(sum(increase(exmarketing_reward_callback_total[30d])), 1)` |
| DLQ cleanup timeliness | Open DLQ age to replay or close. | P1 DLQ 95% cleaned within 24h; P0-linked DLQ has conclusion within 4h. | `exmarketing_outbox_dlq_count`, `exmarketing_outbox_dlq_total`, `GET /admin/v1/outbox/dlq`, audit actions `OUTBOX_DLQ_REPLAY` and `OUTBOX_DLQ_CLOSE`. |
| Data consistency | Quota consistency failures and core DB write failures. | Monthly 0 unexplained P0; every P0 reviewed. | `sum(increase(exmarketing_quota_fail_total{reason=~"quota_aggregate_conflict|quota_reservation_missing"}[30d]))` and `sum(increase(exmarketing_db_operation_total{result="failed"}[30d]))`. |
| Worker claim latency | p95 eligible outbox to worker claim. | 5m p95 <= 60s. | `histogram_quantile(0.95, sum by (tenant_id,target_channel,le) (rate(exmarketing_worker_claim_latency_seconds_bucket[5m])))` |
| Fact Query success | Success plus cache hit divided by total Fact Query. | Monthly >= 98.5%, with tenant/fact_code drilldown. | `sum(increase(exmarketing_fact_query_total{result=~"success|cache_hit"}[30d])) / clamp_min(sum(increase(exmarketing_fact_query_total[30d])), 1)` |

Use `rate` or `increase` for counters because process restarts reset absolute values. For low-volume windows, pair failure rates with minimum failure counts before paging.

## Drill Record Template

```markdown
# ExMarketing Alert Drill Record

- Drill ID:
- Date:
- Environment:
- Participants:
- Alert name:
- Severity:
- Tenant/channel/fact_code:
- Trigger or simulation method:
- Expected impact:
- Actual impact:
- Dashboard/API/query evidence:
- Triage timeline:
- Stopgap action:
- Recovery action:
- Verification evidence:
- Business confirmation needed: yes/no, approver:
- Issues found:
- Improvement item, owner, due date:
- Drill result: PASS/FAIL
```

## Incident Review Template

```markdown
# ExMarketing Incident Review

- Incident ID:
- Severity:
- Summary:
- Started at:
- Detected at:
- Acknowledged at:
- Stopgap at:
- Recovered at:
- Closed at:
- Affected tenant/channel/activity/source/reward provider/fact provider:
- Affected counts, redacted:
- Root cause category: code/config/DB/provider/source/capacity/observability/operator/unknown
- Timeline:
- Actions taken and effect:
- Data repair or compensation status:
- Customer/business communication:
- What worked:
- What failed:
- Prevention items, owner, due date:
- Verification for prevention:
```

## Alert To Runbook Mapping

| Alert | Severity | Owner | Dashboard | Runbook anchor | Key labels | Trigger summary | Business confirmation | Main recovery verification |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `ExMarketingWorkerAllStopped` | P0 | backend-oncall | `${GRAFANA_BASE_URL}/d/exmarketing/activity-engine?var-tenant={{ $labels.tenant_id }}&var-channel={{ $labels.target_channel }}` | `#exmarketingworkerallstopped` | `tenant_id`, `target_channel` | Eligible backlog present and no successful claim for 5m. | Conditional: required for activity pause/provider action. | Claim success resumes; backlog and oldest eligible age fall. |
| `ExMarketingDBWriteFailure` | P0 | dba-infra | `${GRAFANA_BASE_URL}/d/exmarketing/activity-engine?viewPanel=db` | `#exmarketingdbwritefailure` | `operation`, `table` | Core DB write failed. | No, but communicate impact after recovery. | Failed writes stop; DB latency and dependent flows recover. |
| `ExMarketingQuotaOverDeductRisk` | P0 | backend-oncall | `${GRAFANA_BASE_URL}/d/exmarketing/activity-engine?var-tenant={{ $labels.tenant_id }}&viewPanel=quota` | `#exmarketingquotaoverdeductrisk` | `tenant_id`, `quota_type`, `scope` | Quota aggregate conflict or reservation missing. | Yes. | Reconciliation clean; aggregate/reservation totals match. |
| `ExMarketingDLQGrowing` | P1 | backend-oncall | `${GRAFANA_BASE_URL}/d/exmarketing/activity-engine?var-tenant={{ $labels.tenant_id }}&var-channel={{ $labels.target_channel }}&viewPanel=dlq` | `#exmarketingdlqgrowing` | `tenant_id`, `target_channel`, `reason` | New DLQ messages increase. | Yes for replay/close business outcome. | DLQ count decreases/stabilizes; audit records replay/close. |
| `ExMarketingOutboxBacklogHigh` | P1 | backend-oncall | `${GRAFANA_BASE_URL}/d/exmarketing/activity-engine?var-tenant={{ $labels.tenant_id }}&var-channel={{ $labels.target_channel }}&viewPanel=outbox` | `#exmarketingoutboxbackloghigh` | `tenant_id`, `target_channel` | Eligible backlog > 100 and oldest eligible age > 60s. | Conditional for activity pause/provider action. | Backlog age and reward command p95 recover; no DLQ surge. |
| `ExMarketingCallbackFailRateHigh` | P1 | backend-oncall | `${GRAFANA_BASE_URL}/d/exmarketing/activity-engine?var-tenant={{ $labels.tenant_id }}&var-channel={{ $labels.grant_channel }}&viewPanel=callback` | `#exmarketingcallbackfailratehigh` | `tenant_id`, `grant_channel`, `result`, `reason` | Failed/rejected/conflict callbacks >= 5 and > 5%. | Yes for replay/pause/timeout-release. | Accepted callbacks resume; reconciliation and quota state clean. |
| `ExMarketingEventIntakeErrorRateHigh` | P2 | backend-oncall | `${GRAFANA_BASE_URL}/d/exmarketing/activity-engine?var-tenant={{ $labels.tenant_id }}&viewPanel=intake` | `#exmarketingeventintakeerrorratehigh` | `tenant_id`, `source_system`, `event_type` | Failed/rejected events >= 20 and > 5%. | Conditional when customer-visible/source retry needed. | Intake success SLO recovers; representative events reconcile. |
| `ExMarketingClaimLatencyHigh` | P2 | backend-oncall | `${GRAFANA_BASE_URL}/d/exmarketing/activity-engine?var-tenant={{ $labels.tenant_id }}&var-channel={{ $labels.target_channel }}&viewPanel=worker` | `#exmarketingclaimlatencyhigh` | `tenant_id`, `target_channel` | Worker claim p95 > 60s. | No unless customer-visible delay. | Claim p95 <= 60s and command latency recovers. |
| `ExMarketingRewardRetrySpike` | P2 | backend-oncall | `${GRAFANA_BASE_URL}/d/exmarketing/activity-engine?var-tenant={{ $labels.tenant_id }}&var-channel={{ $labels.target_channel }}&viewPanel=reward-command` | `#exmarketingrewardretryspike` | `tenant_id`, `target_channel`, `reason` | Reward command retries >= 20 over 5m. | Conditional for provider pause/activity pause. | Retry growth stops; sent rate recovers; DLQ stable. |
| `ExMarketingFactQueryFailRateHigh` | P2 | backend-oncall | `${GRAFANA_BASE_URL}/d/exmarketing/activity-engine?var-tenant={{ $labels.tenant_id }}&viewPanel=fact-query` | `#exmarketingfactqueryfailratehigh` | `tenant_id`, `fact_code`, `result`, `reason` | Fact Query failures >= 10 and > 10%. | Conditional, required for provider-wide mitigation. | Success/cache-hit rate recovers; simulation/event path works. |
| `ExMarketingScrapeTargetDown` | P3 | platform-observability | `${GRAFANA_BASE_URL}/d/exmarketing/activity-engine?viewPanel=scrape` | `#exmarketingscrapetargetdown` | `instance`, `job` | Prometheus cannot scrape target for 2m. | No. | `up == 1`; `/metrics` and dashboard panels recover. |
