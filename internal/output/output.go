package output

import "github.com/ZeroYe/probekit/internal/metrics"

type Output interface {
	Name() string
	Start() error
	Stop() error
	Write(metrics []metrics.Metric) error
}
