package observability

import (
	"strings"
	"testing"
	"time"
)

func TestRenderPrometheusIncludesBoundedRequestAndPoolMetrics(t *testing.T) {
	metrics := NewMetrics()
	metrics.RecordRequest("GET", "/v2/tenants/:id/counters/:id", 200, 25*time.Millisecond)
	metrics.RecordEvent("rate_limit_rejected")

	output := metrics.RenderPrometheus(PoolMetrics{
		OpenConnections: 4,
		InUse:           2,
		Idle:            2,
		WaitCount:       7,
		WaitDuration:    500 * time.Millisecond,
	}, "test", 7)

	for _, expected := range []string{
		`counter_requests_total{method="GET",route="/v2/tenants/:id/counters/:id",status="200"} 1`,
		`counter_events_total{name="rate_limit_rejected"} 1`,
		`counter_database_pool{state="in_use"} 2`,
		`counter_database_pool_wait_total 7`,
		`counter_build_info{version="test",schema_version="7"} 1`,
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("metrics output missing %q:\n%s", expected, output)
		}
	}
}

func TestRenderPrometheusIncludesGoRuntimeMetrics(t *testing.T) {
	output := NewMetrics().RenderPrometheus(PoolMetrics{}, "test", 7)
	for _, metric := range []string{
		"counter_go_runtime_goroutines",
		"counter_go_runtime_heap_alloc_bytes",
		"counter_go_runtime_heap_sys_bytes",
		"counter_go_runtime_gc_cycles_total",
		"counter_go_runtime_gc_pause_seconds_total",
	} {
		if !strings.Contains(output, metric) {
			t.Fatalf("runtime metric %q missing from output:\n%s", metric, output)
		}
	}
}
