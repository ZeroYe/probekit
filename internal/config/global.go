package config

import "time"

type GlobalConfig struct {
	Concurrency     int               `yaml:"concurrency"`
	LogLevel        string            `yaml:"log_level"`
	VictoriaMetrics VMConfig          `yaml:"victoria_metrics"`
	MCPServer       MCPConfig         `yaml:"mcp_server"`
	SelfMetrics     SelfMetricsConfig `yaml:"self_metrics"`
}

func (g GlobalConfig) Validate() error {
	if g.Concurrency <= 0 {
		g.Concurrency = 20
	}
	if g.LogLevel == "" {
		g.LogLevel = "info"
	}
	g.SelfMetrics.Validate()
	return g.VictoriaMetrics.Validate()
}

type SelfMetricsConfig struct {
	Enabled        bool   `yaml:"enabled"`
	Path           string `yaml:"path"`
	CollectRuntime bool   `yaml:"collect_runtime"`
}

func (s *SelfMetricsConfig) Validate() {
	if s.Path == "" {
		s.Path = "/metrics"
	}
}

type VMConfig struct {
	PushURL       string        `yaml:"push_url"`
	BatchSize     int           `yaml:"batch_size"`
	FlushInterval time.Duration `yaml:"flush_interval"`
	BufferSize    int           `yaml:"buffer_size"`
	Retry         RetryConfig   `yaml:"retry"`
}

func (v VMConfig) Validate() error {
	if v.BatchSize <= 0 {
		v.BatchSize = 500
	}
	if v.FlushInterval <= 0 {
		v.FlushInterval = 10 * time.Second
	}
	if v.BufferSize <= 0 {
		v.BufferSize = 10000
	}
	return nil
}

type RetryConfig struct {
	MaxRetries     int           `yaml:"max_retries"`
	InitialBackoff time.Duration `yaml:"initial_backoff"`
	MaxBackoff     time.Duration `yaml:"max_backoff"`
}

type MCPConfig struct {
	Enabled bool       `yaml:"enabled"`
	Listen  string     `yaml:"listen"`
	Auth    AuthConfig `yaml:"auth"`
}

type AuthConfig struct {
	Type string   `yaml:"type"`
	Keys []string `yaml:"keys"`
}
