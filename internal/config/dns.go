package config

import "time"

type DNSConfig struct {
	Targets []DNSTarget `yaml:"targets"`
}

func (c DNSConfig) Validate() error {
	for i := range c.Targets {
		c.Targets[i].Validate()
	}
	return nil
}

type DNSTarget struct {
	Domain     string            `yaml:"domain"`
	Server     string            `yaml:"server"`
	RecordType string            `yaml:"record_type"`
	Interval   time.Duration     `yaml:"interval"`
	Labels     map[string]string `yaml:"labels"`
}

func (t *DNSTarget) Validate() {
	if t.Interval <= 0 {
		t.Interval = 60 * time.Second
	}
	if t.RecordType == "" {
		t.RecordType = "A"
	}
}
