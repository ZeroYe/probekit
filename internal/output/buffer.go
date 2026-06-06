package output

import (
	"sync"

	"github.com/ZeroYe/probekit/internal/metrics"
)

type RingBuffer struct {
	mu    sync.Mutex
	data  []metrics.Metric
	head  int
	tail  int
	count int
	size  int
}

func NewRingBuffer(size int) *RingBuffer {
	if size <= 0 {
		size = 10000
	}
	return &RingBuffer{
		data: make([]metrics.Metric, size),
		size: size,
	}
}

func (b *RingBuffer) Push(m metrics.Metric) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.data[b.tail] = m
	b.tail = (b.tail + 1) % b.size
	if b.count == b.size {
		b.head = (b.head + 1) % b.size
	} else {
		b.count++
	}
}

func (b *RingBuffer) PopN(n int) []metrics.Metric {
	b.mu.Lock()
	defer b.mu.Unlock()

	if n <= 0 || b.count == 0 {
		return nil
	}
	if n > b.count {
		n = b.count
	}

	result := make([]metrics.Metric, n)
	for i := 0; i < n; i++ {
		result[i] = b.data[b.head]
		b.head = (b.head + 1) % b.size
	}
	b.count -= n

	return result
}

func (b *RingBuffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.count
}

func (b *RingBuffer) PushBatch(ms []metrics.Metric) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for _, m := range ms {
		b.data[b.tail] = m
		b.tail = (b.tail + 1) % b.size
		if b.count == b.size {
			b.head = (b.head + 1) % b.size
		} else {
			b.count++
		}
	}
}
