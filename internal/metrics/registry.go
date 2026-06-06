package metrics

import (
	"sort"
	"sync"
)

type Registry struct {
	mu      sync.RWMutex
	metrics map[string][]Metric
}

func NewRegistry() *Registry {
	return &Registry{
		metrics: make(map[string][]Metric),
	}
}

func (r *Registry) Store(key string, ms []Metric) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.metrics[key] = ms
}

func (r *Registry) Get(key string) []Metric {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.metrics[key]
}

func (r *Registry) Keys() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	keys := make([]string, 0, len(r.metrics))
	for k := range r.metrics {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func (r *Registry) Delete(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.metrics, key)
}

func (r *Registry) All() map[string][]Metric {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c := make(map[string][]Metric, len(r.metrics))
	for k, v := range r.metrics {
		ms := make([]Metric, len(v))
		copy(ms, v)
		c[k] = ms
	}
	return c
}
