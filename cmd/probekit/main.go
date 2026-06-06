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

	pipeline, err := output.NewPipelineFromConfig(output.PipelineConfig{
		VMConfig: cfg.Global.VictoriaMetrics,
		Registry: registry,
		Logger:   logger.Log,
	})
	if err != nil {
		logger.Log.Error("failed to create pipeline", zap.Error(err))
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := pipeline.Start(); err != nil {
		logger.Log.Error("failed to start pipeline", zap.Error(err))
		os.Exit(1)
	}

	colMgr := collector.NewManager()

	colMgr.Add(collector.NewICMPCollector(cfg.ICMP, logger.Log))
	colMgr.Add(collector.NewDNSCollector(cfg.DNS, logger.Log))
	colMgr.Add(collector.NewSNMPCollector(cfg.SNMP, logger.Log))

	if err := colMgr.Start(ctx, pipeline); err != nil {
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
				if err := restartCollectors(colMgr, newCfg, pipeline); err != nil {
					logger.Log.Error("failed to restart collectors", zap.Error(err))
					return err
				}
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
				if err := restartCollectors(colMgr, newCfg, pipeline); err != nil {
					logger.Log.Error("failed to restart collectors", zap.Error(err))
					continue
				}
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
	cancel()
	colMgr.Stop()
	pipeline.Stop()
}

type targetCounterFn func() map[string]int

func (f targetCounterFn) Counts() map[string]int { return f() }

func restartCollectors(colMgr *collector.Manager, cfg *config.Config, pipeline *output.Pipeline) error {
	colMgr.Reset()
	colMgr.Add(collector.NewICMPCollector(cfg.ICMP, logger.Log))
	colMgr.Add(collector.NewDNSCollector(cfg.DNS, logger.Log))
	colMgr.Add(collector.NewSNMPCollector(cfg.SNMP, logger.Log))
	return colMgr.Start(context.Background(), pipeline)
}
