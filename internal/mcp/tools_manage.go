package mcp

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ZeroYe/probekit/internal/config"
	mcpcore "github.com/mark3labs/mcp-go/mcp"
	"gopkg.in/yaml.v3"
)

type csvTarget struct {
	host         string
	port         int
	protocol     string
	interval     time.Duration
	timeout      time.Duration
	count        int
	server       string
	recordType   string
	method       string
	bodyContains string
	labels       map[string]string
}

func (s *Server) registerAddTarget() {
	tool := mcpcore.NewTool("add_target",
		mcpcore.WithDescription("Add a new monitoring target and reload config"),
		mcpcore.WithString("module",
			mcpcore.Required(),
			mcpcore.Description("Module: icmp, dns, snmp, port, http"),
		),
		mcpcore.WithString("host",
			mcpcore.Required(),
			mcpcore.Description("Target host/IP (or domain for dns, URL for http)"),
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
		mcpcore.WithNumber("port",
			mcpcore.Description("Port number (port module only)"),
		),
		mcpcore.WithString("protocol",
			mcpcore.Description("Protocol: tcp/udp (port module only, default tcp)"),
		),
		mcpcore.WithString("method",
			mcpcore.Description("HTTP method (http module only, default GET)"),
		),
		mcpcore.WithString("expected_body_contains",
			mcpcore.Description("Expected body substring (http module only)"),
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
	case "port":
		port := int(argNumber(req, "port"))
		if port <= 0 {
			port = 80
		}
		protocol := argString(req, "protocol")
		if protocol == "" {
			protocol = "tcp"
		}
		s.addPortTarget(host, port, protocol, interval, labels)
	case "http":
		method := argString(req, "method")
		if method == "" {
			method = "GET"
		}
		bodyContains := argString(req, "expected_body_contains")
		s.addHTTPTarget(host, method, bodyContains, interval, labels)
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
			mcpcore.Description("Module: icmp, dns, snmp, port, http"),
		),
		mcpcore.WithString("host",
			mcpcore.Required(),
			mcpcore.Description("Target host/IP (or domain for dns, URL for http)"),
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
	case "port":
		found = s.removePortTarget(host)
	case "http":
		found = s.removeHTTPTarget(host)
	default:
		return mcpcore.NewToolResultError(fmt.Sprintf("unknown module: %s", module)), nil
	}

	if !found {
		return mcpcore.NewToolResultText(fmt.Sprintf("target %s not found in %s", host, module)), nil
	}

	s.reload()

	return mcpcore.NewToolResultText(fmt.Sprintf("removed target %s from %s and reloaded config", host, module)), nil
}

func (s *Server) registerBatchAddTargets() {
	tool := mcpcore.NewTool("batch_add_targets",
		mcpcore.WithDescription("Batch add targets from hosts/CIDR or CSV (inline or file), then reload"),
		mcpcore.WithString("module",
			mcpcore.Required(),
			mcpcore.Description("Module: icmp, dns, snmp, port, http"),
		),
		mcpcore.WithString("hosts",
			mcpcore.Description("Comma-separated IPs/domains, or CIDR (e.g. 10.0.0.0/24). Not needed if csv or csv_file is provided."),
		),
		mcpcore.WithString("csv",
			mcpcore.Description("CSV text with per-row fields (header + data). For small batches (<100 rows)."),
		),
		mcpcore.WithString("csv_file",
			mcpcore.Description("Path to a local CSV file on the server. Recommended for 100+ targets."),
		),
		mcpcore.WithString("interval",
			mcpcore.Description("Probe interval (e.g. 30s, 1m). Used with hosts param; for csv use the interval column."),
		),
		mcpcore.WithString("labels",
			mcpcore.Description("Optional labels as JSON: {\"region\":\"us\"}. Used with hosts param; for csv use labels column."),
		),
		mcpcore.WithNumber("count",
			mcpcore.Description("ICMP packet count (icmp only, default 4)"),
		),
		mcpcore.WithString("timeout",
			mcpcore.Description("Probe timeout (default 5s)"),
		),
		mcpcore.WithNumber("port",
			mcpcore.Description("Port number (port module only)"),
		),
		mcpcore.WithString("protocol",
			mcpcore.Description("Protocol: tcp/udp (port module only, default tcp)"),
		),
		mcpcore.WithString("method",
			mcpcore.Description("HTTP method (http module only, default GET)"),
		),
		mcpcore.WithString("expected_body_contains",
			mcpcore.Description("Expected body substring (http module only)"),
		),
	)

	s.mcpServer.AddTool(tool, s.handleBatchAddTargets)
}

func (s *Server) handleBatchAddTargets(ctx context.Context, req mcpcore.CallToolRequest) (*mcpcore.CallToolResult, error) {
	module := argString(req, "module")
	hostsStr := argString(req, "hosts")
	csvStr := argString(req, "csv")
	csvFile := argString(req, "csv_file")

	if module == "" {
		return mcpcore.NewToolResultError("module is required"), nil
	}

	sources := 0
	if hostsStr != "" {
		sources++
	}
	if csvStr != "" {
		sources++
	}
	if csvFile != "" {
		sources++
	}
	if sources != 1 {
		return mcpcore.NewToolResultError("exactly one of hosts, csv, or csv_file is required"), nil
	}

	if csvStr != "" {
		return s.handleBatchAddCSV(module, csvStr)
	}

	if csvFile != "" {
		data, err := os.ReadFile(csvFile)
		if err != nil {
			return mcpcore.NewToolResultError(fmt.Sprintf("read csv_file: %s", err)), nil
		}
		return s.handleBatchAddCSV(module, string(data))
	}

	interval, _ := time.ParseDuration(argString(req, "interval"))
	timeout, _ := time.ParseDuration(argString(req, "timeout"))

	var labels map[string]string
	if s := argString(req, "labels"); s != "" {
		yaml.Unmarshal([]byte(s), &labels)
	}

	count := int(argNumber(req, "count"))
	port := int(argNumber(req, "port"))
	protocol := argString(req, "protocol")
	method := argString(req, "method")
	bodyContains := argString(req, "expected_body_contains")

	hosts := expandHosts(hostsStr)
	if len(hosts) == 0 {
		return mcpcore.NewToolResultError("no valid hosts found"), nil
	}

	switch module {
	case "icmp":
		s.batchAddICMP(hosts, interval, timeout, count, labels)
	case "dns":
		s.batchAddDNS(hosts, interval, labels)
	case "snmp":
		s.batchAddSNMP(hosts, interval, labels)
	case "port":
		s.batchAddPort(hosts, port, protocol, interval, timeout, labels)
	case "http":
		s.batchAddHTTP(hosts, method, bodyContains, interval, timeout, labels)
	default:
		return mcpcore.NewToolResultError(fmt.Sprintf("unknown module: %s", module)), nil
	}

	s.reload()

	return mcpcore.NewToolResultText(fmt.Sprintf("added %d targets to %s and reloaded config", len(hosts), module)), nil
}

func (s *Server) handleBatchAddCSV(module, csvStr string) (*mcpcore.CallToolResult, error) {
	lines := strings.Split(strings.TrimSpace(csvStr), "\n")
	if len(lines) < 2 {
		return mcpcore.NewToolResultError("csv must have at least a header row and one data row"), nil
	}

	header := strings.Split(strings.TrimSpace(lines[0]), ",")
	for i := range header {
		header[i] = strings.TrimSpace(strings.ToLower(header[i]))
	}

	var targets []csvTarget
	for _, line := range lines[1:] {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		vals := parseCSVLine(line)
		if len(vals) == 0 {
			continue
		}

		t := csvTarget{}
		for i, col := range header {
			val := ""
			if i < len(vals) {
				val = strings.TrimSpace(vals[i])
			}
			switch col {
			case "host", "ip", "domain", "url":
				t.host = val
			case "port":
				t.port, _ = strconv.Atoi(val)
			case "protocol":
				t.protocol = val
			case "interval":
				t.interval, _ = time.ParseDuration(val)
			case "timeout":
				t.timeout, _ = time.ParseDuration(val)
			case "count":
				t.count, _ = strconv.Atoi(val)
			case "server":
				t.server = val
			case "record_type", "recordtype", "type":
				t.recordType = val
			case "method":
				t.method = val
			case "expected_body_contains", "body_contains", "bodycontains":
				t.bodyContains = val
			case "labels", "tags":
				t.labels = parseCSVLabels(val)
			}
		}
		if t.host != "" {
			targets = append(targets, t)
		}
	}

	if len(targets) == 0 {
		return mcpcore.NewToolResultError("no valid targets found in csv"), nil
	}

	switch module {
	case "icmp":
		s.batchAddICMPFromCSV(targets)
	case "dns":
		s.batchAddDNSFromCSV(targets)
	case "snmp":
		s.batchAddSNMPFromCSV(targets)
	case "port":
		s.batchAddPortFromCSV(targets)
	case "http":
		s.batchAddHTTPFromCSV(targets)
	default:
		return mcpcore.NewToolResultError(fmt.Sprintf("unknown module: %s", module)), nil
	}

	s.reload()
	return mcpcore.NewToolResultText(fmt.Sprintf("added %d targets from csv to %s and reloaded config", len(targets), module)), nil
}

func parseCSVLine(line string) []string {
	var vals []string
	cur := strings.Builder{}
	inQuotes := false
	for _, ch := range line {
		switch {
		case ch == '"':
			inQuotes = !inQuotes
		case ch == ',' && !inQuotes:
			vals = append(vals, cur.String())
			cur.Reset()
		default:
			cur.WriteRune(ch)
		}
	}
	vals = append(vals, cur.String())
	return vals
}

func parseCSVLabels(s string) map[string]string {
	if s == "" {
		return nil
	}
	m := make(map[string]string)
	for _, pair := range strings.Split(s, "|") {
		pair = strings.TrimSpace(pair)
		if kv := strings.SplitN(pair, "=", 2); len(kv) == 2 {
			m[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
		}
	}
	return m
}

func expandHosts(hostsStr string) []string {
	if strings.Contains(hostsStr, "/") {
		_, ipnet, err := net.ParseCIDR(hostsStr)
		if err == nil {
			var ips []string
			ip := ipnet.IP.Mask(ipnet.Mask)
			for ip := ip.Mask(ipnet.Mask); ipnet.Contains(ip); incIP(ip) {
				if ipnet.Contains(ip) && !ip.Equal(ipnet.IP) {
					ips = append(ips, ip.String())
				}
			}
			return ips
		}
	}

	var hosts []string
	for _, h := range strings.Split(hostsStr, ",") {
		h = strings.TrimSpace(h)
		if h != "" {
			hosts = append(hosts, h)
		}
	}
	return hosts
}

func incIP(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}

func (s *Server) batchAddICMP(hosts []string, interval, timeout time.Duration, count int, labels map[string]string) {
	if interval <= 0 {
		interval = 60 * time.Second
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	if count <= 0 {
		count = 4
	}

	var targets []config.ICMPTarget
	for _, host := range hosts {
		targets = append(targets, config.ICMPTarget{
			Host:     host,
			Interval: interval,
			Count:    count,
			Timeout:  timeout,
			Labels:   copyLabels(labels),
		})
	}

	s.appendICMPTargets(targets)
}

func (s *Server) batchAddDNS(hosts []string, interval time.Duration, labels map[string]string) {
	if interval <= 0 {
		interval = 60 * time.Second
	}

	var targets []config.DNSTarget
	for _, host := range hosts {
		targets = append(targets, config.DNSTarget{
			Domain:   host,
			Server:   "8.8.8.8",
			Interval: interval,
			Labels:   copyLabels(labels),
		})
	}

	s.appendDNSTargets(targets)
}

func (s *Server) batchAddSNMP(hosts []string, interval time.Duration, labels map[string]string) {
	if interval <= 0 {
		interval = 60 * time.Second
	}

	var targets []config.SNMPTarget
	for _, host := range hosts {
		targets = append(targets, config.SNMPTarget{
			Host:     host,
			Interval: interval,
			Labels:   copyLabels(labels),
		})
	}

	s.appendSNMPTargets(targets)
}

func (s *Server) batchAddPort(hosts []string, port int, protocol string, interval, timeout time.Duration, labels map[string]string) {
	if interval <= 0 {
		interval = 60 * time.Second
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	if port <= 0 {
		port = 80
	}
	if protocol == "" {
		protocol = "tcp"
	}

	var targets []config.PortTarget
	for _, host := range hosts {
		targets = append(targets, config.PortTarget{
			Host:     host,
			Port:     port,
			Protocol: protocol,
			Timeout:  timeout,
			Interval: interval,
			Labels:   copyLabels(labels),
		})
	}

	s.appendPortTargets(targets)
}

func (s *Server) batchAddHTTP(hosts []string, method, bodyContains string, interval, timeout time.Duration, labels map[string]string) {
	if interval <= 0 {
		interval = 60 * time.Second
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	if method == "" {
		method = "GET"
	}

	var targets []config.HTTPTarget
	for _, host := range hosts {
		targets = append(targets, config.HTTPTarget{
			URL:                  host,
			Method:               method,
			ExpectedStatusCodes:  []int{200},
			ExpectedBodyContains: bodyContains,
			Timeout:              timeout,
			Interval:             interval,
			Labels:               copyLabels(labels),
		})
	}

	s.appendHTTPTargets(targets)
}

// CSV batch helpers

func (s *Server) batchAddICMPFromCSV(targets []csvTarget) {
	var out []config.ICMPTarget
	for _, t := range targets {
		iv := t.interval
		if iv <= 0 {
			iv = 60 * time.Second
		}
		tv := t.timeout
		if tv <= 0 {
			tv = 5 * time.Second
		}
		ct := t.count
		if ct <= 0 {
			ct = 4
		}
		out = append(out, config.ICMPTarget{
			Host:     t.host,
			Interval: iv,
			Count:    ct,
			Timeout:  tv,
			Labels:   t.labels,
		})
	}
	s.appendICMPTargets(out)
}

func (s *Server) batchAddDNSFromCSV(targets []csvTarget) {
	var out []config.DNSTarget
	for _, t := range targets {
		iv := t.interval
		if iv <= 0 {
			iv = 60 * time.Second
		}
		sv := t.server
		if sv == "" {
			sv = "8.8.8.8"
		}
		rt := t.recordType
		if rt == "" {
			rt = "A"
		}
		out = append(out, config.DNSTarget{
			Domain:     t.host,
			Server:     sv,
			RecordType: rt,
			Interval:   iv,
			Labels:     t.labels,
		})
	}
	s.appendDNSTargets(out)
}

func (s *Server) batchAddSNMPFromCSV(targets []csvTarget) {
	var out []config.SNMPTarget
	for _, t := range targets {
		iv := t.interval
		if iv <= 0 {
			iv = 60 * time.Second
		}
		out = append(out, config.SNMPTarget{
			Host:     t.host,
			Interval: iv,
			Labels:   t.labels,
		})
	}
	s.appendSNMPTargets(out)
}

func (s *Server) batchAddPortFromCSV(targets []csvTarget) {
	var out []config.PortTarget
	for _, t := range targets {
		iv := t.interval
		if iv <= 0 {
			iv = 60 * time.Second
		}
		tv := t.timeout
		if tv <= 0 {
			tv = 5 * time.Second
		}
		p := t.port
		if p <= 0 {
			p = 80
		}
		prot := t.protocol
		if prot == "" {
			prot = "tcp"
		}
		out = append(out, config.PortTarget{
			Host:     t.host,
			Port:     p,
			Protocol: prot,
			Timeout:  tv,
			Interval: iv,
			Labels:   t.labels,
		})
	}
	s.appendPortTargets(out)
}

func (s *Server) batchAddHTTPFromCSV(targets []csvTarget) {
	var out []config.HTTPTarget
	for _, t := range targets {
		iv := t.interval
		if iv <= 0 {
			iv = 60 * time.Second
		}
		tv := t.timeout
		if tv <= 0 {
			tv = 10 * time.Second
		}
		m := t.method
		if m == "" {
			m = "GET"
		}
		out = append(out, config.HTTPTarget{
			URL:                  t.host,
			Method:               m,
			ExpectedStatusCodes:  []int{200},
			ExpectedBodyContains: t.bodyContains,
			Timeout:              tv,
			Interval:             iv,
			Labels:               t.labels,
		})
	}
	s.appendHTTPTargets(out)
}

func (s *Server) appendICMPTargets(newTargets []config.ICMPTarget) {
	path := filepath.Join(s.deps.ConfigDir, "icmp.yaml")

	cfg := &config.ICMPConfig{}
	readYAMLFile(path, cfg)

	if cfg.TargetsFile != "" {
		tf := cfg.TargetsFile
		if !filepath.IsAbs(tf) {
			tf = filepath.Join(s.deps.ConfigDir, tf)
		}

		data, _ := os.ReadFile(tf)
		var existing []config.ICMPTarget
		if len(data) > 0 {
			yaml.Unmarshal(data, &existing)
		}
		existing = append(existing, newTargets...)
		out, _ := yaml.Marshal(existing)
		os.WriteFile(tf, out, 0644)
		return
	}

	cfg.Targets = append(cfg.Targets, newTargets...)
	writeYAMLFile(path, cfg)
}

func (s *Server) appendDNSTargets(newTargets []config.DNSTarget) {
	path := filepath.Join(s.deps.ConfigDir, "dns.yaml")
	cfg := &config.DNSConfig{}
	readYAMLFile(path, cfg)
	cfg.Targets = append(cfg.Targets, newTargets...)
	writeYAMLFile(path, cfg)
}

func (s *Server) appendSNMPTargets(newTargets []config.SNMPTarget) {
	path := filepath.Join(s.deps.ConfigDir, "snmp.yaml")
	cfg := &config.SNMPConfig{}
	readYAMLFile(path, cfg)
	cfg.Targets = append(cfg.Targets, newTargets...)
	writeYAMLFile(path, cfg)
}

func (s *Server) appendPortTargets(newTargets []config.PortTarget) {
	path := filepath.Join(s.deps.ConfigDir, "port.yaml")
	cfg := &config.PortConfig{}
	readYAMLFile(path, cfg)
	cfg.Targets = append(cfg.Targets, newTargets...)
	writeYAMLFile(path, cfg)
}

func (s *Server) appendHTTPTargets(newTargets []config.HTTPTarget) {
	path := filepath.Join(s.deps.ConfigDir, "http.yaml")
	cfg := &config.HTTPConfig{}
	readYAMLFile(path, cfg)
	cfg.Targets = append(cfg.Targets, newTargets...)
	writeYAMLFile(path, cfg)
}

// existing helpers kept below

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

func (s *Server) addPortTarget(host string, port int, protocol string, interval time.Duration, labels map[string]string) {
	cfg := &config.PortConfig{}
	path := filepath.Join(s.deps.ConfigDir, "port.yaml")
	readYAMLFile(path, cfg)

	if interval <= 0 {
		interval = 60 * time.Second
	}

	cfg.Targets = append(cfg.Targets, config.PortTarget{
		Host:     host,
		Port:     port,
		Protocol: protocol,
		Timeout:  5 * time.Second,
		Interval: interval,
		Labels:   labels,
	})

	writeYAMLFile(path, cfg)
}

func (s *Server) addHTTPTarget(url, method, bodyContains string, interval time.Duration, labels map[string]string) {
	cfg := &config.HTTPConfig{}
	path := filepath.Join(s.deps.ConfigDir, "http.yaml")
	readYAMLFile(path, cfg)

	if interval <= 0 {
		interval = 60 * time.Second
	}

	cfg.Targets = append(cfg.Targets, config.HTTPTarget{
		URL:                  url,
		Method:               method,
		ExpectedStatusCodes:  []int{200},
		ExpectedBodyContains: bodyContains,
		Timeout:              10 * time.Second,
		Interval:             interval,
		Labels:               labels,
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

func (s *Server) removePortTarget(host string) bool {
	cfg := &config.PortConfig{}
	path := filepath.Join(s.deps.ConfigDir, "port.yaml")
	readYAMLFile(path, cfg)

	filtered := cfg.Targets[:0]
	found := false
	for _, t := range cfg.Targets {
		key := fmt.Sprintf("%s:%d", t.Host, t.Port)
		if key == host || t.Host == host {
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

func (s *Server) removeHTTPTarget(host string) bool {
	cfg := &config.HTTPConfig{}
	path := filepath.Join(s.deps.ConfigDir, "http.yaml")
	readYAMLFile(path, cfg)

	filtered := cfg.Targets[:0]
	found := false
	for _, t := range cfg.Targets {
		if t.URL == host {
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

func argNumber(req mcpcore.CallToolRequest, name string) float64 {
	args, ok := req.Params.Arguments.(map[string]any)
	if !ok {
		return 0
	}
	v, ok := args[name]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case string:
		f, _ := strconv.ParseFloat(n, 64)
		return f
	}
	return 0
}

func copyLabels(labels map[string]string) map[string]string {
	if labels == nil {
		return nil
	}
	m := make(map[string]string, len(labels))
	for k, v := range labels {
		m[k] = v
	}
	return m
}
