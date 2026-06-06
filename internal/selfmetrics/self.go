package selfmetrics

import (
	"runtime"
	"sync/atomic"
	"time"
)

var (
	BuildVersion = "dev"
	StartTime    = time.Now()
	GoVersion    = runtime.Version()

	MetricsCollected atomic.Int64
	MetricsPushed    atomic.Int64
	PushErrors       atomic.Int64
	QueueLength      atomic.Int64
)
