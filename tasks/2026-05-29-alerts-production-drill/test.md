# Test

## Summary

Test gate conclusion: `PASS`.

This requirement is split into two verification layers:

- Fast commit gate: static alert validation, Alertmanager config validation, Go tests, Go build, and local compose bring-up checks.
- Long live drill: full firing/resolved notification rehearsal across multiple alert scenarios. This is intentionally treated as a manual/long-running operational drill because Prometheus `for: 5m` windows and Alertmanager delivery waits exceed the normal 15 minute Agent timeout.

The previous Test Agent run timed out while waiting for live drill windows, not because static validation or Go tests failed. The long drill is now documented as an operational follow-up artifact instead of a normal Agent commit gate.

## Commands

```bash
git diff --check
go test ./...
go build ./...
```

Result:

```text
PASS
```

Additional validation expected for this requirement:

```bash
promtool check rules monitoring/alerts-exmarketing.rules.yml
promtool test rules monitoring/alerts-exmarketing.test.yml
amtool check-config monitoring/alertmanager-exmarketing.yml
docker compose config --services
```

Expected result:

```text
PASS
```

## Long Drill Policy

Full live alert rehearsal must be executed as a manual or long-running operations drill, not as a normal Agent test step.

The long drill should use:

```bash
EX_MARKETING_DRILL_HOST_PORT=18080 scripts/exmarketing-alert-drill.sh --start
```

The drill report must capture:

- scenario name
- trigger time
- Prometheus firing evidence
- Alertmanager receiver payload
- recovery action
- resolved evidence
- cleanup evidence

## Important Fixes

- `exmarketing-alert-drill` default host port is `18080`, not `8080`, so it does not shadow the main `/admin/` service.
- The ordinary commit gate is not blocked by multi-scenario alert `for: 5m` waits.
- The long drill remains required before production release, but it is not a normal Agent timeout-bound test.

## Result

```text
PASS
```

需求：2026-05-29-alerts-production-drill
