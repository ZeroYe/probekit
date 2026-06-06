package metrics

import (
	"sort"
	"strconv"
	"time"
)

type Histogram struct {
	Buckets []float64
	Counts  []uint64
	Count   uint64
	Sum     float64
}

func NewHistogram(buckets []float64) *Histogram {
	sorted := make([]float64, len(buckets))
	copy(sorted, buckets)
	sort.Float64s(sorted)

	return &Histogram{
		Buckets: sorted,
		Counts:  make([]uint64, len(sorted)),
	}
}

func (h *Histogram) Observe(value float64) {
	h.Count++
	h.Sum += value

	for i, b := range h.Buckets {
		if value <= b {
			h.Counts[i]++
		}
	}
}

func (h *Histogram) Reset() {
	h.Count = 0
	h.Sum = 0
	for i := range h.Counts {
		h.Counts[i] = 0
	}
}

func (h *Histogram) Metrics(name string, labels map[string]string, ts int64) []Metric {
	merge := make(map[string]string, len(labels)+1)
	for k, v := range labels {
		merge[k] = v
	}

	now := time.Unix(ts, 0)
	var ms []Metric

	ms = append(ms, Metric{
		Name:      name + "_count",
		Labels:    copyMapLabels(labels),
		Value:     float64(h.Count),
		Timestamp: now,
		Type:      TypeGauge,
	})

	ms = append(ms, Metric{
		Name:      name + "_sum",
		Labels:    copyMapLabels(labels),
		Value:     h.Sum,
		Timestamp: now,
		Type:      TypeGauge,
	})

	for i, bucket := range h.Buckets {
		merge["le"] = floatStr(bucket)
		ms = append(ms, Metric{
			Name:      name + "_bucket",
			Labels:    copyMapLabels(merge),
			Value:     float64(h.Counts[i]),
			Timestamp: now,
			Type:      TypeGauge,
		})
	}

	merge["le"] = "+Inf"
	ms = append(ms, Metric{
		Name:      name + "_bucket",
		Labels:    copyMapLabels(merge),
		Value:     float64(h.Count),
		Timestamp: now,
		Type:      TypeGauge,
	})

	return ms
}

func floatStr(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func copyMapLabels(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	c := make(map[string]string, len(m))
	for k, v := range m {
		c[k] = v
	}
	return c
}
