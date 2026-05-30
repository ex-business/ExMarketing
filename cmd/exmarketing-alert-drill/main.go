package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

type server struct {
	mu            sync.Mutex
	startedAt     time.Time
	active        map[string]time.Time
	notifications []notification
}

type notification struct {
	ReceivedAt time.Time       `json:"receivedAt"`
	Receiver   string          `json:"receiver"`
	Status     string          `json:"status"`
	Alerts     []receivedAlert `json:"alerts"`
	Raw        json.RawMessage `json:"raw"`
}

type receivedAlert struct {
	Status      string            `json:"status"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	StartsAt    string            `json:"startsAt"`
	EndsAt      string            `json:"endsAt"`
}

func main() {
	addr := env("EX_MARKETING_DRILL_ADDR", ":8080")
	s := &server{
		startedAt: time.Now().UTC(),
		active:    map[string]time.Time{},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /metrics", s.handleMetrics)
	mux.HandleFunc("GET /drill/status", s.handleStatus)
	mux.HandleFunc("GET /drill/notifications", s.handleNotifications)
	mux.HandleFunc("POST /drill/notifications/clear", s.handleClearNotifications)
	mux.HandleFunc("POST /drill/cleanup", s.handleCleanup)
	mux.HandleFunc("POST /drill/scenario/{scenario}/trigger", s.handleTrigger)
	mux.HandleFunc("POST /drill/scenario/{scenario}/recover", s.handleRecover)
	mux.HandleFunc("POST /alertmanager/{receiver}", s.handleAlertmanager)

	log.Printf("exmarketing alert drill listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

func (s *server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	now := time.Now().UTC()

	s.mu.Lock()
	active := make(map[string]time.Time, len(s.active))
	for scenario, startedAt := range s.active {
		active[scenario] = startedAt
	}
	startedAt := s.startedAt
	s.mu.Unlock()

	m := newMetrics()
	uptime := now.Sub(startedAt).Seconds()
	baselineTicks := 1 + int(uptime/15)

	m.counter("exmarketing_worker_claim_total", float64(baselineTicks), labels{
		"worker_id": "drill-worker-baseline", "tenant_id": "drill_baseline", "target_channel": "drill_wallet", "result": "success",
	})
	m.counter("exmarketing_reward_callback_total", float64(baselineTicks), labels{
		"tenant_id": "drill_baseline", "grant_channel": "drill_wallet", "callback_status": "ACCEPTED", "result": "accepted", "reason": "ok",
	})
	m.counter("exmarketing_quota_attempt_total", float64(baselineTicks), labels{
		"tenant_id": "drill_baseline", "quota_type": "BUDGET", "scope": "ACTIVITY", "action": "reserve", "result": "success",
	})
	m.counter("exmarketing_db_operation_total", float64(baselineTicks), labels{
		"operation": "grant_order_write", "table": "marketing_grant_order", "result": "success", "reason": "ok",
	})
	m.histogram("exmarketing_db_operation_latency_seconds", []float64{0.02, 0.03, 0.04}, labels{
		"operation": "grant_order_write", "table": "marketing_grant_order", "result": "success",
	})

	for scenario, scenarioStartedAt := range active {
		elapsed := now.Sub(scenarioStartedAt).Seconds()
		if elapsed < 0 {
			elapsed = 0
		}
		ticks := float64(1 + int(elapsed/10))
		switch scenario {
		case "worker_stopped":
			m.gauge("exmarketing_outbox_backlog", 1, labels{
				"tenant_id": "drill_worker", "target_channel": "drill_wallet", "send_status": "PENDING",
			})
			m.gauge("exmarketing_outbox_oldest_eligible_age_seconds", 121+elapsed, labels{
				"tenant_id": "drill_worker", "target_channel": "drill_wallet",
			})
		case "outbox_backlog":
			m.gauge("exmarketing_outbox_backlog", 101, labels{
				"tenant_id": "drill_outbox", "target_channel": "drill_wallet", "send_status": "PENDING",
			})
			m.gauge("exmarketing_outbox_oldest_eligible_age_seconds", 61+elapsed, labels{
				"tenant_id": "drill_outbox", "target_channel": "drill_wallet",
			})
		case "dlq_growing":
			m.counter("exmarketing_outbox_dlq_total", 5+ticks, labels{
				"tenant_id": "drill_dlq", "target_channel": "drill_wallet", "reason": "send_failed",
			})
			m.gauge("exmarketing_outbox_dlq_count", 5, labels{
				"tenant_id": "drill_dlq", "target_channel": "drill_wallet",
			})
		case "callback_failures":
			m.counter("exmarketing_reward_callback_total", 5+ticks, labels{
				"tenant_id": "drill_callback", "grant_channel": "drill_wallet", "callback_status": "FAILED", "result": "failed", "reason": "provider_error",
			})
			m.counter("exmarketing_reward_callback_total", 1, labels{
				"tenant_id": "drill_callback", "grant_channel": "drill_wallet", "callback_status": "ACCEPTED", "result": "accepted", "reason": "ok",
			})
			m.histogram("exmarketing_reward_callback_latency_seconds", []float64{0.03, 0.05, 0.08}, labels{
				"tenant_id": "drill_callback", "grant_channel": "drill_wallet", "result": "failed",
			})
		case "db_write_failure":
			m.counter("exmarketing_db_operation_total", 1+ticks, labels{
				"operation": "grant_order_write", "table": "marketing_grant_order", "result": "failed", "reason": "db_error",
			})
			m.histogram("exmarketing_db_operation_latency_seconds", []float64{0.7, 1.2, 2.4}, labels{
				"operation": "grant_order_write", "table": "marketing_grant_order", "result": "failed",
			})
		case "db_slow_query":
			m.histogram("exmarketing_db_operation_latency_seconds", []float64{1.3, 1.8, 2.2, 3.1}, labels{
				"operation": "outbox_backlog_read", "table": "marketing_outbox", "result": "success",
			})
		case "quota_consistency":
			m.counter("exmarketing_quota_fail_total", 1+ticks, labels{
				"tenant_id": "drill_quota", "quota_type": "BUDGET", "scope": "ACTIVITY", "action": "confirm", "reason": "quota_aggregate_conflict",
			})
		case "quota_insufficient":
			m.counter("exmarketing_quota_attempt_total", 20+ticks, labels{
				"tenant_id": "drill_quota_insufficient", "quota_type": "BUDGET", "scope": "ACTIVITY", "action": "reserve", "result": "failed",
			})
			m.counter("exmarketing_quota_fail_total", 20+ticks, labels{
				"tenant_id": "drill_quota_insufficient", "quota_type": "BUDGET", "scope": "ACTIVITY", "action": "reserve", "reason": "quota_insufficient",
			})
		}
	}

	_, _ = io.WriteString(w, m.render())
}

func (s *server) handleTrigger(w http.ResponseWriter, r *http.Request) {
	scenario := r.PathValue("scenario")
	if !knownScenario(scenario) {
		http.Error(w, "unknown scenario", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	if _, ok := s.active[scenario]; !ok {
		s.active[scenario] = time.Now().UTC()
	}
	s.mu.Unlock()
	writeJSON(w, map[string]string{"scenario": scenario, "state": "triggered"})
}

func (s *server) handleRecover(w http.ResponseWriter, r *http.Request) {
	scenario := r.PathValue("scenario")
	if !knownScenario(scenario) {
		http.Error(w, "unknown scenario", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	delete(s.active, scenario)
	s.mu.Unlock()
	writeJSON(w, map[string]string{"scenario": scenario, "state": "recovered"})
}

func (s *server) handleCleanup(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.active = map[string]time.Time{}
	s.mu.Unlock()
	writeJSON(w, map[string]string{"state": "clean"})
}

func (s *server) handleStatus(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	active := make(map[string]string, len(s.active))
	for scenario, startedAt := range s.active {
		active[scenario] = startedAt.Format(time.RFC3339)
	}
	count := len(s.notifications)
	s.mu.Unlock()
	writeJSON(w, map[string]any{"active": active, "notifications": count})
}

func (s *server) handleNotifications(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	notifications := append([]notification(nil), s.notifications...)
	s.mu.Unlock()
	writeJSON(w, notifications)
}

func (s *server) handleClearNotifications(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.notifications = nil
	s.mu.Unlock()
	writeJSON(w, map[string]string{"notifications": "cleared"})
}

func (s *server) handleAlertmanager(w http.ResponseWriter, r *http.Request) {
	receiver := r.PathValue("receiver")
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var payload struct {
		Status string          `json:"status"`
		Alerts []receivedAlert `json:"alerts"`
	}
	_ = json.Unmarshal(body, &payload)
	s.mu.Lock()
	s.notifications = append(s.notifications, notification{
		ReceivedAt: time.Now().UTC(),
		Receiver:   receiver,
		Status:     payload.Status,
		Alerts:     payload.Alerts,
		Raw:        append(json.RawMessage(nil), body...),
	})
	s.mu.Unlock()
	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte("ok\n"))
}

func knownScenario(scenario string) bool {
	switch scenario {
	case "worker_stopped", "outbox_backlog", "dlq_growing", "callback_failures", "db_write_failure", "db_slow_query", "quota_consistency", "quota_insufficient":
		return true
	default:
		return false
	}
}

type labels map[string]string

type metricSample struct {
	name   string
	value  float64
	labels labels
}

type histogramSample struct {
	name    string
	values  []float64
	labels  labels
	buckets []float64
}

type metrics struct {
	counters   []metricSample
	gauges     []metricSample
	histograms []histogramSample
}

func newMetrics() *metrics {
	return &metrics{}
}

func (m *metrics) counter(name string, value float64, l labels) {
	m.counters = append(m.counters, metricSample{name: name, value: value, labels: l})
}

func (m *metrics) gauge(name string, value float64, l labels) {
	m.gauges = append(m.gauges, metricSample{name: name, value: value, labels: l})
}

func (m *metrics) histogram(name string, values []float64, l labels) {
	m.histograms = append(m.histograms, histogramSample{
		name:    name,
		values:  values,
		labels:  l,
		buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120, 300},
	})
}

func (m *metrics) render() string {
	var b strings.Builder
	writeFamily(&b, "exmarketing_event_intake_total", "counter", "Event API intake results.")
	writeFamily(&b, "exmarketing_quota_attempt_total", "counter", "Quota reserve, confirm, and release attempts.")
	writeFamily(&b, "exmarketing_quota_fail_total", "counter", "Quota failed attempts.")
	writeFamily(&b, "exmarketing_outbox_backlog", "gauge", "Current outbox backlog by status.")
	writeFamily(&b, "exmarketing_outbox_oldest_eligible_age_seconds", "gauge", "Oldest eligible outbox age in seconds.")
	writeFamily(&b, "exmarketing_outbox_dlq_count", "gauge", "Current open DLQ outbox count.")
	writeFamily(&b, "exmarketing_outbox_dlq_total", "counter", "Outbox messages newly moved to DLQ.")
	writeFamily(&b, "exmarketing_reward_callback_total", "counter", "Reward callback results.")
	writeFamily(&b, "exmarketing_reward_callback_latency_seconds", "histogram", "Reward callback processing latency in seconds.")
	writeFamily(&b, "exmarketing_worker_claim_total", "counter", "Outbox worker claim attempts.")
	writeFamily(&b, "exmarketing_worker_claim_latency_seconds", "histogram", "Outbox eligible-to-claim latency in seconds.")
	writeFamily(&b, "exmarketing_reward_command_latency_seconds", "histogram", "Outbox create-to-terminal delivery latency in seconds.")
	writeFamily(&b, "exmarketing_db_operation_total", "counter", "Database operation results.")
	writeFamily(&b, "exmarketing_db_operation_latency_seconds", "histogram", "Database operation latency in seconds.")

	for _, sample := range appendSorted(nil, m.counters...) {
		fmt.Fprintf(&b, "%s%s %.0f\n", sample.name, renderLabels(sample.labels), sample.value)
	}
	for _, sample := range appendSorted(nil, m.gauges...) {
		fmt.Fprintf(&b, "%s%s %g\n", sample.name, renderLabels(sample.labels), sample.value)
	}
	for _, sample := range m.histograms {
		sort.Float64s(sample.values)
		sum := 0.0
		for _, value := range sample.values {
			sum += value
		}
		for _, bucket := range sample.buckets {
			count := 0
			for _, value := range sample.values {
				if value <= bucket {
					count++
				}
			}
			fmt.Fprintf(&b, "%s_bucket%s %d\n", sample.name, renderLabels(withLabel(sample.labels, "le", formatBucket(bucket))), count)
		}
		fmt.Fprintf(&b, "%s_bucket%s %d\n", sample.name, renderLabels(withLabel(sample.labels, "le", "+Inf")), len(sample.values))
		fmt.Fprintf(&b, "%s_sum%s %g\n", sample.name, renderLabels(sample.labels), sum)
		fmt.Fprintf(&b, "%s_count%s %d\n", sample.name, renderLabels(sample.labels), len(sample.values))
	}
	return b.String()
}

func writeFamily(b *strings.Builder, name, typ, help string) {
	fmt.Fprintf(b, "# HELP %s %s\n# TYPE %s %s\n", name, help, name, typ)
}

func appendSorted(dst []metricSample, samples ...metricSample) []metricSample {
	dst = append(dst, samples...)
	sort.Slice(dst, func(i, j int) bool {
		left := dst[i].name + renderLabels(dst[i].labels)
		right := dst[j].name + renderLabels(dst[j].labels)
		return left < right
	})
	return dst
}

func renderLabels(l labels) string {
	if len(l) == 0 {
		return ""
	}
	keys := make([]string, 0, len(l))
	for key := range l {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf(`%s="%s"`, key, escape(l[key])))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func withLabel(l labels, key, value string) labels {
	next := make(labels, len(l)+1)
	for k, v := range l {
		next[k] = v
	}
	next[key] = value
	return next
}

func formatBucket(value float64) string {
	text := fmt.Sprintf("%g", value)
	if !strings.Contains(text, ".") && !strings.Contains(text, "e") {
		return text
	}
	return text
}

func escape(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, "\n", `\n`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return value
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(value)
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
