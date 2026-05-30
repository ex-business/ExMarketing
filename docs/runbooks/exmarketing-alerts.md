# ExMarketing Alert Runbook

## Common Fields

Every alert includes `service`, `tenant`, `channel`, `severity`, `summary`, `dashboard`, `runbook`, `owner`, `current_value`, and `threshold`. Alert labels must stay low-cardinality; use audit, reconciliation, admin queries, or redacted logs for event, activity, subject, order, command, or idempotency-key lookup.

Maintenance silences should match `service="exmarketing"` plus the narrowest safe combination of `env`, `tenant`, `channel`, `severity`, and `alertname`. Planned maintenance silences require owner, reason, start/end time, expected impact, rollback contact, and a default maximum duration of 4 hours unless approved by oncall and business operations.

## Escalation

P0 alerts page backend oncall or DBA/infra immediately. If unacknowledged for 10 minutes, escalate to backend and infra leads; if unresolved after 20 minutes, notify business operations leadership.

P1 alerts notify backend oncall and the relevant business or DBA receiver. If unacknowledged for 30 minutes, escalate to the module owner; if unresolved after 60 minutes, escalate to the backend lead.

P2 alerts go to the backend work queue/oncall channel and are reviewed within 1 hour. Repeated P2 alerts across two windows should be treated as a P1 candidate.

P3 alerts go to platform observability and are handled in the next working window.

## ExMarketingWorkerAllStopped

Check the worker process, recent deployments, `exmarketing_worker_claim_total`, outbox backlog by `tenant/channel`, and reward sender connectivity. Restart workers only after confirming no DB write P0 is active and no maintenance silence is in effect.

## ExMarketingDBWriteFailure

Check PostgreSQL availability, connection pool saturation, migration status, and failed write path labels `operation` and `table`. Keep downstream flood alerts inhibited, but do not silence quota consistency alerts or the DB alert itself.

## ExMarketingQuotaOverDeductRisk

Stop affected activity rollout if the alert is active. Inspect quota reservations and aggregate quota rows for the affected tenant and quota type. Reconcile frozen/used totals before replaying callbacks or DLQ messages.

## ExMarketingDLQGrowing

Inspect DLQ reason distribution for the affected tenant and channel. Confirm whether the root cause is sender configuration, external reward provider, payload validation, or DB write failure before replaying.

## ExMarketingOutboxBacklogHigh

Check worker concurrency, sender health, retry delay, and eligible age. If DB P0 is active, fix DB first. If only one channel is affected, route to the reward provider owner.

## ExMarketingCallbackFailRateHigh

Check callback authentication, duplicate/conflict rates, provider status codes, and quota finalize writes. Confirm whether business operations need to pause provider callbacks or replay failed callbacks.

## ExMarketingEventIntakeErrorRateHigh

Check source system error reasons, API auth changes, schema validation failures, and recent activity configuration changes. Avoid paging business operations unless customer-visible event intake is degraded.

## ExMarketingClaimLatencyHigh

Check worker saturation, DB latency, pending outbox age, and retry scheduling. Scale workers only after excluding DB write or external provider root causes.

## ExMarketingRewardRetrySpike

Check reward sender timeout/auth/allowlist errors and external provider status. Confirm whether retry volume is isolated to one tenant/channel or is platform-wide.

## ExMarketingFactQueryFailRateHigh

Check fact provider endpoint availability, timeout rate, mapping failures, field-missing reasons, and activities that reference `facts.*`. If the failure is provider-wide, coordinate with the provider owner before changing rule configuration.

## ExMarketingScrapeTargetDown

Check service process health, `/metrics` reachability, Prometheus target config, and network routing. This alert is inhibited by stronger service-level symptoms where configured by the alerting platform.
