package collector

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ZeroYe/probekit/internal/config"
	"github.com/ZeroYe/probekit/internal/metrics"
	"github.com/ZeroYe/probekit/internal/output"
	"github.com/gosnmp/gosnmp"
	"go.uber.org/zap"
)

type SNMPCollector struct {
	cfg     config.SNMPConfig
	runners []*snmpRunner
	logger  *zap.Logger
}

func NewSNMPCollector(cfg config.SNMPConfig, logger *zap.Logger) *SNMPCollector {
	return &SNMPCollector{
		cfg:    cfg,
		logger: logger.Named("snmp"),
	}
}

func (c *SNMPCollector) Name() string { return "snmp" }

func (c *SNMPCollector) Start(ctx context.Context, pipeline *output.Pipeline) error {
	if len(c.cfg.Targets) == 0 {
		c.logger.Info("no targets, skipping")
		return nil
	}

	for _, t := range c.cfg.Targets {
		runner := newSNMPRunner(t, c.cfg.Defaults, c.logger)
		c.runners = append(c.runners, runner)
		go runner.run(ctx, pipeline)
	}

	c.logger.Info("started", zap.Int("targets", len(c.runners)))
	return nil
}

func (c *SNMPCollector) Stop() error {
	for _, r := range c.runners {
		r.stop()
	}
	return nil
}

type snmpRunner struct {
	target   config.SNMPTarget
	defaults config.SNMPDefaults
	logger   *zap.Logger
	mu       sync.Mutex
	stopped  bool
}

func newSNMPRunner(target config.SNMPTarget, defaults config.SNMPDefaults, logger *zap.Logger) *snmpRunner {
	return &snmpRunner{
		target:   target,
		defaults: defaults,
		logger:   logger.With(zap.String("target", target.Host)),
	}
}

func (r *snmpRunner) stop() {
	r.mu.Lock()
	r.stopped = true
	r.mu.Unlock()
}

func (r *snmpRunner) isStopped() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.stopped
}

