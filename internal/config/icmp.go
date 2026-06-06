package config

import "time"

type ICMPConfig struct {
	HistogramBucketsMs []int        `yaml:"histogram_buckets_ms"`
	Targets            []ICMPTarget `yaml:"targets"`
}

func (c ICMPConfig) Validate() error {
	if len(c.HistogramBucketsMs) == 0 {
		c.HistogramBucketsMs = []int{1, 5, 10, 20, 50, 100, 200, 500, 1000}
	}
	return nil
}

type ICMPTarget struct {
	Host     string            `yaml:"host"`
	Interval time.Duration     `yaml:"interval"`
	Count    int               `yaml:"count"`
	Timeout  time.Duration     `yaml:"timeout"`
	Labels   map[string]string `yaml:"labels"`
}

func (t ICMPTarget) Validate() error {
	if t.Host == "" {
		return nil
	}
	if t.Interval <= 0 {
		t.Interval = 60 * time.Second
	}
	if t.Count <= 0 {
		t.Count = 4
	}
	if t.Timeout <= 0 {
		t.Timeout = 5 * time.Second
	}
	return nil
}
