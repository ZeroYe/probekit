package metrics

import (
	"testing"
	"time"
)

func TestMetricLabelString(t *testing.T) {
	t.Run("nil labels", func(t *testing.T) {
		m := &Metric{Name: "test", Value: 1}
		if s := m.LabelString(); s != "" {
			t.Errorf("expected empty, got %q", s)
		}
	})

	t.Run("single label", func(t *testing.T) {
		m := &Metric{Name: "test", Value: 1, Labels: map[string]string{"k": "v"}}
		if s := m.LabelString(); s != "k=v" {
			t.Errorf("expected k=v, got %q", s)
		}
	})

	t.Run("multiple labels sorted", func(t *testing.T) {
		m := &Metric{Name: "test", Value: 1, Labels: map[string]string{"b": "2", "a": "1"}}
		if s := m.LabelString(); s != "a=1,b=2" {
			t.Errorf("expected a=1,b=2, got %q", s)
		}
	})
}

func TestMetricPool(t *testing.T) {
	pool := NewMetricPool()

	m := pool.Get()
	m.Name = "test"
	m.Value = 42.0
	m.Labels = map[string]string{"k": "v"}
	m.Type = TypeGauge
	m.Timestamp = time.Now()

	pool.Put(m)

	recycled := pool.Get()
	if recycled.Name != "" {
		t.Errorf("expected empty name after Put")
	}
	if recycled.Labels != nil {
		t.Errorf("expected nil labels after Put")
	}
	if recycled.Value != 0 {
		t.Errorf("expected zero value after Put")
	}
}

func TestMetricTypeConstants(t *testing.T) {
	if TypeGauge != "gauge" {
		t.Errorf("TypeGauge should be gauge")
	}
	if TypeCounter != "counter" {
		t.Errorf("TypeCounter should be counter")
	}
	if TypeHistogram != "histogram" {
		t.Errorf("TypeHistogram should be histogram")
	}
	if TypeUntyped != "untyped" {
		t.Errorf("TypeUntyped should be untyped")
	}
}
