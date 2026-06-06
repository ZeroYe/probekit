package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEmptyDir(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "global.yaml"), []byte("log_level: debug\nvictoria_metrics:\n  push_url: http://vm:8428/api/v1/import/prometheus\n  flush_interval: 10s\n  batch_size: 500\nmcp_server:\n  enabled: true\n  listen: :9801\n  auth:\n    type: api_key\n    keys: [test-key]\n"), 0644)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Global.LogLevel != "debug" {
		t.Errorf("expected log_level debug, got %q", cfg.Global.LogLevel)
	}
	if cfg.Global.VictoriaMetrics.PushURL != "http://vm:8428/api/v1/import/prometheus" {
		t.Errorf("unexpected push_url")
	}
	if !cfg.Global.MCPServer.Enabled {
		t.Errorf("expected mcp enabled")
	}
	if len(cfg.Global.MCPServer.Auth.Keys) != 1 || cfg.Global.MCPServer.Auth.Keys[0] != "test-key" {
		t.Errorf("unexpected auth keys")
	}
}

func TestLoadAllConfigs(t *testing.T) {
	dir := t.TempDir()

	writeYAML(t, dir, "global.yaml", globalYAML)
	writeYAML(t, dir, "icmp.yaml", icmpYAML)
	writeYAML(t, dir, "snmp.yaml", snmpYAML)
	writeYAML(t, dir, "dns.yaml", dnsYAML)
	writeYAML(t, dir, "port.yaml", portYAML)
	writeYAML(t, dir, "http.yaml", httpYAML)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(cfg.ICMP.Targets) != 2 {
		t.Errorf("icmp: expected 2 targets, got %d", len(cfg.ICMP.Targets))
	}
	if len(cfg.SNMP.Targets) != 2 {
		t.Errorf("snmp: expected 2 targets, got %d", len(cfg.SNMP.Targets))
	}
	if len(cfg.DNS.Targets) != 3 {
		t.Errorf("dns: expected 3 targets, got %d", len(cfg.DNS.Targets))
	}
	if len(cfg.Port.Targets) != 2 {
		t.Errorf("port: expected 2 targets, got %d", len(cfg.Port.Targets))
	}
	if len(cfg.HTTP.Targets) != 2 {
		t.Errorf("http: expected 2 targets, got %d", len(cfg.HTTP.Targets))
	}
}

func TestLoadMissingNonGlobal(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, "global.yaml", globalYAML)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load failed with missing non-global configs: %v", err)
	}
	if cfg == nil {
		t.Fatal("cfg is nil")
	}
}

func writeYAML(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

const globalYAML = `log_level: info
victoria_metrics:
  push_url: "http://victoria:8428/api/v1/import/prometheus"
  flush_interval: 10s
  batch_size: 500
mcp_server:
  enabled: true
  listen: ":9801"
  auth:
    type: api_key
    keys: ["test-key-1", "test-key-2"]
`

const icmpYAML = `histogram_buckets_ms: [1, 5, 10, 20, 50, 100]
targets:
  - host: "8.8.8.8"
    interval: 60s
    count: 4
    timeout: 5s
    labels:
      region: global
  - host: "114.114.114.114"
    interval: 30s
    count: 2
    timeout: 3s
`

const snmpYAML = `defaults:
  version: "2c"
  community: "public"
  port: 161
  timeout: 5s
  retries: 1
targets:
  - host: "192.168.1.1"
    interval: 60s
    labels:
      role: gw
    oids:
      scalar: ["1.3.6.1.2.1.1.3.0", "1.3.6.1.2.1.1.5.0"]
      tables:
        - oid: "1.3.6.1.2.1.2.2"
          index: "ifIndex"
          tag: "ifDescr"
          metrics:
            - oid: "1.3.6.1.2.1.2.2.1.10"
              name: "ifInOctets"
              type: "counter"
  - host: "192.168.1.2"
    interval: 120s
`

const portYAML = `targets:
  - host: "192.168.1.1"
    port: 443
    protocol: tcp
    timeout: 5s
    interval: 30s
    labels:
      service: web
  - host: "8.8.8.8"
    port: 53
    protocol: udp
    timeout: 3s
    interval: 60s
`

const httpYAML = `targets:
  - url: "https://example.com/health"
    method: GET
    expected_status_codes: [200]
    timeout: 10s
    interval: 60s
    labels:
      service: example
  - url: "https://api.example.com/status"
    method: GET
    expected_status_codes: [200, 301]
    expected_body_contains: "ok"
    timeout: 5s
    interval: 30s
`

const dnsYAML = `targets:
  - domain: "example.com"
    server: "8.8.8.8"
    record_type: "A"
    interval: 60s
    labels:
      type: external
  - domain: "example.com"
    server: "8.8.8.8"
    record_type: "AAAA"
    interval: 60s
  - domain: "mail.example.com"
    server: "1.1.1.1"
    record_type: "MX"
    interval: 120s
`
