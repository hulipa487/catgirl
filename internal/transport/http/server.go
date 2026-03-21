package http

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/hulipa487/catgirl/internal/config"
	"github.com/hulipa487/catgirl/internal/runtime"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

type Server struct {
	httpServer *http.Server
	runtime   *runtime.RuntimeCoordinator
	config    *config.Config
	logger    zerolog.Logger
}

func NewServer(rt *runtime.RuntimeCoordinator, cfg *config.Config, logger zerolog.Logger) *Server {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(requestLogger(logger))

	handlers := NewHandlers(rt, cfg, logger)

	router.GET("/health", handlers.HealthCheck)
	router.GET("/health/detailed", handlers.GetHealth)

	router.GET("/api/v1/sessions", handlers.ListSessions)
	router.GET("/api/v1/sessions/:session_id", handlers.GetSession)

	router.GET("/api/v1/tasks", handlers.ListTasks)
	router.GET("/api/v1/tasks/:instance_id", handlers.GetTask)
	router.GET("/api/v1/queue/status", handlers.GetQueueStatus)

	router.GET("/api/v1/agents", handlers.ListAgents)
	router.GET("/api/v1/agents/pool/status", handlers.GetAgentPoolStatus)

	router.GET("/api/v1/snapshots", handlers.ListSnapshots)

	router.GET("/api/v1/skills", handlers.ListSkills)

	router.GET("/api/v1/mcp/servers", handlers.ListMCPServers)

	router.GET("/api/v1/usage/summary", handlers.GetUsageSummary)

	router.GET("/api/v1/memory/search", handlers.SearchMemory)

	router.GET("/api/v1/metrics", handlers.GetSystemMetrics)

	return &Server{
		httpServer: &http.Server{
			Addr:         cfg.Server.Addr(),
			Handler:      router,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
			IdleTimeout:  60 * time.Second,
		},
		runtime: rt,
		config:  cfg,
		logger:  logger,
	}
}

func (s *Server) Start() error {
	s.logger.Info().Str("addr", s.config.Server.Addr()).Msg("starting HTTP server")

	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("HTTP server error: %w", err)
	}

	return nil
}

func (s *Server) Stop(ctx context.Context) error {
	s.logger.Info().Msg("stopping HTTP server")
	return s.httpServer.Shutdown(ctx)
}

func requestLogger(logger zerolog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		method := c.Request.Method

		c.Next()

		logger.Info().
			Str("method", method).
			Str("path", path).
			Int("status", c.Writer.Status()).
			Dur("latency", time.Since(start)).
			Msg("HTTP request")
	}
}
