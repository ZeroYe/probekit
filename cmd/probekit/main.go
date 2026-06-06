package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/ZeroYe/probekit/internal/collector"
	"github.com/ZeroYe/probekit/internal/config"
	"github.com/ZeroYe/probekit/internal/logger"
	probeMCP "github.com/ZeroYe/probekit/internal/mcp"
	"github.com/ZeroYe/probekit/internal/metrics"
	"github.com/ZeroYe/probekit/internal/output"
	"github.com/ZeroYe/probekit/internal/selfmetrics"
	"go.uber.org/zap"
)

func main() {
	var configDir string
	flag.StringVar(&configDir, "config-dir", "./configs", "path to config directory")
	flag.Parse()

	cfg, err := config.Load(configDir)
	if err != nil {
		os.Stderr.WriteString("failed to load config: " + err.Error() + "\n")
		os.Exit(1)
	}

	if err := logger.Init(cfg.Global.LogLevel); err != nil {
		os.Stderr.WriteString("failed to init logger: " + err.Error() + "\n")
		os.Exit(1)
	}

	logger.Log.Info("starting probe-agent",
		zap.String("config_dir", configDir),
	)

	registry := metrics.NewRegistry()

	modulePipelines := make(map[string]*output.Pipeline)
	addPipeline("icmp", cfg.ICMP.EffectiveVM(cfg.Global.VictoriaMetrics), registry, modulePipelines)
	addPipeline("dns", cfg.DNS.EffectiveVM(cfg.Global.VictoriaMetrics), registry, modulePipelines)
	addPipeline("snmp", cfg.SNMP.EffectiveVM(cfg.Global.VictoriaMetrics), registry, modulePipelines)
	addPipeline("port", cfg.Port.EffectiveVM(cfg.Global.VictoriaMetrics), registry, modulePipelines)
	addPipeline("http", cfg.HTTP.EffectiveVM(cfg.Global.VictoriaMetrics), registry, modulePipelines)

	colMgr := collector.NewManager()

	colMgr.Add(collector.NewICMPCollector(cfg.ICMP, logger.Log, cfg.Global.Concurrency, modulePipelines["icmp"]))
	colMgr.Add(collector.NewDNSCollector(cfg.DNS, logger.Log, modulePipelines["dns"]))
	colMgr.Add(collector.NewSNMPCollector(cfg.SNMP, logger.Log, modulePipelines["snmp"]))
	colMgr.Add(collector.NewPortCollector(cfg.Port, logger.Log, modulePipelines["port"]))
	colMgr.Add(collector.NewHTTPCollector(cfg.HTTP, logger.Log, modulePipelines["http"]))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := colMgr.Start(ctx); err != nil {
		logger.Log.Error("failed to start collectors", zap.Error(err))
		os.Exit(1)
	}

	var mcpServer *probeMCP.Server

	if cfg.Global.MCPServer.Enabled {
		mcpServer, err = probeMCP.New(probeMCP.Dependencies{
			Registry:  registry,
			Config:    cfg,
			ConfigDir: configDir,
			Logger:    logger.Log,
			OnReload: func() error {
				logger.Log.Info("reloading config via mcp")
				newCfg, err := config.Load(configDir)
				if err != nil {
					logger.Log.Error("failed to reload config", zap.Error(err))
					return err
				}
				restartAll(modulePipelines, colMgr, newCfg)
				*cfg = *newCfg
				if mcpServer != nil {
					mcpServer.UpdateAuth(cfg.Global.MCPServer)
				}
				logger.Log.Info("config reloaded")
				return nil
			},
		}, cfg.Global.MCPServer)
		if err != nil {
			logger.Log.Error("failed to create mcp server", zap.Error(err))
			os.Exit(1)
		}

		if cfg.Global.SelfMetrics.Enabled {
			tc := targetCounterFn(func() map[string]int {
				return map[string]int{
					"icmp": len(cfg.ICMP.Targets),
					"dns":  len(cfg.DNS.Targets),
					"snmp": len(cfg.SNMP.Targets),
					"port": len(cfg.Port.Targets),
					"http": len(cfg.HTTP.Targets),
				}
			})
			mcpServer.AddHandler(cfg.Global.SelfMetrics.Path, selfmetrics.Handler(cfg.Global.SelfMetrics.CollectRuntime, tc))
			logger.Log.Info("self metrics endpoint registered",
				zap.String("path", cfg.Global.SelfMetrics.Path),
				zap.Bool("collect_runtime", cfg.Global.SelfMetrics.CollectRuntime),
			)
		}

		if err := mcpServer.Start(cfg.Global.MCPServer.Listen); err != nil {
			logger.Log.Error("failed to start mcp server", zap.Error(err))
			os.Exit(1)
		}
		defer mcpServer.Stop(context.Background())
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	reload := make(chan os.Signal, 1)
	signal.Notify(reload, syscall.SIGHUP)

	go func() {
		for {
			select {
			case <-reload:
				logger.Log.Info("reloading config via sighup")
				newCfg, err := config.Load(configDir)
				if err != nil {
					logger.Log.Error("failed to reload config", zap.Error(err))
					continue
				}
				restartAll(modulePipelines, colMgr, newCfg)
				*cfg = *newCfg
				if mcpServer != nil {
					mcpServer.UpdateAuth(cfg.Global.MCPServer)
				}
				logger.Log.Info("config reloaded")
			case <-ctx.Done():
				return
			}
		}
	}()

	logger.Log.Info("probe-agent started")

	<-stop
	logger.Log.Info("shutting down...")
	colMgr.Stop()
	for _, p := range modulePipelines {
		p.Stop()
	}
}

func addPipeline(name string, vmCfg config.VMConfig, registry *metrics.Registry, pipelines map[string]*output.Pipeline) {
	p, err := output.NewPipelineFromConfig(output.PipelineConfig{
		VMConfig: vmCfg,
		Registry: registry,
		Logger:   logger.Log,
	})
	if err != nil {
		logger.Log.Fatal("failed to create pipeline", zap.String("module", name), zap.Error(err))
	}
	if err := p.Start(); err != nil {
		logger.Log.Fatal("failed to start pipeline", zap.String("module", name), zap.Error(err))
	}
	pipelines[name] = p
}

func restartAll(pipelines map[string]*output.Pipeline, colMgr *collector.Manager, cfg *config.Config) {
	colMgr.Reset()

	for name, vmCfg := range map[string]config.VMConfig{
		"icmp": cfg.ICMP.EffectiveVM(cfg.Global.VictoriaMetrics),
		"dns":  cfg.DNS.EffectiveVM(cfg.Global.VictoriaMetrics),
		"snmp": cfg.SNMP.EffectiveVM(cfg.Global.VictoriaMetrics),
		"port": cfg.Port.EffectiveVM(cfg.Global.VictoriaMetrics),
		"http": cfg.HTTP.EffectiveVM(cfg.Global.VictoriaMetrics),
	} {
		old := pipelines[name]
		if old != nil {
			old.Stop()
		}
		addPipeline(name, vmCfg, pipelines[name].Registry(), pipelines)
	}

	colMgr.Add(collector.NewICMPCollector(cfg.ICMP, logger.Log, cfg.Global.Concurrency, pipelines["icmp"]))
	colMgr.Add(collector.NewDNSCollector(cfg.DNS, logger.Log, pipelines["dns"]))
	colMgr.Add(collector.NewSNMPCollector(cfg.SNMP, logger.Log, pipelines["snmp"]))
	colMgr.Add(collector.NewPortCollector(cfg.Port, logger.Log, pipelines["port"]))
	colMgr.Add(collector.NewHTTPCollector(cfg.HTTP, logger.Log, pipelines["http"]))

	if err := colMgr.Start(context.Background()); err != nil {
		logger.Log.Error("failed to restart collectors", zap.Error(err))
	}
}

type targetCounterFn func() map[string]int

func (f targetCounterFn) Counts() map[string]int { return f() }
