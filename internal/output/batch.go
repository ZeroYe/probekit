package output

import (
	"strconv"
	"strings"
	"sync"
	"time"

	"probe-agent/internal/metrics"
)

type Batcher struct {
	mu       sync.Mutex
	buf      strings.Builder
	count    int
	batchSize int
	interval time.Duration
}

func NewBatcher(batchSize int, interval time.Duration) *Batcher {
	return &Batcher{
		batchSize: batchSize,
		interval:  interval,
	}
}

func (b *Batcher) Add(ms []metrics.Metric) string {
	b.mu.Lock()
	defer b.mu.Unlock()

	for _, m := range ms {
		b.writeMetric(m)
	}

	if b.count >= b.batchSize {
		return b.flush()
	}
	return ""
}

func (b *Batcher) Flush() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.flush()
}

func (b *Batcher) flush() string {
	if b.count == 0 {
		return ""
	}
	data := b.buf.String()
	b.buf.Reset()
	b.count = 0
	return data
}

func (b *Batcher) writeMetric(m metrics.Metric) {
	b.buf.WriteString(m.Name)

	if len(m.Labels) > 0 {
		b.buf.WriteByte('{')
		first := true
		keys := make([]string, 0, len(m.Labels))
		for k := range m.Labels {
			keys = append(keys, k)
		}
		sortStrings(keys)
		for _, k := range keys {
			if !first {
				b.buf.WriteByte(',')
			}
			b.buf.WriteString(k)
			b.buf.WriteString(`="`)
			b.buf.WriteString(m.Labels[k])
			b.buf.WriteByte('"')
			first = false
		}
		b.buf.WriteByte('}')
	}

	b.buf.WriteByte(' ')
	b.buf.WriteString(float64Str(m.Value))
	b.buf.WriteByte(' ')
	b.buf.WriteString(strconv.FormatInt(m.Timestamp.Unix(), 10))
	b.buf.WriteByte('\n')
	b.count++
}

func float64Str(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func sortStrings(s []string) {
	for i := 0; i < len(s)-1; i++ {
		for j := i + 1; j < len(s); j++ {
			if s[i] > s[j] {
				s[i], s[j] = s[j], s[i]
			}
		}
	}
}