func (r *snmpRunner) run(ctx context.Context, pipeline *output.Pipeline) {
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

func (r *snmpRunner) probe(pipeline *output.Pipeline) {
	timeout := r.defaults.Timeout
	if r.target.Timeout > 0 {
		timeout = r.target.Timeout
	}

	g := &gosnmp.GoSNMP{
		Target:    r.target.Host,
		Port:      r.defaults.Port,
		Community: r.defaults.Community,
		Version:   snmpVersion(r.defaults.Version),
		Timeout:   timeout,
		Retries:   r.defaults.Retries,
	}

	err := g.Connect()
	if err != nil {
		r.logger.Warn("connect failed", zap.Error(err))
		r.submitError(pipeline, err)
		return
	}
	defer g.Conn.Close()

	labels := targetLabels(r.target.Host, r.target.Labels, nil)
	now := time.Now().Unix()
	var ms []metrics.Metric

	ms = append(ms, metrics.Metric{
		Name: "snmp_up", Value: 1, Labels: copyLabels(labels),
		Timestamp: time.Unix(now, 0), Type: metrics.TypeGauge,
	})

	for _, oid := range r.target.OIDs.Scalar {
		scalarMS := r.getScalar(g, oid, labels, now)
		ms = append(ms, scalarMS...)
	}

	for _, table := range r.target.OIDs.Tables {
		tableMS := r.walkTable(g, table, labels, now)
		ms = append(ms, tableMS...)
	}

	pipeline.Submit("snmp/"+r.target.Host, ms)
}

func (r *snmpRunner) submitError(pipeline *output.Pipeline, err error) {
	labels := targetLabels(r.target.Host, r.target.Labels, nil)
	now := time.Now().Unix()
	ms := []metrics.Metric{
		{
			Name: "snmp_up", Value: 0, Labels: copyLabels(labels),
			Timestamp: time.Unix(now, 0), Type: metrics.TypeGauge,
		},
	}
	pipeline.Submit("snmp/"+r.target.Host, ms)
}

func (r *snmpRunner) getScalar(g *gosnmp.GoSNMP, oid string, baseLabels map[string]string, ts int64) []metrics.Metric {
	result, err := g.Get([]string{oid})
	if err != nil {
		r.logger.Warn("get failed", zap.String("oid", oid), zap.Error(err))
		return nil
	}

	if len(result.Variables) == 0 {
		return nil
	}

	v := result.Variables[0]
	val := snmpValue(v)

	labels := make(map[string]string, len(baseLabels)+1)
	for k, v := range baseLabels {
		labels[k] = v
	}
	labels["oid"] = oid

	name := snmpMetricName(oid)

	return []metrics.Metric{
		{
			Name: name, Value: val, Labels: labels,
			Timestamp: time.Unix(ts, 0), Type: metricTypeForValue(val),
		},
	}
}

func (r *snmpRunner) walkTable(g *gosnmp.GoSNMP, table config.SNMPTable, baseLabels map[string]string, ts int64) []metrics.Metric {
	tagMap, err := r.walkTag(g, table.Tag, table.OID)
	if err != nil {
		r.logger.Warn("walk tag failed", zap.String("table", table.OID), zap.Error(err))
	}

	var ms []metrics.Metric

	for _, m := range table.Metrics {
		metricOID := m.OID
		metricMS := r.walkMetric(g, metricOID, m.Name, table.Index, tagMap, baseLabels, ts)
		ms = append(ms, metricMS...)
	}

	return ms
}

func (r *snmpRunner) walkTag(g *gosnmp.GoSNMP, tagOID string, tableOID string) (map[string]string, error) {
	if tagOID == "" {
		return nil, nil
	}

	tagMap := make(map[string]string)

	err := g.Walk(tagOID, func(pdu gosnmp.SnmpPDU) error {
		idx := extractIndex(pdu.Name, tableOID, tagOID)
		if idx == "" {
			return nil
		}
		tagMap[idx] = pduValueToString(pdu.Value)
		return nil
	})

	if err != nil {
		return tagMap, err
	}

	return tagMap, nil
}

func (r *snmpRunner) walkMetric(g *gosnmp.GoSNMP, metricOID string, metricName string, indexOID string, tagMap map[string]string, baseLabels map[string]string, ts int64) []metrics.Metric {
	var ms []metrics.Metric

	err := g.Walk(metricOID, func(pdu gosnmp.SnmpPDU) error {
		idx := extractIndex(pdu.Name, metricOID, indexOID)
		if idx == "" {
			return nil
		}

		labels := make(map[string]string, len(baseLabels)+3)
		for k, v := range baseLabels {
			labels[k] = v
		}
		labels[indexOID] = idx

		if tagMap != nil {
			if tag, ok := tagMap[idx]; ok {
				labels["ifDescr"] = tag
			}
		}

		val := snmpValue(pdu)

		ms = append(ms, metrics.Metric{
			Name: metricName, Value: val, Labels: labels,
			Timestamp: time.Unix(ts, 0), Type: metricTypeForValue(val),
		})

		return nil
	})

	if err != nil {
		r.logger.Warn("walk failed", zap.String("oid", metricOID), zap.Error(err))
	}

	return ms
}

func snmpVersion(v string) gosnmp.SnmpVersion {
	switch v {
	case "1":
		return gosnmp.Version1
	case "2c":
		return gosnmp.Version2c
	case "3":
		return gosnmp.Version3
	default:
		return gosnmp.Version2c
	}
}

func snmpValue(pdu gosnmp.SnmpPDU) float64 {
	switch v := pdu.Value.(type) {
	case int:
		return float64(v)
	case int32:
		return float64(v)
	case int64:
		return float64(v)
	case uint:
		return float64(v)
	case uint32:
		return float64(v)
	case uint64:
		return float64(v)
	case float32:
		return float64(v)
	case float64:
		return v
	case string:
		f, err := strconv.ParseFloat(v, 64)
		if err == nil {
			return f
		}
		return 0
	case []byte:
		f, err := strconv.ParseFloat(string(v), 64)
		if err == nil {
			return f
		}
		return 0
	default:
		return 0
	}
}

func pduValueToString(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case []byte:
		return string(val)
	default:
		return fmt.Sprintf("%v", val)
	}
}

func extractIndex(oid, baseOID, indexOID string) string {
	cleanOID := strings.TrimPrefix(oid, ".")
	cleanBase := strings.TrimPrefix(baseOID, ".")

	if !strings.HasPrefix(cleanOID, cleanBase+".") {
		return ""
	}

	suffix := strings.TrimPrefix(cleanOID, cleanBase+".")
	if suffix == "" || suffix == cleanBase {
		return ""
	}

	parts := strings.SplitN(suffix, ".", 2)
	return parts[0]
}

func snmpMetricName(oid string) string {
	parts := strings.Split(oid, ".")
	if len(parts) > 0 {
		last := parts[len(parts)-1]
		return "snmp_oid_" + last
	}
	return "snmp_unknown"
}

func metricTypeForValue(val float64) metrics.MetricType {
	if val == float64(uint64(val)) && val > math.MaxInt32 {
		return metrics.TypeCounter
	}
	return metrics.TypeGauge
}
