package config

import "time"

type DNSConfig struct {
	FlushInterval time.Duration `yaml:"flush_interval"`
	BatchSize     int           `yaml:"batch_size"`
	BufferSize    int           `yaml:"buffer_size"`
	Targets       []DNSTarget   `yaml:"targets"`
}

func (c DNSConfig) Validate() error {
	for i := range c.Targets {
		c.Targets[i].Validate()
	}
	return nil
}

func (c DNSConfig) EffectiveVM(global VMConfig) VMConfig {
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

type DNSTarget struct {
	Domain     string            `yaml:"domain"`
	Server     string            `yaml:"server"`
	RecordType string            `yaml:"record_type"`
	Interval   time.Duration     `yaml:"interval"`
	Timeout    time.Duration     `yaml:"timeout"`
	Labels     map[string]string `yaml:"labels"`
}

func (t *DNSTarget) Validate() {
	if t.Interval <= 0 {
		t.Interval = 60 * time.Second
	}
	if t.RecordType == "" {
		t.RecordType = "A"
	}
	if t.Timeout <= 0 {
		t.Timeout = 5 * time.Second
	}
}
