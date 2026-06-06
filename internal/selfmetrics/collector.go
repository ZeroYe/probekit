package selfmetrics

import (
	"runtime"
	"time"
)

type SelfMetric struct {
	Name   string
	Help   string
	Type   string
	Value  float64
	Labels map[string]string
}

func Collect(collectRuntime bool, targetCounts map[string]int) []SelfMetric {
	ms := []SelfMetric{
		{
			Name: "probe_agent_info", Type: "gauge", Value: 1,
			Labels: map[string]string{
				"version":   BuildVersion,
				"goversion": GoVersion,
			},
		},
		{Name: "probe_agent_uptime_seconds", Type: "gauge", Value: time.Since(StartTime).Seconds()},
		{Name: "probe_agent_metrics_collected_total", Type: "counter", Value: float64(MetricsCollected.Load())},
		{Name: "probe_agent_metrics_pushed_total", Type: "counter", Value: float64(MetricsPushed.Load())},
		{Name: "probe_agent_push_errors_total", Type: "counter", Value: float64(PushErrors.Load())},
		{Name: "probe_agent_queue_length", Type: "gauge", Value: float64(QueueLength.Load())},
	}

	for module, count := range targetCounts {
		ms = append(ms, SelfMetric{
			Name: "probe_agent_targets", Type: "gauge", Value: float64(count),
			Labels: map[string]string{"module": module},
		})
	}

	if collectRuntime {
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)
		ms = append(ms,
			SelfMetric{Name: "probe_agent_goroutines", Type: "gauge", Value: float64(runtime.NumGoroutine())},
			SelfMetric{Name: "probe_agent_memory_alloc_bytes", Type: "gauge", Value: float64(mem.Alloc)},
			SelfMetric{Name: "probe_agent_memory_heap_bytes", Type: "gauge", Value: float64(mem.HeapAlloc)},
			SelfMetric{Name: "probe_agent_memory_sys_bytes", Type: "gauge", Value: float64(mem.Sys)},
			SelfMetric{Name: "probe_agent_gc_pauses_total", Type: "counter", Value: float64(mem.NumGC)},
		)
	}

	return ms
}
