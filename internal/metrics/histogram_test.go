package metrics

import (
	"testing"
)

func TestNewHistogram(t *testing.T) {
	h := NewHistogram([]float64{10, 5, 1})
	if len(h.Buckets) != 3 {
		t.Fatalf("expected 3 buckets, got %d", len(h.Buckets))
	}
	if h.Buckets[0] != 1 || h.Buckets[1] != 5 || h.Buckets[2] != 10 {
		t.Errorf("buckets not sorted: %v", h.Buckets)
	}
	if len(h.Counts) != 3 {
		t.Errorf("expected 3 counts, got %d", len(h.Counts))
	}
}

func TestHistogramObserve(t *testing.T) {
	h := NewHistogram([]float64{5, 10, 20})

	h.Observe(3)
	h.Observe(7)
	h.Observe(15)
	h.Observe(25)

	if h.Count != 4 {
		t.Errorf("expected count 4, got %d", h.Count)
	}
	if h.Sum != 50 {
		t.Errorf("expected sum 50, got %f", h.Sum)
	}

	if h.Counts[0] != 1 {
		t.Errorf("bucket <=5: expected 1, got %d", h.Counts[0])
	}
	if h.Counts[1] != 2 {
		t.Errorf("bucket <=10: expected 2, got %d", h.Counts[1])
	}
	if h.Counts[2] != 3 {
		t.Errorf("bucket <=20: expected 3, got %d", h.Counts[2])
	}
}

func TestHistogramReset(t *testing.T) {
	h := NewHistogram([]float64{5, 10})
	h.Observe(3)
	h.Observe(7)
	h.Reset()

	if h.Count != 0 {
		t.Errorf("expected count 0 after reset, got %d", h.Count)
	}
	if h.Sum != 0 {
		t.Errorf("expected sum 0 after reset, got %f", h.Sum)
	}
	for i, c := range h.Counts {
		if c != 0 {
			t.Errorf("counts[%d]: expected 0 after reset, got %d", i, c)
		}
	}
}

func TestHistogramMetrics(t *testing.T) {
	h := NewHistogram([]float64{5, 10})
	h.Observe(3)
	h.Observe(15)

	ms := h.Metrics("test_metric", map[string]string{"host": "x"}, 1000)

	count := 0
	sum := 0.0
	buckets := 0

	for _, m := range ms {
		switch m.Name {
		case "test_metric_count":
			count++
			if m.Value != 2 {
				t.Errorf("count: expected 2, got %f", m.Value)
			}
		case "test_metric_sum":
			sum = m.Value
			if m.Value != 18 {
				t.Errorf("sum: expected 18, got %f", m.Value)
			}
		case "test_metric_bucket":
			buckets++
		}
	}

	if count != 1 {
		t.Errorf("expected 1 count metric, got %d", count)
	}
	if sum != 18 {
		t.Errorf("expected sum 18, got %f", sum)
	}
	if buckets != 3 {
		t.Errorf("expected 3 bucket metrics (2 buckets + +Inf), got %d", buckets)
	}
}

func TestFloatStr(t *testing.T) {
	tests := []struct {
		input float64
		want  string
	}{
		{1.0, "1"},
		{1.5, "1.5"},
		{0.001, "0.001"},
		{1000, "1000"},
	}

	for _, tt := range tests {
		if got := floatStr(tt.input); got != tt.want {
			t.Errorf("floatStr(%f) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
