package metrics

import (
	"sort"
	"strings"
	"sync"
	"time"
)

type MetricType string

const (
	TypeGauge     MetricType = "gauge"
	TypeCounter   MetricType = "counter"
	TypeHistogram MetricType = "histogram"
	TypeUntyped   MetricType = "untyped"
)

type Metric struct {
	Name      string
	Labels    map[string]string
	Value     float64
	Timestamp time.Time
	Type      MetricType
}

func (m *Metric) LabelString() string {
	if len(m.Labels) == 0 {
		return ""
	}
	keys := make([]string, 0, len(m.Labels))
	for k := range m.Labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(m.Labels[k])
	}
	return b.String()
}

type MetricBatch []Metric

type MetricPool struct {
	pool sync.Pool
}

func NewMetricPool() *MetricPool {
	return &MetricPool{
		pool: sync.Pool{
			New: func() any {
				return &Metric{}
			},
		},
	}
}

func (p *MetricPool) Get() *Metric {
	return p.pool.Get().(*Metric)
}

func (p *MetricPool) Put(m *Metric) {
	m.Name = ""
	m.Labels = nil
	m.Value = 0
	m.Type = ""
	m.Timestamp = time.Time{}
	p.pool.Put(m)
}
