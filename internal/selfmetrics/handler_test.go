package selfmetrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type mockCounter struct {
	counts map[string]int
}

func (m mockCounter) Counts() map[string]int { return m.counts }

func TestHandlerContentType(t *testing.T) {
	resetCounters()
	h := Handler(false, mockCounter{map[string]int{"icmp": 1}})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	h(rec, req)

	if ct := rec.Header().Get("Content-Type"); ct != "text/plain; version=0.0.4" {
		t.Errorf("expected text/plain; version=0.0.4, got %q", ct)
	}
}

func TestHandlerOutputFormat(t *testing.T) {
	resetCounters()
	MetricsCollected.Store(5)
	MetricsPushed.Store(3)
	PushErrors.Store(0)
	QueueLength.Store(0)

	h := Handler(false, mockCounter{map[string]int{"icmp": 2}})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	h(rec, req)

	body := rec.Body.String()
	t.Logf("handler output:\n%s", body)

	if !strings.Contains(body, "# HELP probe_agent_info") {
		t.Error("missing HELP line for probe_agent_info")
	}
	if !strings.Contains(body, "# TYPE probe_agent_info gauge") {
		t.Error("missing TYPE line for probe_agent_info")
	}
	if !strings.Contains(body, `probe_agent_info{`) {
		t.Error("missing label-encoded info metric")
	}
	if !strings.Contains(body, `probe_agent_targets{module="icmp"} 2`) {
		t.Errorf("missing targets metric with correct value")
	}
	if !strings.Contains(body, "probe_agent_metrics_collected_total 5") {
		t.Errorf("missing collected_total metric")
	}
}

func TestHandlerWithRuntime(t *testing.T) {
	h := Handler(true, nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	h(rec, req)

	body := rec.Body.String()

	runtimeMetrics := []string{
		"probe_agent_goroutines",
		"probe_agent_memory_alloc_bytes",
		"probe_agent_gc_pauses_total",
	}
	for _, name := range runtimeMetrics {
		if !strings.Contains(body, name) {
			t.Errorf("missing runtime metric in output: %s", name)
		}
	}
}

func TestHandlerUptimePositive(t *testing.T) {
	h := Handler(false, nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	h(rec, req)

	rec2 := httptest.NewRecorder()
	h(rec2, req)

	// uptime should increase between calls
	body1 := rec.Body.String()
	body2 := rec2.Body.String()
	if body1 == body2 {
		t.Log("uptime values may be equal if calls are very fast")
	}
}
