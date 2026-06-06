package collector

import (
	"context"
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
	cfg      config.SNMPConfig
	runners  []*snmpRunner
	logger   *zap.Logger
	pipeline *output.Pipeline
}

func NewSNMPCollector(cfg config.SNMPConfig, logger *zap.Logger, pipeline *output.Pipeline) *SNMPCollector {
	return &SNMPCollector{
		cfg:      cfg,
		logger:   logger.Named("snmp"),
		pipeline: pipeline,
	}
}

func (c *SNMPCollector) Name() string { return "snmp" }

func (c *SNMPCollector) Start(ctx context.Context) error {
	if len(c.cfg.Targets) == 0 {
		c.logger.Info("no targets, skipping")
		return nil
	}

	for _, t := range c.cfg.Targets {
		target := t
		runner := newSNMPRunner(target, c.cfg.Defaults, c.logger)
		c.runners = append(c.runners, runner)
		go runner.run(ctx, c.pipeline)
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
	timeout := r.target.Timeout
	if timeout <= 0 {
		timeout = r.defaults.Timeout
	}

	port := r.defaults.Port

	gs := &gosnmp.GoSNMP{
		Target:    r.target.Host,
		Port:      port,
		Version:   snmpVersion(r.defaults.Version),
		Community: r.defaults.Community,
		Timeout:   timeout,
		Retries:   r.defaults.Retries,
	}

	err := gs.Connect()
	if err != nil {
		r.logger.Warn("snmp connect failed", zap.Error(err))
		r.reportUp(0, pipeline)
		return
	}
	defer gs.Conn.Close()

	now := time.Now().Unix()
	labels := targetLabels(r.target.Host, r.target.Labels, nil)

	var ms []metrics.Metric

	// Scalar OIDs
	for _, oid := range r.target.OIDs.Scalar {
		result, err := gs.Get([]string{oid})
		if err != nil {
			r.logger.Warn("snmp get failed", zap.String("oid", oid), zap.Error(err))
			continue
		}
		for _, v := range result.Variables {
			ms = append(ms, metrics.Metric{
				Name:  oidToName(v.Name),
				Value: snmpValue(v),
				Labels: targetLabels(r.target.Host, r.target.Labels, map[string]string{
					"oid": v.Name,
				}),
				Timestamp: time.Unix(now, 0),
				Type:      metrics.TypeGauge,
			})
		}
	}

	// Table OIDs
	for _, table := range r.target.OIDs.Tables {
		results, err := gs.BulkWalkAll(table.OID)
		if err != nil {
			r.logger.Warn("snmp walk failed", zap.String("oid", table.OID), zap.Error(err))
			continue
		}

		rows := groupTableRows(results, table.Index)
		for _, row := range rows {
			for _, m := range table.Metrics {
				if v, ok := row.pdus[m.OID]; ok {
					rowLabels := targetLabels(r.target.Host, r.target.Labels, row.labels)
					ms = append(ms, metrics.Metric{
						Name:      m.Name,
						Value:     snmpValue(v),
						Labels:    rowLabels,
						Timestamp: time.Unix(now, 0),
						Type:      metrics.TypeGauge,
					})
				}
			}
		}
	}

	ms = append(ms, metrics.Metric{
		Name: "snmp_up", Value: 1, Labels: copyLabels(labels),
		Timestamp: time.Unix(now, 0), Type: metrics.TypeGauge,
	})

	pipeline.Submit("snmp/"+r.target.Host, ms)
}

func (r *snmpRunner) reportUp(up float64, pipeline *output.Pipeline) {
	now := time.Now().Unix()
	labels := targetLabels(r.target.Host, r.target.Labels, nil)
	ms := []metrics.Metric{
		{Name: "snmp_up", Value: up, Labels: labels, Timestamp: time.Unix(now, 0), Type: metrics.TypeGauge},
	}
	pipeline.Submit("snmp/"+r.target.Host, ms)
}

func snmpVersion(v string) gosnmp.SnmpVersion {
	switch strings.ToLower(v) {
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

func oidToName(oid string) string {
	return strings.ReplaceAll(oid, ".", "_")
}

func snmpValue(v gosnmp.SnmpPDU) float64 {
	switch val := v.Value.(type) {
	case int:
		return float64(val)
	case int32:
		return float64(val)
	case int64:
		return float64(val)
	case uint:
		return float64(val)
	case uint32:
		return float64(val)
	case uint64:
		return float64(val)
	case float32:
		return float64(val)
	case float64:
		return val
	case string:
		f, _ := strconv.ParseFloat(val, 64)
		return f
	case []byte:
		f, _ := strconv.ParseFloat(string(val), 64)
		return f
	default:
		return 0
	}
}

type tableRow struct {
	labels map[string]string
	pdus   map[string]gosnmp.SnmpPDU
}

func groupTableRows(pdus []gosnmp.SnmpPDU, indexOID string) []*tableRow {
	rows := make(map[string]*tableRow)

	for _, pdu := range pdus {
		idx := extractIndex(pdu.Name, indexOID)
		if idx == "" {
			continue
		}

		row, ok := rows[idx]
		if !ok {
			row = &tableRow{
				labels: map[string]string{},
				pdus:   make(map[string]gosnmp.SnmpPDU),
			}
			rows[idx] = row
		}

		row.pdus[pdu.Name] = pdu
	}

	result := make([]*tableRow, 0, len(rows))
	for _, row := range rows {
		row.labels = extractLabels(row.pdus)
		result = append(result, row)
	}
	return result
}

func extractIndex(oid, baseOID string) string {
	if !strings.HasPrefix(oid, baseOID) {
		return ""
	}
	idx := strings.TrimPrefix(oid, baseOID)
	idx = strings.TrimPrefix(idx, ".")
	return idx
}

func extractLabels(pdus map[string]gosnmp.SnmpPDU) map[string]string {
	labels := make(map[string]string)
	for oid, pdu := range pdus {
		if str, ok := pdu.Value.(string); ok {
			labels[oid] = str
		}
	}
	return labels
}
