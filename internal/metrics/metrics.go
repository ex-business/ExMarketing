package metrics

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var Default = NewRegistry()

type Registry struct {
	mu         sync.Mutex
	counters   map[string]float64
	gauges     map[string]float64
	histograms map[string]*histogram
}

type histogram struct {
	buckets []float64
	counts  []uint64
	sum     float64
	count   uint64
}

type Label struct {
	Name  string
	Value string
}

func L(name, value string) Label {
	return Label{Name: safeLabelName(name), Value: safeLabelValue(value)}
}

func NewRegistry() *Registry {
	return &Registry{
		counters:   map[string]float64{},
		gauges:     map[string]float64{},
		histograms: map[string]*histogram{},
	}
}

func (r *Registry) IncCounter(name string, labels ...Label) {
	r.AddCounter(name, 1, labels...)
}

func (r *Registry) AddCounter(name string, value float64, labels ...Label) {
	if r == nil || value <= 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.counters[key(name, labels)] += value
}

func (r *Registry) SetGauge(name string, value float64, labels ...Label) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.gauges[key(name, labels)] = value
}

func (r *Registry) DeleteMetric(name string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	deleteByMetricName(r.counters, name)
	deleteByMetricName(r.gauges, name)
	deleteByMetricName(r.histograms, name)
}

func (r *Registry) Observe(name string, value float64, labels ...Label) {
	if r == nil || value < 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	k := key(name, labels)
	h := r.histograms[k]
	if h == nil {
		buckets := []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120, 300}
		h = &histogram{buckets: buckets, counts: make([]uint64, len(buckets))}
		r.histograms[k] = h
	}
	for i, bucket := range h.buckets {
		if value <= bucket {
			h.counts[i]++
		}
	}
	h.sum += value
	h.count++
}

func deleteByMetricName[V any](values map[string]V, name string) {
	for key := range values {
		if key == name || strings.HasPrefix(key, name+"{") {
			delete(values, key)
		}
	}
}

func ObserveDuration(r *Registry, name string, startedAt time.Time, labels ...Label) {
	if startedAt.IsZero() {
		return
	}
	r.Observe(name, time.Since(startedAt).Seconds(), labels...)
}

func (r *Registry) RenderPrometheus() []byte {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	var out bytes.Buffer
	writeMetricFamily(&out, "exmarketing_event_intake_total", "counter", "Event API intake results.")
	writeMetricFamily(&out, "exmarketing_event_intake_latency_seconds", "histogram", "Event API processing latency in seconds.")
	writeMetricFamily(&out, "exmarketing_quota_attempt_total", "counter", "Quota reserve, confirm, and release attempts.")
	writeMetricFamily(&out, "exmarketing_quota_fail_total", "counter", "Quota failed attempts.")
	writeMetricFamily(&out, "exmarketing_outbox_backlog", "gauge", "Current outbox backlog by status.")
	writeMetricFamily(&out, "exmarketing_outbox_oldest_eligible_age_seconds", "gauge", "Oldest eligible outbox age in seconds.")
	writeMetricFamily(&out, "exmarketing_outbox_dlq_count", "gauge", "Current open DLQ outbox count.")
	writeMetricFamily(&out, "exmarketing_outbox_dlq_total", "counter", "Outbox messages newly moved to DLQ.")
	writeMetricFamily(&out, "exmarketing_reward_callback_total", "counter", "Reward callback results.")
	writeMetricFamily(&out, "exmarketing_reward_callback_latency_seconds", "histogram", "Reward callback processing latency in seconds.")
	writeMetricFamily(&out, "exmarketing_worker_claim_total", "counter", "Outbox worker claim attempts.")
	writeMetricFamily(&out, "exmarketing_worker_claim_latency_seconds", "histogram", "Outbox eligible-to-claim latency in seconds.")
	writeMetricFamily(&out, "exmarketing_worker_inflight", "gauge", "Outbox messages currently processed by worker.")
	writeMetricFamily(&out, "exmarketing_reward_command_total", "counter", "Reward command delivery results.")
	writeMetricFamily(&out, "exmarketing_reward_command_latency_seconds", "histogram", "Outbox create-to-terminal delivery latency in seconds.")
	writeMetricFamily(&out, "exmarketing_reward_command_delivery_latency_seconds", "histogram", "Single external reward command delivery latency in seconds.")

	for _, k := range sortedKeys(r.counters) {
		fmt.Fprintf(&out, "%s %s\n", k, formatFloat(r.counters[k]))
	}
	for _, k := range sortedKeys(r.gauges) {
		fmt.Fprintf(&out, "%s %s\n", k, formatFloat(r.gauges[k]))
	}
	for _, k := range sortedKeys(r.histograms) {
		h := r.histograms[k]
		base, labels := splitKey(k)
		for i, bucket := range h.buckets {
			fmt.Fprintf(&out, "%s_bucket%s %d\n", base, appendLabel(labels, "le", formatFloat(bucket)), h.counts[i])
		}
		fmt.Fprintf(&out, "%s_bucket%s %d\n", base, appendLabel(labels, "le", "+Inf"), h.count)
		fmt.Fprintf(&out, "%s_sum%s %s\n", base, labels, formatFloat(h.sum))
		fmt.Fprintf(&out, "%s_count%s %d\n", base, labels, h.count)
	}
	return out.Bytes()
}

func writeMetricFamily(out *bytes.Buffer, name, typ, help string) {
	fmt.Fprintf(out, "# HELP %s %s\n# TYPE %s %s\n", name, help, name, typ)
}

func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func key(name string, labels []Label) string {
	name = safeMetricName(name)
	if len(labels) == 0 {
		return name
	}
	normalized := append([]Label(nil), labels...)
	sort.Slice(normalized, func(i, j int) bool { return normalized[i].Name < normalized[j].Name })
	parts := make([]string, 0, len(normalized))
	for _, label := range normalized {
		parts = append(parts, label.Name+`="`+escapeLabelValue(label.Value)+`"`)
	}
	return name + "{" + strings.Join(parts, ",") + "}"
}

func splitKey(k string) (string, string) {
	idx := strings.IndexByte(k, '{')
	if idx < 0 {
		return k, ""
	}
	return k[:idx], k[idx:]
}

func appendLabel(labels, name, value string) string {
	if labels == "" {
		return `{` + name + `="` + escapeLabelValue(value) + `"}`
	}
	return strings.TrimSuffix(labels, "}") + `,` + name + `="` + escapeLabelValue(value) + `"}`
}

func safeMetricName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "exmarketing_unknown"
	}
	return value
}

func safeLabelName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	return value
}

func safeLabelValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	if len(value) > 120 {
		return value[:120]
	}
	return value
}

func escapeLabelValue(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, "\n", `\n`)
	return strings.ReplaceAll(value, `"`, `\"`)
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}
