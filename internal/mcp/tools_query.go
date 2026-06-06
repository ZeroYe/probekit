package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"probe-agent/internal/metrics"
	mcpcore "github.com/mark3labs/mcp-go/mcp"
)

func (s *Server) registerGetTargets() {
	tool := mcpcore.NewTool("get_targets",
		mcpcore.WithDescription("List all monitoring targets with their module, host, and labels"),
		mcpcore.WithString("module",
			mcpcore.Description("Filter by module: icmp, snmp, dns (empty returns all)"),
		),
	)

	s.mcpServer.AddTool(tool, s.handleGetTargets)
}

func (s *Server) handleGetTargets(ctx context.Context, req mcpcore.CallToolRequest) (*mcpcore.CallToolResult, error) {
	filter := argString(req, "module")

	type targetInfo struct {
		Host   string            `json:"host"`
		Module string            `json:"module"`
		Labels map[string]string `json:"labels"`
		Up     bool              `json:"up"`
	}

	var targets []targetInfo

	if filter == "" || filter == "icmp" {
		for _, t := range s.deps.Config.ICMP.Targets {
			targets = append(targets, targetInfo{
				Host:   t.Host,
				Module: "icmp",
				Labels: t.Labels,
				Up:     checkModuleUp(s.deps.Registry, "icmp/"+t.Host),
			})
		}
	}

	if filter == "" || filter == "dns" {
		for _, t := range s.deps.Config.DNS.Targets {
			targets = append(targets, targetInfo{
				Host:   t.Domain,
				Module: "dns",
				Labels: mergeMapLabels(t.Labels, "server", t.Server, "record_type", t.RecordType),
				Up:     checkModuleUp(s.deps.Registry, "dns/"+t.Domain+"|"+t.Server),
			})
		}
	}

	if filter == "" || filter == "snmp" {
		for _, t := range s.deps.Config.SNMP.Targets {
			targets = append(targets, targetInfo{
				Host:   t.Host,
				Module: "snmp",
				Labels: t.Labels,
				Up:     checkModuleUp(s.deps.Registry, "snmp/"+t.Host),
			})
		}
	}

	if len(targets) == 0 {
		return mcpcore.NewToolResultText("No targets found."), nil
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Found %d target(s):\n\n", len(targets)))
	for _, t := range targets {
		status := "UP"
		if !t.Up {
			status = "DOWN"
		}
		b.WriteString(fmt.Sprintf("[%s] %s [%s] labels=%v\n", status, t.Module, t.Host, t.Labels))
	}

	return mcpcore.NewToolResultText(b.String()), nil
}

func (s *Server) registerGetMetrics() {
	tool := mcpcore.NewTool("get_metrics",
		mcpcore.WithDescription("Get latest metrics for a specific target"),
		mcpcore.WithString("target",
			mcpcore.Required(),
			mcpcore.Description("Target identifier: host IP for icmp/snmp, domain for dns"),
		),
		mcpcore.WithString("module",
			mcpcore.Description("Module: icmp, snmp, dns (auto-detected if not specified)"),
		),
	)

	s.mcpServer.AddTool(tool, s.handleGetMetrics)
}

func (s *Server) handleGetMetrics(ctx context.Context, req mcpcore.CallToolRequest) (*mcpcore.CallToolResult, error) {
	target := argString(req, "target")
	module := argString(req, "module")

	if target == "" {
		return mcpcore.NewToolResultError("target is required"), nil
	}

	keys := s.findMetricKeys(target, module)

	if len(keys) == 0 {
		return mcpcore.NewToolResultText(fmt.Sprintf("No metrics found for target: %s", target)), nil
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Metrics for %s:\n\n", target))

	for _, key := range keys {
		ms := s.deps.Registry.Get(key)
		if len(ms) == 0 {
			continue
		}
		b.WriteString(fmt.Sprintf("--- %s ---\n", key))
		for _, m := range ms {
			labelStr := fmtLabels(m.Labels)
			b.WriteString(fmt.Sprintf("  %s%s = %.3f\n", m.Name, labelStr, m.Value))
		}
		b.WriteString("\n")
	}

	return mcpcore.NewToolResultText(b.String()), nil
}

func (s *Server) findMetricKeys(target, module string) []string {
	var allKeys []string

	if module == "" || module == "icmp" {
		if key := "icmp/" + target; s.deps.Registry.Get(key) != nil {
			allKeys = append(allKeys, key)
		}
	}

	if module == "" || module == "dns" {
		for _, k := range s.deps.Registry.Keys() {
			if strings.HasPrefix(k, "dns/") && strings.Contains(k, target) {
				allKeys = append(allKeys, k)
			}
		}
	}

	if module == "" || module == "snmp" {
		if key := "snmp/" + target; s.deps.Registry.Get(key) != nil {
			allKeys = append(allKeys, key)
		}
	}

	return allKeys
}

func checkModuleUp(registry *metrics.Registry, key string) bool {
	ms := registry.Get(key)
	if ms == nil {
		return true
	}
	for _, m := range ms {
		if strings.HasSuffix(m.Name, "_up") && m.Value == 0 {
			return false
		}
	}
	return true
}

func mergeMapLabels(labels map[string]string, kv ...string) map[string]string {
	m := make(map[string]string, len(labels)+len(kv)/2)
	for k, v := range labels {
		m[k] = v
	}
	for i := 0; i < len(kv)-1; i += 2 {
		m[kv[i]] = kv[i+1]
	}
	return m
}

func fmtLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	data, err := json.Marshal(labels)
	if err != nil {
		return fmt.Sprintf("%v", labels)
	}
	return string(data)
}
