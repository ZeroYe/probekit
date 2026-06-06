package collector

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/ZeroYe/probekit/internal/config"
	"github.com/ZeroYe/probekit/internal/metrics"
	"github.com/ZeroYe/probekit/internal/output"
	"go.uber.org/zap"
)

type DNSCollector struct {
	cfg     config.DNSConfig
	runners []*dnsRunner
	logger  *zap.Logger
}

func NewDNSCollector(cfg config.DNSConfig, logger *zap.Logger) *DNSCollector {
	return &DNSCollector{
		cfg:    cfg,
		logger: logger.Named("dns"),
	}
}

func (c *DNSCollector) Name() string { return "dns" }

func (c *DNSCollector) Start(ctx context.Context, pipeline *output.Pipeline) error {
	if len(c.cfg.Targets) == 0 {
		c.logger.Info("no targets, skipping")
		return nil
	}

	for _, t := range c.cfg.Targets {
		runner := newDNSRunner(t, c.logger)
		c.runners = append(c.runners, runner)
		go runner.run(ctx, pipeline)
	}

	c.logger.Info("started", zap.Int("targets", len(c.runners)))
	return nil
}

func (c *DNSCollector) Stop() error {
	for _, r := range c.runners {
		r.stop()
	}
	return nil
}

type dnsRunner struct {
	target   config.DNSTarget
	resolver *net.Resolver
	logger   *zap.Logger
	mu       sync.Mutex
	stopped  bool
}

func newDNSRunner(target config.DNSTarget, logger *zap.Logger) *dnsRunner {
	addr := net.JoinHostPort(target.Server, "53")

	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			d := net.Dialer{Timeout: 5 * time.Second}
			return d.DialContext(ctx, network, addr)
		},
	}

	return &dnsRunner{
		target:   target,
		resolver: resolver,
		logger:   logger.With(zap.String("domain", target.Domain), zap.String("server", target.Server)),
	}
}

func (r *dnsRunner) stop() {
	r.mu.Lock()
	r.stopped = true
	r.mu.Unlock()
}

func (r *dnsRunner) isStopped() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.stopped
}

func (r *dnsRunner) run(ctx context.Context, pipeline *output.Pipeline) {
	ticker := time.NewTicker(r.target.Interval)
	defer ticker.Stop()

	r.probe(ctx, pipeline)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if r.isStopped() {
				return
			}
			r.probe(ctx, pipeline)
		}
	}
}

func (r *dnsRunner) probe(ctx context.Context, pipeline *output.Pipeline) {
	start := time.Now()
	count, code, err := r.lookup(ctx)
	elapsed := time.Since(start)

	labels := targetLabels(r.target.Domain, r.target.Labels, map[string]string{
		"server":      r.target.Server,
		"record_type": r.target.RecordType,
	})

	up := 1.0
	if err != nil {
		up = 0.0
	}

	now := time.Now().Unix()
	var ms []metrics.Metric

	ms = append(ms, metrics.Metric{
		Name: "dns_up", Value: up, Labels: copyLabels(labels),
		Timestamp: time.Unix(now, 0), Type: metrics.TypeGauge,
	})

	ms = append(ms, metrics.Metric{
		Name: "dns_lookup_duration_seconds", Value: elapsed.Seconds(), Labels: copyLabels(labels),
		Timestamp: time.Unix(now, 0), Type: metrics.TypeGauge,
	})

	ms = append(ms, metrics.Metric{
		Name: "dns_answer_count", Value: float64(count), Labels: copyLabels(labels),
		Timestamp: time.Unix(now, 0), Type: metrics.TypeGauge,
	})

	codeLabels := make(map[string]string, len(labels)+1)
	for k, v := range labels {
		codeLabels[k] = v
	}
	codeLabels["code"] = code

	ms = append(ms, metrics.Metric{
		Name: "dns_response", Value: 1, Labels: codeLabels,
		Timestamp: time.Unix(now, 0), Type: metrics.TypeUntyped,
	})

	pipeline.Submit("dns/"+r.target.Domain+"|"+r.target.Server, ms)
}

func (r *dnsRunner) lookup(ctx context.Context) (int, string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	switch strings.ToUpper(r.target.RecordType) {
	case "A":
		ips, err := r.resolver.LookupNetIP(ctx, "ip4", r.target.Domain)
		return len(ips), dnsCode(err), err
	case "AAAA":
		ips, err := r.resolver.LookupNetIP(ctx, "ip6", r.target.Domain)
		return len(ips), dnsCode(err), err
	case "MX":
		mx, err := r.resolver.LookupMX(ctx, r.target.Domain)
		return len(mx), dnsCode(err), err
	case "NS":
		ns, err := r.resolver.LookupNS(ctx, r.target.Domain)
		return len(ns), dnsCode(err), err
	case "CNAME":
		cname, err := r.resolver.LookupCNAME(ctx, r.target.Domain)
		if err != nil {
			return 0, dnsCode(err), err
		}
		if cname != "" {
			return 1, "NOERROR", nil
		}
		return 0, "NOERROR", nil
	case "TXT":
		txts, err := r.resolver.LookupTXT(ctx, r.target.Domain)
		return len(txts), dnsCode(err), err
	default:
		ips, err := r.resolver.LookupNetIP(ctx, "ip", r.target.Domain)
		return len(ips), dnsCode(err), err
	}
}

func dnsCode(err error) string {
	if err == nil {
		return "NOERROR"
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		if dnsErr.IsTimeout {
			return "TIMEOUT"
		}
		if dnsErr.IsNotFound {
			return "NXDOMAIN"
		}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "TIMEOUT"
	}
	return "SERVFAIL"
}
