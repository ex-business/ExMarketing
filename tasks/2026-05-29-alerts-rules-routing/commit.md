# Commit

## Scope

Committed the alerts-rules-routing task implementation:

- ExMarketing Prometheus alert rules and promtool tests.
- Alertmanager routing, receivers, inhibition rules, and message template.
- Alert runbook and operations documentation updates.
- Fact Query Prometheus metrics and tests.
- Outbox worker claim-release fix and regression test.
- Task artifacts excluding `tasks/*/runs/` execution logs.

Excluded unrelated workspace changes, including other task directories and `requirements/backlog.json`.

## Review

```text
PASS
```

Source: `tasks/2026-05-29-alerts-rules-routing/review.md`

## Test

```text
PASS
```

Source: `tasks/2026-05-29-alerts-rules-routing/test.md`

## Pre-Commit Checks

```text
git status -sb: PASS
git diff --check: PASS
go test ./...: PASS
go build ./...: PASS
```

## Commit

```text
hash: 850b1740c166f127217f3e8d2f9778c5cc24593a
message: Add ExMarketing alert rules and routing
remote: not pushed
```

需求：2026-05-29-alerts-rules-routing
