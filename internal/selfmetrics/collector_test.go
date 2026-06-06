package selfmetrics

import (
	"testing"
)

func TestCollectWithoutRuntime(t *testing.T) {
	targets := map[string]int{"icmp": 2, "dns": 3, "snmp": 1}
	ms := Collect(false, targets)

	var infoCount, uptimeCount, targetsCount int
	for _, m := range ms {
		switch m.Name {
		case "probe_agent_info":
			infoCount++
			if v := m.Labels["version"]; v != "dev" {
				t.Errorf("expected version dev, got %s", v)
			}
		case "probe_agent_uptime_seconds":
			uptimeCount++
			if m.Value < 0 {
				t.Errorf("uptime should be >= 0, got %f", m.Value)
			}
		case "probe_agent_targets":
			targetsCount++
		case "probe_agent_goroutines":
			t.Errorf("should not collect runtime metrics when disabled")
		}
	}

	if infoCount != 1 {
		t.Errorf("expected 1 info metric, got %d", infoCount)
	}
	if uptimeCount != 1 {
		t.Errorf("expected 1 uptime metric, got %d", uptimeCount)
	}
	if targetsCount != 3 {
		t.Errorf("expected 3 target metrics (icmp,dns,snmp), got %d", targetsCount)
	}
}

func TestCollectWithRuntime(t *testing.T) {
	ms := Collect(true, nil)

	found := make(map[string]bool)
	for _, m := range ms {
		found[m.Name] = true
	}

	runtimeMetrics := []string{
		"probe_agent_goroutines",
		"probe_agent_memory_alloc_bytes",
		"probe_agent_memory_heap_bytes",
		"probe_agent_memory_sys_bytes",
		"probe_agent_gc_pauses_total",
	}

	for _, name := range runtimeMetrics {
		if !found[name] {
			t.Errorf("missing runtime metric: %s", name)
		}
	}
}

func TestCollectEmptyTargets(t *testing.T) {
	ms := Collect(false, nil)
	for _, m := range ms {
		if m.Name == "probe_agent_targets" {
			t.Error("should not emit targets metric with nil map")
		}
	}
}

func TestCollectCounterMetrics(t *testing.T) {
	resetCounters()
	MetricsCollected.Add(42)
	MetricsPushed.Add(30)
	PushErrors.Add(2)

	ms := Collect(false, nil)

	for _, m := range ms {
		switch m.Name {
		case "probe_agent_metrics_collected_total":
			if m.Value != 42 {
				t.Errorf("expected 42, got %f", m.Value)
			}
		case "probe_agent_metrics_pushed_total":
			if m.Value != 30 {
				t.Errorf("expected 30, got %f", m.Value)
			}
		case "probe_agent_push_errors_total":
			if m.Value != 2 {
				t.Errorf("expected 2, got %f", m.Value)
			}
		}
	}

	MetricsCollected.Add(-42)
	MetricsPushed.Add(-30)
	PushErrors.Add(-2)
}
