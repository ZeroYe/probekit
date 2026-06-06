package output

import (
	"sync"

	"probe-agent/internal/config"
	"probe-agent/internal/metrics"
	"probe-agent/internal/selfmetrics"
	"go.uber.org/zap"
)

type Pipeline struct {
	pusher  *VMPusher
	registry *metrics.Registry
	input   chan []metrics.Metric
	wg      sync.WaitGroup
	stopCh  chan struct{}
}

func NewPipeline(pusher *VMPusher, registry *metrics.Registry, bufSize int) *Pipeline {
	if bufSize <= 0 {
		bufSize = 1000
	}
	return &Pipeline{
		pusher:   pusher,
		registry: registry,
		input:    make(chan []metrics.Metric, bufSize),
		stopCh:   make(chan struct{}),
	}
}

func (p *Pipeline) Start() error {
	if err := p.pusher.Start(); err != nil {
		return err
	}

	p.wg.Add(1)
	go p.processLoop()
	return nil
}

func (p *Pipeline) Stop() error {
	close(p.stopCh)
	p.wg.Wait()
	return p.pusher.Stop()
}

func (p *Pipeline) Submit(key string, ms []metrics.Metric) {
	if p.registry != nil {
		p.registry.Store(key, ms)
	}
	selfmetrics.MetricsCollected.Add(int64(len(ms)))
	select {
	case p.input <- ms:
	default:
	}
}

func (p *Pipeline) processLoop() {
	defer p.wg.Done()
	for {
		select {
		case <-p.stopCh:
			return
		case ms := <-p.input:
			p.pusher.Write(ms)
		}
	}
}

type PipelineConfig struct {
	VMConfig config.VMConfig
	Registry *metrics.Registry
	Logger   *zap.Logger
}

func NewPipelineFromConfig(cfg PipelineConfig) (*Pipeline, error) {
	reg := cfg.Registry
	if reg == nil {
		reg = metrics.NewRegistry()
	}
	pusher := NewVMPusher(cfg.VMConfig, cfg.Logger)
	return NewPipeline(pusher, reg, 1000), nil
}
