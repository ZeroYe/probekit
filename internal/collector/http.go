package collector

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ZeroYe/probekit/internal/config"
	"github.com/ZeroYe/probekit/internal/metrics"
	"github.com/ZeroYe/probekit/internal/output"
	"go.uber.org/zap"
)

type HTTPCollector struct {
	cfg     config.HTTPConfig
	runners []*httpRunner
	logger  *zap.Logger
}

func NewHTTPCollector(cfg config.HTTPConfig, logger *zap.Logger) *HTTPCollector {
	return &HTTPCollector{
		cfg:    cfg,
		logger: logger.Named("http"),
	}
}

func (c *HTTPCollector) Name() string { return "http" }

func (c *HTTPCollector) Start(ctx context.Context, pipeline *output.Pipeline) error {
	if len(c.cfg.Targets) == 0 {
		c.logger.Info("no targets, skipping")
		return nil
	}

	for _, t := range c.cfg.Targets {
		runner := newHTTPRunner(t, c.logger)
		c.runners = append(c.runners, runner)
		go runner.run(ctx, pipeline)
	}

	c.logger.Info("started", zap.Int("targets", len(c.runners)))
	return nil
}

func (c *HTTPCollector) Stop() error {
	for _, r := range c.runners {
		r.stop()
	}
	return nil
}

type httpRunner struct {
	target  config.HTTPTarget
	client  *http.Client
	logger  *zap.Logger
	mu      sync.Mutex
	stopped bool
}

func newHTTPRunner(target config.HTTPTarget, logger *zap.Logger) *httpRunner {
	client := &http.Client{
		Timeout: target.Timeout,
		Transport: &http.Transport{
			MaxIdleConns:    1,
			IdleConnTimeout: 90 * time.Second,
		},
	}

	return &httpRunner{
		target: target,
		client: client,
		logger: logger.With(zap.String("url", target.URL)),
	}
}

func (r *httpRunner) stop() {
	r.mu.Lock()
	r.stopped = true
	r.mu.Unlock()
}

func (r *httpRunner) isStopped() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.stopped
}

func (r *httpRunner) run(ctx context.Context, pipeline *output.Pipeline) {
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

func (r *httpRunner) probe(pipeline *output.Pipeline) {
	start := time.Now()

	req, err := http.NewRequest(r.target.Method, r.target.URL, nil)
	if err != nil {
		r.logger.Warn("create request", zap.Error(err))
		return
	}

	for k, v := range r.target.Headers {
		req.Header.Set(k, v)
	}

	resp, err := r.client.Do(req)
	elapsed := time.Since(start)

	labels := targetLabels(r.target.URL, r.target.Labels, map[string]string{
		"method": r.target.Method,
	})

	now := time.Now().Unix()

	if err != nil {
		ms := []metrics.Metric{
			{Name: "http_up", Value: 0, Labels: copyLabels(labels), Timestamp: time.Unix(now, 0), Type: metrics.TypeGauge},
			{Name: "http_duration_seconds", Value: elapsed.Seconds(), Labels: copyLabels(labels), Timestamp: time.Unix(now, 0), Type: metrics.TypeGauge},
		}
		pipeline.Submit("http/"+r.target.URL, ms)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	up := 1.0
	if !isExpectedStatus(resp.StatusCode, r.target.ExpectedStatusCodes) {
		up = 0.0
	}
	if up == 1.0 && r.target.ExpectedBodyContains != "" && !strings.Contains(string(body), r.target.ExpectedBodyContains) {
		up = 0.0
	}

	r.logger.Debug("probe result",
		zap.Int("status_code", resp.StatusCode),
		zap.Float64("up", up),
		zap.Duration("duration", elapsed),
	)

	ms := []metrics.Metric{
		{Name: "http_up", Value: up, Labels: copyLabels(labels), Timestamp: time.Unix(now, 0), Type: metrics.TypeGauge},
		{Name: "http_status_code", Value: float64(resp.StatusCode), Labels: copyLabels(labels), Timestamp: time.Unix(now, 0), Type: metrics.TypeGauge},
		{Name: "http_duration_seconds", Value: elapsed.Seconds(), Labels: copyLabels(labels), Timestamp: time.Unix(now, 0), Type: metrics.TypeGauge},
		{Name: "http_response_size_bytes", Value: float64(len(body)), Labels: copyLabels(labels), Timestamp: time.Unix(now, 0), Type: metrics.TypeGauge},
	}

	pipeline.Submit("http/"+r.target.URL, ms)
}

func isExpectedStatus(code int, expected []int) bool {
	for _, c := range expected {
		if code == c {
			return true
		}
	}
	return false
}
