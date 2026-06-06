package collector

import (
	"context"
	"math"
	"net"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/ZeroYe/probekit/internal/config"
	"github.com/ZeroYe/probekit/internal/metrics"
	"github.com/ZeroYe/probekit/internal/output"
	"go.uber.org/zap"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

type ICMPCollector struct {
	cfg      config.ICMPConfig
	runners  []*icmpRunner
	logger   *zap.Logger
	sem      chan struct{}
	pipeline *output.Pipeline
}

func NewICMPCollector(cfg config.ICMPConfig, logger *zap.Logger, concurrency int, pipeline *output.Pipeline) *ICMPCollector {
	if concurrency <= 0 {
		concurrency = 20
	}
	return &ICMPCollector{
		cfg:      cfg,
		logger:   logger.Named("icmp"),
		sem:      make(chan struct{}, concurrency),
		pipeline: pipeline,
	}
}

func (c *ICMPCollector) Name() string { return "icmp" }

func (c *ICMPCollector) Start(ctx context.Context) error {
	if len(c.cfg.Targets) == 0 {
		c.logger.Info("no targets, skipping")
		return nil
	}

	for _, t := range c.cfg.Targets {
		runner := newICMPRunner(t, c.cfg.HistogramBucketsMs, c.sem, c.logger)
		c.runners = append(c.runners, runner)
		go runner.run(ctx, c.pipeline)
	}

	c.logger.Info("started", zap.Int("targets", len(c.runners)), zap.Int("concurrency", cap(c.sem)))
	return nil
}

func (c *ICMPCollector) Stop() error {
	for _, r := range c.runners {
		r.stop()
	}
	return nil
}

type icmpRunner struct {
	target      config.ICMPTarget
	buckets     []float64
	histogram   *metrics.Histogram
	logger      *zap.Logger
	protocol    int
	consecLoss  int
	stopped     bool
	mu          sync.Mutex
	sem         chan struct{}
}

func newICMPRunner(target config.ICMPTarget, bucketsMs []int, sem chan struct{}, logger *zap.Logger) *icmpRunner {
	buckets := make([]float64, len(bucketsMs))
	for i, v := range bucketsMs {
		buckets[i] = float64(v)
	}

	return &icmpRunner{
		target:    target,
		buckets:   buckets,
		histogram: metrics.NewHistogram(buckets),
		protocol:  1,
		sem:       sem,
		logger:    logger.With(zap.String("target", target.Host)),
	}
}

func (r *icmpRunner) stop() {
	r.mu.Lock()
	r.stopped = true
	r.mu.Unlock()
}

func (r *icmpRunner) isStopped() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.stopped
}

func (r *icmpRunner) run(ctx context.Context, pipeline *output.Pipeline) {
	conn, err := icmp.ListenPacket("ip4:icmp", "0.0.0.0")
	if err != nil {
		r.logger.Error("listen icmp", zap.Error(err))
		return
	}
	defer conn.Close()

	ticker := time.NewTicker(r.target.Interval)
	defer ticker.Stop()

	r.sem <- struct{}{}
	r.probe(conn, pipeline)
	<-r.sem

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if r.isStopped() {
				return
			}
			r.sem <- struct{}{}
			r.probe(conn, pipeline)
			<-r.sem
		}
	}
}

