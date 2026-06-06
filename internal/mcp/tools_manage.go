package mcp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"probe-agent/internal/config"
	mcpcore "github.com/mark3labs/mcp-go/mcp"
	"gopkg.in/yaml.v3"
)

func (s *Server) registerAddTarget() {
	tool := mcpcore.NewTool("add_target",
		mcpcore.WithDescription("Add a new monitoring target and reload config"),
		mcpcore.WithString("module",
			mcpcore.Required(),
			mcpcore.Description("Module: icmp, snmp, or dns"),
		),
		mcpcore.WithString("host",
			mcpcore.Required(),
			mcpcore.Description("Target host/IP (or domain for dns)"),
		),
		mcpcore.WithString("interval",
			mcpcore.Description("Probe interval (e.g. 30s, 1m)"),
		),
		mcpcore.WithString("labels",
			mcpcore.Description("Optional labels as JSON: {\"region\":\"us\"}"),
		),
		mcpcore.WithString("server",
			mcpcore.Description("DNS server IP (dns module only)"),
		),
		mcpcore.WithString("record_type",
			mcpcore.Description("DNS record type: A, AAAA, MX, NS, CNAME, TXT (dns module only)"),
		),
		mcpcore.WithString("community",
			mcpcore.Description("SNMP community (snmp module only)"),
		),
	)

	s.mcpServer.AddTool(tool, s.handleAddTarget)
}

func (s *Server) handleAddTarget(ctx context.Context, req mcpcore.CallToolRequest) (*mcpcore.CallToolResult, error) {
	module := argString(req, "module")
	host := argString(req, "host")

	if module == "" || host == "" {
		return mcpcore.NewToolResultError("module and host are required"), nil
	}

	interval, _ := time.ParseDuration(argString(req, "interval"))

	var labels map[string]string
	if s := argString(req, "labels"); s != "" {
		yaml.Unmarshal([]byte(s), &labels)
	}

	switch module {
	case "icmp":
		s.addICMPTarget(host, interval, labels)
	case "dns":
		server := argString(req, "server")
		recordType := argString(req, "record_type")
		if server == "" {
			server = "8.8.8.8"
		}
		if recordType == "" {
			recordType = "A"
		}
		s.addDNSTarget(host, server, recordType, interval, labels)
	case "snmp":
		s.addSNMPTarget(host, interval, labels)
	default:
		return mcpcore.NewToolResultError(fmt.Sprintf("unknown module: %s", module)), nil
	}

	s.reload()

	return mcpcore.NewToolResultText(fmt.Sprintf("added target %s to %s and reloaded config", host, module)), nil
}

func (s *Server) registerRemoveTarget() {
	tool := mcpcore.NewTool("remove_target",
		mcpcore.WithDescription("Remove a monitoring target and reload config"),
		mcpcore.WithString("module",
			mcpcore.Required(),
			mcpcore.Description("Module: icmp, snmp, or dns"),
		),
		mcpcore.WithString("host",
			mcpcore.Required(),
			mcpcore.Description("Target host/IP (or domain for dns)"),
		),
	)

	s.mcpServer.AddTool(tool, s.handleRemoveTarget)
}

func (s *Server) handleRemoveTarget(ctx context.Context, req mcpcore.CallToolRequest) (*mcpcore.CallToolResult, error) {
	module := argString(req, "module")
	host := argString(req, "host")

	if module == "" || host == "" {
		return mcpcore.NewToolResultError("module and host are required"), nil
	}

	var found bool

	switch module {
	case "icmp":
		found = s.removeICMPTarget(host)
	case "dns":
		found = s.removeDNSTarget(host)
	case "snmp":
		found = s.removeSNMPTarget(host)
	default:
		return mcpcore.NewToolResultError(fmt.Sprintf("unknown module: %s", module)), nil
	}

	if !found {
		return mcpcore.NewToolResultText(fmt.Sprintf("target %s not found in %s", host, module)), nil
	}

	s.reload()

	return mcpcore.NewToolResultText(fmt.Sprintf("removed target %s from %s and reloaded config", host, module)), nil
}

