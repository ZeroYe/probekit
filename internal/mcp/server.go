package mcp

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"github.com/ZeroYe/probekit/internal/config"
	"github.com/ZeroYe/probekit/internal/metrics"
	mcpcore "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"go.uber.org/zap"
)

type Dependencies struct {
	Registry  *metrics.Registry
	Config    *config.Config
	ConfigDir string
	Logger    *zap.Logger
	OnReload  func() error
}

type Server struct {
	mcpServer  *server.MCPServer
	httpServer *server.StreamableHTTPServer
	auth       *APIKeyAuth
	deps       Dependencies
	cfg        config.MCPConfig
	handler    http.Handler
	mux        *http.ServeMux
	httpSrv    *http.Server
	mu         sync.Mutex
}

func New(deps Dependencies, cfg config.MCPConfig) (*Server, error) {
	s := &Server{
		deps: deps,
		cfg:  cfg,
	}

	s.mcpServer = server.NewMCPServer(
		"ProbeKit",
		"1.0.0",
		server.WithToolCapabilities(true),
	)

	s.registerTools()

	s.httpServer = server.NewStreamableHTTPServer(
		s.mcpServer,
		server.WithEndpointPath("/mcp"),
	)

	handler := http.Handler(s.httpServer)

	if cfg.Auth.Type == "api_key" && len(cfg.Auth.Keys) > 0 {
		s.auth = NewAPIKeyAuth(cfg.Auth.Keys)
		handler = AuthMiddleware(handler, s.auth)
		deps.Logger.Info("mcp api key auth enabled")
	}

	s.mux = http.NewServeMux()
	s.mux.Handle("/mcp", handler)

	s.handler = s.mux

	return s, nil
}

func (s *Server) Handler() http.Handler {
	return s.handler
}

func (s *Server) AddHandler(path string, h http.Handler) {
	s.mux.Handle(path, h)
}

func (s *Server) Start(addr string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.httpSrv != nil {
		return fmt.Errorf("mcp server already started")
	}

	s.httpSrv = &http.Server{
		Addr:    addr,
		Handler: s.mux,
	}

	s.deps.Logger.Info("mcp server starting",
		zap.String("addr", addr),
		zap.String("endpoint", "/mcp"),
	)

	go func() {
		if err := s.httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.deps.Logger.Error("mcp server error", zap.Error(err))
		}
	}()

	return nil
}

func (s *Server) UpdateAuth(cfg config.MCPConfig) {
	if s.auth != nil && cfg.Auth.Type == "api_key" {
		s.auth.UpdateKeys(cfg.Auth.Keys)
	}
}

func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.httpSrv == nil {
		return nil
	}

	return s.httpSrv.Shutdown(ctx)
}

func (s *Server) registerTools() {
	s.registerGetTargets()
	s.registerGetMetrics()
	s.registerAddTarget()
	s.registerRemoveTarget()
	s.registerReloadConfig()
}

func argString(req mcpcore.CallToolRequest, name string) string {
	args, ok := req.Params.Arguments.(map[string]any)
	if !ok {
		return ""
	}
	v, ok := args[name]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}
