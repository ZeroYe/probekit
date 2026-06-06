package config

import (
	"net/http"
	"time"
)

type HTTPConfig struct {
	FlushInterval time.Duration `yaml:"flush_interval"`
	BatchSize     int           `yaml:"batch_size"`
	BufferSize    int           `yaml:"buffer_size"`
	Targets       []HTTPTarget  `yaml:"targets"`
}

func (c HTTPConfig) Validate() error {
	for i := range c.Targets {
		c.Targets[i].Validate()
	}
	return nil
}

func (c HTTPConfig) EffectiveVM(global VMConfig) VMConfig {
	if c.FlushInterval > 0 {
		global.FlushInterval = c.FlushInterval
	}
	if c.BatchSize > 0 {
		global.BatchSize = c.BatchSize
	}
	if c.BufferSize > 0 {
		global.BufferSize = c.BufferSize
	}
	return global
}

type HTTPTarget struct {
	URL                  string            `yaml:"url"`
	Method               string            `yaml:"method"`
	ExpectedStatusCodes  []int             `yaml:"expected_status_codes"`
	ExpectedBodyContains string            `yaml:"expected_body_contains"`
	Timeout              time.Duration     `yaml:"timeout"`
	Interval             time.Duration     `yaml:"interval"`
	Headers              map[string]string `yaml:"headers"`
	Labels               map[string]string `yaml:"labels"`
}

func (t *HTTPTarget) Validate() {
	if t.URL == "" {
		return
	}
	if t.Method == "" {
		t.Method = http.MethodGet
	}
	if len(t.ExpectedStatusCodes) == 0 {
		t.ExpectedStatusCodes = []int{200}
	}
	if t.Timeout <= 0 {
		t.Timeout = 10 * time.Second
	}
	if t.Interval <= 0 {
		t.Interval = 60 * time.Second
	}
}
