package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

type ICMPConfig struct {
	FlushInterval      time.Duration `yaml:"flush_interval"`
	BatchSize          int           `yaml:"batch_size"`
	BufferSize         int           `yaml:"buffer_size"`
	HistogramBucketsMs []int         `yaml:"histogram_buckets_ms"`
	Targets            []ICMPTarget  `yaml:"targets"`
	TargetsFile        string        `yaml:"targets_file"`
}

func (c ICMPConfig) Validate() error {
	if len(c.HistogramBucketsMs) == 0 {
		c.HistogramBucketsMs = []int{1, 5, 10, 20, 50, 100, 200, 500, 1000}
	}
	return nil
}

func (c *ICMPConfig) LoadTargetsFile(baseDir string) error {
	if c.TargetsFile == "" {
		return nil
	}
	path := c.TargetsFile
	if !filepath.IsAbs(path) {
		path = filepath.Join(baseDir, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("targets_file %s: %w", c.TargetsFile, err)
	}
	var externalTargets []ICMPTarget
	if err := yaml.Unmarshal(data, &externalTargets); err != nil {
		return fmt.Errorf("targets_file %s: %w", c.TargetsFile, err)
	}
	c.Targets = append(c.Targets, externalTargets...)
	return nil
}

func (c ICMPConfig) EffectiveVM(global VMConfig) VMConfig {
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
