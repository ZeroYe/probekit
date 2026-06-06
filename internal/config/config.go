package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Global GlobalConfig
	ICMP   ICMPConfig
	SNMP   SNMPConfig
	DNS    DNSConfig
}

func Load(configDir string) (*Config, error) {
	cfg := &Config{}

	if err := loadYAML(filepath.Join(configDir, "global.yaml"), &cfg.Global); err != nil {
		return nil, fmt.Errorf("global config: %w", err)
	}

	if err := loadYAML(filepath.Join(configDir, "icmp.yaml"), &cfg.ICMP); err != nil {
		return nil, fmt.Errorf("icmp config: %w", err)
	}

	if err := loadYAML(filepath.Join(configDir, "snmp.yaml"), &cfg.SNMP); err != nil {
		return nil, fmt.Errorf("snmp config: %w", err)
	}

	if err := loadYAML(filepath.Join(configDir, "dns.yaml"), &cfg.DNS); err != nil {
		return nil, fmt.Errorf("dns config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation: %w", err)
	}

	return cfg, nil
}

func (c *Config) Validate() error {
	if err := c.Global.Validate(); err != nil {
		return err
	}
	if err := c.ICMP.Validate(); err != nil {
		return err
	}
	if err := c.SNMP.Validate(); err != nil {
		return err
	}
	return c.DNS.Validate()
}

func loadYAML(path string, out any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read %s: %w", path, err)
	}

	if err := yaml.Unmarshal(data, out); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}

	return nil
}
