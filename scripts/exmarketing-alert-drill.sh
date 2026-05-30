#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PROM_URL="${PROM_URL:-http://localhost:9090}"
AM_URL="${AM_URL:-http://localhost:9093}"
DRILL_URL="${DRILL_URL:-http://localhost:8080}"
WAIT_TIMEOUT_SECONDS="${WAIT_TIMEOUT_SECONDS:-900}"
DRILL_ID="${DRILL_ID:-alerts-production-drill-$(date -u +%Y%m%dT%H%M%SZ)}"
REPORT_DIR="${REPORT_DIR:-$ROOT_DIR/tasks/2026-05-29-alerts-production-drill/runs}"
REPORT_FILE="${REPORT_FILE:-$REPORT_DIR/${DRILL_ID}-live-report.md}"

SCENARIOS=(
  "worker_stopped:ExMarketingWorkerAllStopped:backend-oncall-p0"
  "outbox_backlog:ExMarketingOutboxBacklogHigh:backend-oncall"
  "dlq_growing:ExMarketingDLQGrowing:backend-oncall,business-ops"
  "callback_failures:ExMarketingCallbackFailRateHigh:backend-oncall,business-ops"
  "db_write_failure:ExMarketingDBWriteFailure:dba-infra-p0"
  "quota_consistency:ExMarketingQuotaOverDeductRisk:backend-oncall-p0,business-ops"
)

usage() {
  cat <<USAGE
Usage: $0 [--start] [--scenario NAME] [--no-compose]

Runs the local ExMarketing alert drill against the compose harness.

Environment:
  WAIT_TIMEOUT_SECONDS  Per firing/resolved wait timeout, default 900.
  DRILL_ID              Report id, default alerts-production-drill-UTC.
  PROM_URL              Default http://localhost:9090.
  AM_URL                Default http://localhost:9093.
  DRILL_URL             Default http://localhost:8080.
USAGE
}

START_COMPOSE=false
NO_COMPOSE=false
ONLY_SCENARIO=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --start)
      START_COMPOSE=true
      shift
      ;;
    --no-compose)
      NO_COMPOSE=true
      shift
      ;;
    --scenario)
      ONLY_SCENARIO="${2:-}"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

wait_url() {
  local url="$1"
  local name="$2"
  local deadline=$((SECONDS + WAIT_TIMEOUT_SECONDS))
  until curl -fsS "$url" >/dev/null; do
    if (( SECONDS >= deadline )); then
      echo "timeout waiting for $name at $url" >&2
      return 1
    fi
    sleep 2
  done
}

notifications_contain() {
  local alert="$1"
  local status="$2"
  local receiver="${3:-}"
  local payload
  payload="$(curl -fsS "$DRILL_URL/drill/notifications" | tr -d '\n')"
  if [[ -n "$receiver" ]]; then
    grep -F "\"receiver\": \"$receiver\"" <<<"$payload" | grep -F "\"status\": \"$status\"" | grep -F "\"alertname\": \"$alert\"" >/dev/null
  else
    grep -F "\"status\": \"$status\"" <<<"$payload" | grep -F "\"alertname\": \"$alert\"" >/dev/null
  fi
}

wait_notification() {
  local alert="$1"
  local status="$2"
  local receiver="${3:-}"
  local deadline=$((SECONDS + WAIT_TIMEOUT_SECONDS))
  until notifications_contain "$alert" "$status" "$receiver"; do
    if (( SECONDS >= deadline )); then
      echo "timeout waiting for $status notification for $alert receiver=${receiver:-any}" >&2
      curl -fsS "$DRILL_URL/drill/notifications" >&2 || true
      return 1
    fi
    sleep 5
  done
}

query_prometheus() {
  local query="$1"
  curl -fsS -G "$PROM_URL/api/v1/query" --data-urlencode "query=$query"
}

scenario_enabled() {
  local scenario="$1"
  [[ -z "$ONLY_SCENARIO" || "$ONLY_SCENARIO" == "$scenario" ]]
}