func (s *Server) registerReloadConfig() {
	tool := mcpcore.NewTool("reload_config",
		mcpcore.WithDescription("Reload all config files from disk"),
	)

	s.mcpServer.AddTool(tool, s.handleReloadConfig)
}

func (s *Server) handleReloadConfig(ctx context.Context, req mcpcore.CallToolRequest) (*mcpcore.CallToolResult, error) {
	if err := s.reload(); err != nil {
		return mcpcore.NewToolResultError(err.Error()), nil
	}
	return mcpcore.NewToolResultText("config reloaded successfully"), nil
}

func (s *Server) reload() error {
	if s.deps.OnReload != nil {
		return s.deps.OnReload()
	}
	return nil
}

func (s *Server) addICMPTarget(host string, interval time.Duration, labels map[string]string) {
	cfg := &config.ICMPConfig{}
	path := filepath.Join(s.deps.ConfigDir, "icmp.yaml")
	readYAMLFile(path, cfg)

	if interval <= 0 {
		interval = 60 * time.Second
	}

	cfg.Targets = append(cfg.Targets, config.ICMPTarget{
		Host:     host,
		Interval: interval,
		Count:    4,
		Timeout:  5 * time.Second,
		Labels:   labels,
	})

	writeYAMLFile(path, cfg)
}

func (s *Server) addDNSTarget(domain, server, recordType string, interval time.Duration, labels map[string]string) {
	cfg := &config.DNSConfig{}
	path := filepath.Join(s.deps.ConfigDir, "dns.yaml")
	readYAMLFile(path, cfg)

	if interval <= 0 {
		interval = 60 * time.Second
	}

	cfg.Targets = append(cfg.Targets, config.DNSTarget{
		Domain:     domain,
		Server:     server,
		RecordType: recordType,
		Interval:   interval,
		Labels:     labels,
	})

	writeYAMLFile(path, cfg)
}

func (s *Server) addSNMPTarget(host string, interval time.Duration, labels map[string]string) {
	cfg := &config.SNMPConfig{}
	path := filepath.Join(s.deps.ConfigDir, "snmp.yaml")
	readYAMLFile(path, cfg)

	if interval <= 0 {
		interval = 60 * time.Second
	}

	cfg.Targets = append(cfg.Targets, config.SNMPTarget{
		Host:     host,
		Interval: interval,
		Labels:   labels,
	})

	writeYAMLFile(path, cfg)
}

func (s *Server) removeICMPTarget(host string) bool {
	cfg := &config.ICMPConfig{}
	path := filepath.Join(s.deps.ConfigDir, "icmp.yaml")
	readYAMLFile(path, cfg)

	filtered := cfg.Targets[:0]
	found := false
	for _, t := range cfg.Targets {
		if t.Host == host {
			found = true
			continue
		}
		filtered = append(filtered, t)
	}
	if !found {
		return false
	}
	cfg.Targets = filtered
	writeYAMLFile(path, cfg)
	return true
}

func (s *Server) removeDNSTarget(host string) bool {
	cfg := &config.DNSConfig{}
	path := filepath.Join(s.deps.ConfigDir, "dns.yaml")
	readYAMLFile(path, cfg)

	filtered := cfg.Targets[:0]
	found := false
	for _, t := range cfg.Targets {
		if t.Domain == host {
			found = true
			continue
		}
		filtered = append(filtered, t)
	}
	if !found {
		return false
	}
	cfg.Targets = filtered
	writeYAMLFile(path, cfg)
	return true
}

func (s *Server) removeSNMPTarget(host string) bool {
	cfg := &config.SNMPConfig{}
	path := filepath.Join(s.deps.ConfigDir, "snmp.yaml")
	readYAMLFile(path, cfg)

	filtered := cfg.Targets[:0]
	found := false
	for _, t := range cfg.Targets {
		if t.Host == host {
			found = true
			continue
		}
		filtered = append(filtered, t)
	}
	if !found {
		return false
	}
	cfg.Targets = filtered
	writeYAMLFile(path, cfg)
	return true
}

func readYAMLFile(path string, out any) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	yaml.Unmarshal(data, out)
}

func writeYAMLFile(path string, v any) error {
	data, err := yaml.Marshal(v)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
