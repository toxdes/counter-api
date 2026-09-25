package observability

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

type requestKey struct {
	method string
	route  string
	status int
}

type requestMetric struct {
	count        uint64
	durationSecs float64
}

// PoolMetrics is the database pool snapshot consumed by the metrics renderer.
type PoolMetrics struct {
	OpenConnections int
	InUse           int
	Idle            int
	WaitCount       int64
	WaitDuration    time.Duration
}

// Metrics is an in-process, bounded metrics registry. Route templates and a
// fixed event-name set prevent attacker-controlled cardinality growth.
type Metrics struct {
	mu       sync.RWMutex
	requests map[requestKey]requestMetric
	events   map[string]uint64
}

func NewMetrics() *Metrics {
	return &Metrics{
		requests: make(map[requestKey]requestMetric),
		events:   make(map[string]uint64),
	}
}

func (m *Metrics) RecordRequest(method, route string, status int, duration time.Duration) {
	if m == nil {
		return
	}
	key := requestKey{method: method, route: route, status: status}
	m.mu.Lock()
	metric := m.requests[key]
	metric.count++
	metric.durationSecs += duration.Seconds()
	m.requests[key] = metric
	m.mu.Unlock()
}

func (m *Metrics) RecordEvent(name string) {
	if m == nil || !safeEventName(name) {
		return
	}
	m.mu.Lock()
	m.events[name]++
	m.mu.Unlock()
}

func safeEventName(name string) bool {
	if name == "" {
		return false
	}
	for _, character := range name {
		if (character < 'a' || character > 'z') && character != '_' {
			return false
		}
	}
	return true
}

// RenderPrometheus emits stable text exposition without introducing a
// dependency for the small deployment profile. Labels are escaped before
// output and route values have already been normalized by middleware.
func (m *Metrics) RenderPrometheus(pool PoolMetrics, version string, schemaVersion int64) string {
	if m == nil {
		m = NewMetrics()
	}
	m.mu.RLock()
	requests := make(map[requestKey]requestMetric, len(m.requests))
	for key, metric := range m.requests {
		requests[key] = metric
	}
	events := make(map[string]uint64, len(m.events))
	for name, count := range m.events {
		events[name] = count
	}
	m.mu.RUnlock()

	var output strings.Builder
	output.WriteString("# TYPE counter_requests_total counter\n")
	keys := make([]requestKey, 0, len(requests))
	for key := range requests {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].route != keys[j].route {
			return keys[i].route < keys[j].route
		}
		if keys[i].method != keys[j].method {
			return keys[i].method < keys[j].method
		}
		return keys[i].status < keys[j].status
	})
	for _, key := range keys {
		metric := requests[key]
		labels := fmt.Sprintf(`method="%s",route="%s",status="%d"`, escapeLabel(key.method), escapeLabel(key.route), key.status)
		fmt.Fprintf(&output, "counter_requests_total{%s} %d\n", labels, metric.count)
		fmt.Fprintf(&output, "counter_request_duration_seconds_sum{%s} %g\n", labels, metric.durationSecs)
	}

	output.WriteString("# TYPE counter_events_total counter\n")
	knownEvents := []string{
		"idempotency_conflict",
		"idempotency_replay",
		"overload_rejected",
		"rate_limit_rejected",
		"rate_limit_backend_fallback",
		"reconciliation_mismatch",
		"service_error",
		"transaction_error",
	}
	eventNames := make([]string, 0, len(events)+len(knownEvents))
	seenEvents := make(map[string]struct{}, len(events)+len(knownEvents))
	for _, name := range knownEvents {
		eventNames = append(eventNames, name)
		seenEvents[name] = struct{}{}
	}
	for name := range events {
		if _, ok := seenEvents[name]; ok {
			continue
		}
		eventNames = append(eventNames, name)
	}
	sort.Strings(eventNames)
	for _, name := range eventNames {
		fmt.Fprintf(&output, "counter_events_total{name=\"%s\"} %d\n", escapeLabel(name), events[name])
	}

	output.WriteString("# TYPE counter_database_pool gauge\n")
	fmt.Fprintf(&output, "counter_database_pool{state=\"open\"} %d\n", pool.OpenConnections)
	fmt.Fprintf(&output, "counter_database_pool{state=\"in_use\"} %d\n", pool.InUse)
	fmt.Fprintf(&output, "counter_database_pool{state=\"idle\"} %d\n", pool.Idle)
	output.WriteString("# TYPE counter_database_pool_wait_total counter\n")
	fmt.Fprintf(&output, "counter_database_pool_wait_total %d\n", pool.WaitCount)
	output.WriteString("# TYPE counter_database_pool_wait_duration_seconds counter\n")
	fmt.Fprintf(&output, "counter_database_pool_wait_duration_seconds %g\n", pool.WaitDuration.Seconds())

	output.WriteString("# TYPE counter_build_info gauge\n")
	fmt.Fprintf(&output, "counter_build_info{version=\"%s\",schema_version=\"%d\"} 1\n", escapeLabel(version), schemaVersion)
	return output.String()
}

func escapeLabel(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return strings.ReplaceAll(value, "\n", `\n`)
}
