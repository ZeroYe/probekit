package config

import "time"

type SNMPConfig struct {
	FlushInterval time.Duration `yaml:"flush_interval"`
	BatchSize     int           `yaml:"batch_size"`
	BufferSize    int           `yaml:"buffer_size"`
	Defaults      SNMPDefaults  `yaml:"defaults"`
	Targets       []SNMPTarget  `yaml:"targets"`
}

func (c SNMPConfig) Validate() error {
	c.Defaults.Validate()
	for i := range c.Targets {
		c.Targets[i].Validate()
	}
	return nil
}

func (c SNMPConfig) EffectiveVM(global VMConfig) VMConfig {
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

type SNMPDefaults struct {
	Version   string        `yaml:"version"`
	Community string        `yaml:"community"`
	Port      uint16        `yaml:"port"`
	Timeout   time.Duration `yaml:"timeout"`
	Retries   int           `yaml:"retries"`
}

func (d *SNMPDefaults) Validate() {
	if d.Version == "" {
		d.Version = "2c"
	}
	if d.Community == "" {
		d.Community = "public"
	}
	if d.Port <= 0 {
		d.Port = 161
	}
	if d.Timeout <= 0 {
		d.Timeout = 5 * time.Second
	}
	if d.Retries <= 0 {
		d.Retries = 1
	}
}

type SNMPTarget struct {
	Host     string            `yaml:"host"`
	Interval time.Duration     `yaml:"interval"`
	Timeout  time.Duration     `yaml:"timeout"`
	Labels   map[string]string `yaml:"labels"`
	OIDs     SNMPOIDs          `yaml:"oids"`
}

func (t *SNMPTarget) Validate() {
	if t.Interval <= 0 {
		t.Interval = 60 * time.Second
	}
	_ = t.Timeout
}

type SNMPOIDs struct {
	Scalar []string    `yaml:"scalar"`
	Tables []SNMPTable `yaml:"tables"`
}

type SNMPTable struct {
	OID     string             `yaml:"oid"`
	Index   string             `yaml:"index"`
	Tag     string             `yaml:"tag"`
	Metrics []SNMPTableMetric  `yaml:"metrics"`
}

type SNMPTableMetric struct {
	OID  string `yaml:"oid"`
	Name string `yaml:"name"`
	Type string `yaml:"type"`
}
