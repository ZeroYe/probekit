package selfmetrics

import (
	"testing"
)

func resetCounters() {
	MetricsCollected.Store(0)
	MetricsPushed.Store(0)
	PushErrors.Store(0)
	QueueLength.Store(0)
}

func TestBuildVersion(t *testing.T) {
	if BuildVersion == "" {
		t.Error("BuildVersion should not be empty")
	}
}

func TestCountersZeroInit(t *testing.T) {
	resetCounters()
	if v := MetricsCollected.Load(); v != 0 {
		t.Errorf("MetricsCollected: expected 0, got %d", v)
	}
	if v := MetricsPushed.Load(); v != 0 {
		t.Errorf("MetricsPushed: expected 0, got %d", v)
	}
	if v := PushErrors.Load(); v != 0 {
		t.Errorf("PushErrors: expected 0, got %d", v)
	}
	if v := QueueLength.Load(); v != 0 {
		t.Errorf("QueueLength: expected 0, got %d", v)
	}
}

func TestCounterIncrement(t *testing.T) {
	resetCounters()
	MetricsCollected.Add(10)
	MetricsPushed.Add(5)
	PushErrors.Add(1)
	QueueLength.Store(100)

	if v := MetricsCollected.Load(); v != 10 {
		t.Errorf("MetricsCollected: expected 10, got %d", v)
	}
	if v := MetricsPushed.Load(); v != 5 {
		t.Errorf("MetricsPushed: expected 5, got %d", v)
	}
	if v := PushErrors.Load(); v != 1 {
		t.Errorf("PushErrors: expected 1, got %d", v)
	}
	if v := QueueLength.Load(); v != 100 {
		t.Errorf("QueueLength: expected 100, got %d", v)
	}

	MetricsCollected.Add(-10)
	MetricsPushed.Add(-5)
	PushErrors.Add(-1)
	QueueLength.Store(0)
}
