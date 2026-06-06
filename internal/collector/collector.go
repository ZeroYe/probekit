package collector

import (
	"context"

	"github.com/ZeroYe/probekit/internal/output"
)

type Collector interface {
	Name() string
	Start(ctx context.Context, pipeline *output.Pipeline) error
	Stop() error
}

type Manager struct {
	collectors []Collector
}

func NewManager() *Manager {
	return &Manager{}
}

func (m *Manager) Add(c Collector) {
	m.collectors = append(m.collectors, c)
}

func (m *Manager) Start(ctx context.Context, pipeline *output.Pipeline) error {
	for _, c := range m.collectors {
		if err := c.Start(ctx, pipeline); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) Stop() {
	for _, c := range m.collectors {
		c.Stop()
	}
}

func (m *Manager) Reset() {
	m.Stop()
	m.collectors = m.collectors[:0]
}
