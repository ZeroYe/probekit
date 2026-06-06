package mcp

import (
	"context"
	"fmt"
	"testing"

	"github.com/ZeroYe/probekit/internal/config"
	"github.com/ZeroYe/probekit/internal/metrics"
	mcpcore "github.com/mark3labs/mcp-go/mcp"
	"go.uber.org/zap"
)

func textContent(c mcpcore.Content) string {
	tc, ok := mcpcore.AsTextContent(c)
	if !ok {
		return fmt.Sprintf("%+v", c)
	}
	return tc.Text
}

func TestGetTargetsHandler(t *testing.T) {
	s := testServer(t)
	req := mcpcore.CallToolRequest{
		Params: mcpcore.CallToolParams{
			Name: "get_targets",
		},
	}

	result, err := s.handleGetTargets(context.Background(), req)
	if err != nil {
		t.Fatalf("handleGetTargets error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", textContent(result.Content[0]))
	}
	if len(result.Content) == 0 || textContent(result.Content[0]) == "" {
		t.Fatal("expected non-empty result")
	}
	t.Logf("get_targets:\n%s", textContent(result.Content[0]))
}

func TestGetTargetsHandlerWithModule(t *testing.T) {
	s := testServer(t)
	req := mcpcore.CallToolRequest{
		Params: mcpcore.CallToolParams{
			Name: "get_targets",
			Arguments: map[string]any{
				"module": "icmp",
			},
		},
	}

	result, err := s.handleGetTargets(context.Background(), req)
	if err != nil {
		t.Fatalf("handleGetTargets error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", textContent(result.Content[0]))
	}
}

func TestGetMetricsHandler(t *testing.T) {
	s := testServer(t)

	s.deps.Registry.Store("icmp/8.8.8.8", []metrics.Metric{
		{Name: "icmp_rtt_ms", Value: 10.5},
		{Name: "icmp_up", Value: 1},
	})
	s.deps.Registry.Store("dns/example.com|8.8.8.8", []metrics.Metric{
		{Name: "dns_up", Value: 1},
	})

	req := mcpcore.CallToolRequest{
		Params: mcpcore.CallToolParams{
			Name: "get_metrics",
			Arguments: map[string]any{
				"target": "8.8.8.8",
			},
		},
	}

	result, err := s.handleGetMetrics(context.Background(), req)
	if err != nil {
		t.Fatalf("handleGetMetrics error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", textContent(result.Content[0]))
	}
	t.Logf("get_metrics:\n%s", textContent(result.Content[0]))
}

func TestGetMetricsHandlerMissingTarget(t *testing.T) {
	s := testServer(t)

	req := mcpcore.CallToolRequest{
		Params: mcpcore.CallToolParams{
			Name: "get_metrics",
			Arguments: map[string]any{
				"target": "nonexistent",
			},
		},
	}

	result, err := s.handleGetMetrics(context.Background(), req)
	if err != nil {
		t.Fatalf("handleGetMetrics error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success for missing target, got error: %s", textContent(result.Content[0]))
	}
}

func testServer(t *testing.T) *Server {
	t.Helper()

	registry := metrics.NewRegistry()
	cfg := &config.Config{
		ICMP: config.ICMPConfig{
			Targets: []config.ICMPTarget{
				{Host: "8.8.8.8"},
				{Host: "114.114.114.114"},
			},
		},
		DNS: config.DNSConfig{
			Targets: []config.DNSTarget{
				{Domain: "example.com", Server: "8.8.8.8", RecordType: "A"},
				{Domain: "example.com", Server: "8.8.8.8", RecordType: "AAAA"},
			},
		},
		SNMP: config.SNMPConfig{
			Targets: []config.SNMPTarget{
				{Host: "192.168.1.1"},
			},
		},
	}

	logger, _ := zap.NewDevelopment()

	s, err := New(Dependencies{
		Registry: registry,
		Config:   cfg,
		Logger:   logger,
	}, config.MCPConfig{
		Enabled: true,
		Listen:  ":0",
	})
	if err != nil {
		t.Fatalf("New server: %v", err)
	}

	return s
}
