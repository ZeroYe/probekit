package config

import "time"

type PortConfig struct {
	Targets []PortTarget `yaml:"targets"`
}

func (c PortConfig) Validate() error {
	for i := range c.Targets {
		c.Targets[i].Validate()
	}
	return nil
}

type PortTarget struct {
	Host     string            `yaml:"host"`
	Port     int               `yaml:"port"`
	Protocol string            `yaml:"protocol"`
	Timeout  time.Duration     `yaml:"timeout"`
	Interval time.Duration     `yaml:"interval"`
	Labels   map[string]string `yaml:"labels"`
}

func (t *PortTarget) Validate() {
	if t.Port <= 0 {
		t.Port = 80
	}
	if t.Protocol == "" {
		t.Protocol = "tcp"
	}
	if t.Timeout <= 0 {
		t.Timeout = 5 * time.Second
	}
	if t.Interval <= 0 {
		t.Interval = 60 * time.Second
	}
}
