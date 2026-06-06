package output

import (
	"strings"
	"testing"
	"time"

	"probe-agent/internal/metrics"
)

func TestBatcherAdd(t *testing.T) {
	b := NewBatcher(3, time.Second)

	m := []metrics.Metric{
		{Name: "test_counter", Value: 1, Timestamp: time.Unix(1000, 0), Type: metrics.TypeCounter, Labels: map[string]string{"host": "x"}},
	}

	data := b.Add(m)
	if data != "" {
		t.Errorf("expected empty before batch full, got %q", data)
	}

	m2 := []metrics.Metric{
		{Name: "test_counter", Value: 2, Timestamp: time.Unix(1000, 0), Type: metrics.TypeCounter, Labels: map[string]string{"host": "x"}},
	}
	b.Add(m2)

	m3 := []metrics.Metric{
		{Name: "test_gauge", Value: 3, Timestamp: time.Unix(1000, 0), Type: metrics.TypeGauge},
	}
	data = b.Add(m3)

	if data == "" {
		t.Fatal("expected flushed data after batch full")
	}

	lines := strings.Split(strings.TrimSpace(data), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d: %q", len(lines), data)
	}
}

func TestBatcherFormat(t *testing.T) {
	b := NewBatcher(10, time.Second)

	m := []metrics.Metric{
		{
			Name:      "cpu_usage",
			Value:     42.5,
			Timestamp: time.Unix(1234567890, 0),
			Type:      metrics.TypeGauge,
			Labels:    map[string]string{"host": "server1", "region": "us-east"},
		},
	}

	b.Add(m)
	data := b.Flush()

	expected := `cpu_usage{host="server1",region="us-east"} 42.5 1234567890`
	if strings.TrimSpace(data) != expected {
		t.Errorf("unexpected format:\ngot:  %q\nwant: %q", strings.TrimSpace(data), expected)
	}
}

func TestBatcherEmptyFlush(t *testing.T) {
	b := NewBatcher(10, time.Second)
	data := b.Flush()
	if data != "" {
		t.Errorf("expected empty from empty flush, got %q", data)
	}
}

func TestBatcherSortLabels(t *testing.T) {
	b := NewBatcher(10, time.Second)
	m := []metrics.Metric{
		{
			Name:      "m",
			Value:     1,
			Timestamp: time.Unix(0, 0),
			Labels:    map[string]string{"z": "1", "a": "2", "n": "3"},
		},
	}
	b.Add(m)
	data := b.Flush()
	expected := `m{a="2",n="3",z="1"} 1 0`
	if strings.TrimSpace(data) != expected {
		t.Errorf("labels not sorted: got %q, want %q", strings.TrimSpace(data), expected)
	}
}
