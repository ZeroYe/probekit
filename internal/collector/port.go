package collector

import (
	"context"
	"net"
	"sync"
	"time"

	"github.com/ZeroYe/probekit/internal/config"
	"github.com/ZeroYe/probekit/internal/metrics"
	"github.com/ZeroYe/probekit/internal/output"
	"go.uber.org/zap"
)

type PortCollector struct {
	cfg     config.PortConfig
	runners []*portRunner
	logger  *zap.Logger
}

func NewPortCollector(cfg config.PortConfig, logger *zap.Logger) *PortCollector {
	return &PortCollector{
		cfg:    cfg,
		logger: logger.Named("port"),
	}
}

func (c *PortCollector) Name() string { return "port" }

func (c *PortCollector) Start(ctx context.Context, pipeline *output.Pipeline) error {
	if len(c.cfg.Targets) == 0 {
		c.logger.Info("no targets, skipping")
		return nil
	}

	for _, t := range c.cfg.Targets {
		runner := newPortRunner(t, c.logger)
		c.runners = append(c.runners, runner)
		go runner.run(ctx, pipeline)
	}

	c.logger.Info("started", zap.Int("targets", len(c.runners)))
	return nil
}

func (c *PortCollector) Stop() error {
	for _, r := range c.runners {
		r.stop()
	}
	return nil
}

type portRunner struct {
	target  config.PortTarget
	logger  *zap.Logger
	mu      sync.Mutex
	stopped bool
}

func newPortRunner(target config.PortTarget, logger *zap.Logger) *portRunner {
	return &portRunner{
		target: target,
		logger: logger.With(zap.String("target", target.Host), zap.Int("port", target.Port)),
	}
}

func (r *portRunner) stop() {
	r.mu.Lock()
	r.stopped = true
	r.mu.Unlock()
}

func (r *portRunner) isStopped() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.stopped
}

func (r *portRunner) run(ctx context.Context, pipeline *output.Pipeline) {
	ticker := time.NewTicker(r.target.Interval)
	defer ticker.Stop()

	r.probe(pipeline)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if r.isStopped() {
				return
			}
			r.probe(pipeline)
		}
	}
}

func (r *portRunner) probe(pipeline *output.Pipeline) {
	addr := net.JoinHostPort(r.target.Host, itoa(r.target.Port))
	start := time.Now()

	conn, err := net.DialTimeout(r.target.Protocol, addr, r.target.Timeout)
	elapsed := time.Since(start)

	up := 1.0
	if err != nil {
		up = 0.0
	} else {
		conn.Close()
	}

	labels := targetLabels(r.target.Host, r.target.Labels, map[string]string{
		"port":     itoa(r.target.Port),
		"protocol": r.target.Protocol,
	})

	now := time.Now().Unix()
	ms := []metrics.Metric{
		{Name: "port_up", Value: up, Labels: copyLabels(labels), Timestamp: time.Unix(now, 0), Type: metrics.TypeGauge},
		{Name: "port_dial_duration_seconds", Value: elapsed.Seconds(), Labels: copyLabels(labels), Timestamp: time.Unix(now, 0), Type: metrics.TypeGauge},
	}

	pipeline.Submit("port/"+r.target.Host+"|"+r.target.Protocol+"|"+itoa(r.target.Port), ms)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [12]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = '0' + byte(n%10)
		n /= 10
	}
	return string(buf[i:])
}