func (r *icmpRunner) probe(conn *icmp.PacketConn, pipeline *output.Pipeline) {
	addr := &net.IPAddr{IP: net.ParseIP(r.target.Host)}
	if addr.IP == nil {
		r.logger.Warn("invalid target address")
		return
	}

	up := 1.0

	id := os.Getpid() & 0xffff
	timeout := r.target.Timeout
	deadline := time.Now().Add(timeout)
	conn.SetDeadline(deadline)

	sendTimes := make([]time.Time, r.target.Count)
	lost := 0

	for seq := 0; seq < r.target.Count; seq++ {
		msg := icmp.Message{
			Type: ipv4.ICMPTypeEcho,
			Code: 0,
			Body: &icmp.Echo{
				ID:  id,
				Seq: seq,
			},
		}
		wb, err := msg.Marshal(nil)
		if err != nil {
			r.logger.Warn("marshal icmp", zap.Error(err))
			lost++
			continue
		}
		_, err = conn.WriteTo(wb, addr)
		if err != nil {
			r.logger.Warn("write icmp", zap.Error(err))
			lost++
			continue
		}
		sendTimes[seq] = time.Now()
	}

	var rtts []float64

	for i := 0; i < r.target.Count; i++ {
		rb := make([]byte, 1500)
		n, _, err := conn.ReadFrom(rb)
		if err != nil {
			lost += r.target.Count - i
			break
		}

		rm, err := icmp.ParseMessage(r.protocol, rb[:n])
		if err != nil {
			continue
		}

		if rm.Type != ipv4.ICMPTypeEchoReply {
			continue
		}

		echo, ok := rm.Body.(*icmp.Echo)
		if !ok || echo.ID != id || echo.Seq < 0 || echo.Seq >= r.target.Count {
			continue
		}

		if sendTimes[echo.Seq].IsZero() {
			continue
		}

		rtt := time.Since(sendTimes[echo.Seq]).Seconds() * 1000
		rtts = append(rtts, rtt)
	}

	if len(rtts) == 0 {
		r.consecLoss++
		up = 0.0
	} else {
		r.consecLoss = 0
	}

	labels := targetLabels(r.target.Host, r.target.Labels, nil)
	now := time.Now().Unix()

	ms := r.buildMetrics(rtts, lost, up, labels, now)
	pipeline.Submit("icmp/"+r.target.Host, ms)
}

func (r *icmpRunner) buildMetrics(rtts []float64, lost int, up float64, labels map[string]string, ts int64) []metrics.Metric {
	var ms []metrics.Metric

	ms = append(ms, metrics.Metric{
		Name: "icmp_up", Value: up, Labels: copyLabels(labels), Timestamp: time.Unix(ts, 0), Type: metrics.TypeGauge,
	})

	ms = append(ms, metrics.Metric{
		Name: "icmp_packet_loss_ratio", Value: calcLossRatio(lost, r.target.Count), Labels: copyLabels(labels), Timestamp: time.Unix(ts, 0), Type: metrics.TypeGauge,
	})

	ms = append(ms, metrics.Metric{
		Name: "icmp_consecutive_loss", Value: float64(r.consecLoss), Labels: copyLabels(labels), Timestamp: time.Unix(ts, 0), Type: metrics.TypeGauge,
	})

	if len(rtts) == 0 {
		return ms
	}

	sort.Float64s(rtts)

	min := rtts[0]
	max := rtts[len(rtts)-1]
	var sum, sumSq float64
	for _, v := range rtts {
		sum += v
	}
	avg := sum / float64(len(rtts))

	for _, v := range rtts {
		d := v - avg
		sumSq += d * d
	}
	stddev := math.Sqrt(sumSq / float64(len(rtts)))

	var jitter float64
	if len(rtts) > 1 {
		var jSum float64
		for i := 1; i < len(rtts); i++ {
			jSum += math.Abs(rtts[i] - rtts[i-1])
		}
		jitter = jSum / float64(len(rtts)-1)
	}

	ms = append(ms,
		metrics.Metric{Name: "icmp_rtt_min", Value: min, Labels: copyLabels(labels), Timestamp: time.Unix(ts, 0), Type: metrics.TypeGauge},
		metrics.Metric{Name: "icmp_rtt_max", Value: max, Labels: copyLabels(labels), Timestamp: time.Unix(ts, 0), Type: metrics.TypeGauge},
		metrics.Metric{Name: "icmp_rtt_avg", Value: avg, Labels: copyLabels(labels), Timestamp: time.Unix(ts, 0), Type: metrics.TypeGauge},
		metrics.Metric{Name: "icmp_rtt_stddev", Value: stddev, Labels: copyLabels(labels), Timestamp: time.Unix(ts, 0), Type: metrics.TypeGauge},
		metrics.Metric{Name: "icmp_jitter", Value: jitter, Labels: copyLabels(labels), Timestamp: time.Unix(ts, 0), Type: metrics.TypeGauge},
	)

	r.histogram.Reset()
	for _, v := range rtts {
		r.histogram.Observe(v)
	}

	histMs := r.histogram.Metrics("icmp_rtt", labels, ts)
	ms = append(ms, histMs...)

	return ms
}

func calcLossRatio(lost, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(lost) / float64(total)
}

func targetLabels(host string, extra map[string]string, add map[string]string) map[string]string {
	l := map[string]string{"target": host}
	for k, v := range extra {
		l[k] = v
	}
	for k, v := range add {
		l[k] = v
	}
	return l
}

func copyLabels(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	c := make(map[string]string, len(m))
	for k, v := range m {
		c[k] = v
	}
	return c
}