append_report_header() {
  mkdir -p "$REPORT_DIR"
  cat > "$REPORT_FILE" <<REPORT
# ExMarketing Alert Drill Live Report

- Drill ID: \`$DRILL_ID\`
- Environment: local compose harness
- Started at: $(date -u +%Y-%m-%dT%H:%M:%SZ)
- Prometheus: \`$PROM_URL\`
- Alertmanager: \`$AM_URL\`
- Drill metrics/receiver: \`$DRILL_URL\`

| Scenario | Alert | Expected receivers | Firing evidence | Resolved evidence | Cleanup evidence | Result |
| --- | --- | --- | --- | --- | --- | --- |
REPORT
}

append_report_row() {
  local scenario="$1"
  local alert="$2"
  local receivers="$3"
  local result="$4"
  printf '| `%s` | `%s` | `%s` | notification recorded | notification recorded | `/drill/scenario/%s/recover` and `/drill/cleanup` | %s |\n' \
    "$scenario" "$alert" "$receivers" "$scenario" "$result" >> "$REPORT_FILE"
}

append_report_footer() {
  cat >> "$REPORT_FILE" <<REPORT

## Notification Payloads

\`\`\`json
$(curl -fsS "$DRILL_URL/drill/notifications")
\`\`\`

## Final Cleanup Evidence

\`\`\`json
$(curl -fsS "$DRILL_URL/drill/status")
\`\`\`

Finished at: $(date -u +%Y-%m-%dT%H:%M:%SZ)
REPORT
}

cd "$ROOT_DIR"

if [[ "$START_COMPOSE" == "true" && "$NO_COMPOSE" != "true" && "$DRILL_URL" == "http://localhost:8080" ]]; then
  if ! curl -fsS "$DRILL_URL/drill/status" >/dev/null 2>&1 && lsof -nP -iTCP:8080 -sTCP:LISTEN >/dev/null 2>&1; then
    export EX_MARKETING_DRILL_HOST_PORT="${EX_MARKETING_DRILL_HOST_PORT:-18080}"
    DRILL_URL="http://localhost:$EX_MARKETING_DRILL_HOST_PORT"
    echo "localhost:8080 is occupied by a non-drill service; using $DRILL_URL for the local drill receiver" >&2
  fi
fi

if [[ "$START_COMPOSE" == "true" && "$NO_COMPOSE" != "true" ]]; then
  docker compose up -d postgres prometheus alertmanager
fi

wait_url "$DRILL_URL/metrics" "drill metrics"
wait_url "$PROM_URL/-/ready" "prometheus"
wait_url "$AM_URL/-/ready" "alertmanager"

curl -fsS -X POST "$DRILL_URL/drill/cleanup" >/dev/null
curl -fsS -X POST "$DRILL_URL/drill/notifications/clear" >/dev/null
append_report_header

for item in "${SCENARIOS[@]}"; do
  IFS=: read -r scenario alert receivers <<<"$item"
  if ! scenario_enabled "$scenario"; then
    continue
  fi

  echo "triggering $scenario -> $alert"
  curl -fsS -X POST "$DRILL_URL/drill/scenario/$scenario/trigger" >/dev/null
  query_prometheus 'up{job="exmarketing"}' >/dev/null
  IFS=, read -ra receiver_list <<<"$receivers"
  for receiver in "${receiver_list[@]}"; do
    wait_notification "$alert" "firing" "$receiver"
  done

  echo "recovering $scenario -> $alert"
  curl -fsS -X POST "$DRILL_URL/drill/scenario/$scenario/recover" >/dev/null
  for receiver in "${receiver_list[@]}"; do
    wait_notification "$alert" "resolved" "$receiver"
  done
  curl -fsS -X POST "$DRILL_URL/drill/cleanup" >/dev/null
  append_report_row "$scenario" "$alert" "$receivers" "PASS"
done

append_report_footer
echo "$REPORT_FILE"
