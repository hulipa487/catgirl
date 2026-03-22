package http

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/hulipa487/catgirl/internal/config"
	"github.com/hulipa487/catgirl/internal/runtime"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

type Server struct {
	apiServer      *http.Server
	telegramServer *http.Server
	runtime        *runtime.RuntimeCoordinator
	config         *config.Config
	logger         zerolog.Logger
}

func NewServer(rt *runtime.RuntimeCoordinator, cfg *config.Config, logger zerolog.Logger) *Server {
	// API Router
	apiRouter := gin.New()
	apiRouter.Use(gin.Recovery())
	apiRouter.Use(requestLogger(logger))

	handlers := NewHandlers(rt, cfg, logger)

	apiRouter.GET("/health", handlers.HealthCheck)
	apiRouter.GET("/health/detailed", handlers.GetHealth)
	apiRouter.GET("/api/v1/sessions", handlers.ListSessions)
	apiRouter.GET("/api/v1/sessions/:session_id", handlers.GetSession)
	apiRouter.PUT("/api/v1/sessions/:session_id/settings", handlers.UpdateSessionSettings)
	apiRouter.GET("/api/v1/tasks", handlers.ListTasks)
	apiRouter.GET("/api/v1/tasks/:instance_id", handlers.GetTask)
	apiRouter.GET("/api/v1/queue/status", handlers.GetQueueStatus)
	apiRouter.GET("/api/v1/agents", handlers.ListAgents)
	apiRouter.GET("/api/v1/agents/pool/status", handlers.GetAgentPoolStatus)
	apiRouter.GET("/api/v1/snapshots", handlers.ListSnapshots)
	apiRouter.GET("/api/v1/usage/summary", handlers.GetUsageSummary)
	apiRouter.GET("/api/v1/memory/search", handlers.SearchMemory)
	apiRouter.GET("/api/v1/metrics", handlers.GetSystemMetrics)

	apiRouter.GET("/api/v1/config", handlers.GetConfig)
	apiRouter.PUT("/api/v1/config", handlers.UpdateConfig)

	// API static files for admin panel (Vue.js frontend)
	apiRouter.StaticFS("/admin", http.Dir("web/admin/dist"))
	// Setup catch-all for SPA routing if needed (Vue Router)
	apiRouter.NoRoute(func(c *gin.Context) {
		c.File("web/admin/dist/index.html")
	})

	// Telegram Router
	telegramRouter := gin.New()
	telegramRouter.Use(gin.Recovery())
	telegramRouter.Use(requestLogger(logger))

	telegramHandler := NewTelegramHandler(rt.GetTelegramService())

	webhookPath := "/telegram/webhook"
	if u, err := url.Parse(cfg.RuntimeSeed.Telegram.WebhookURL); err == nil && u.Path != "" {
		webhookPath = u.Path
	}
	telegramRouter.POST(webhookPath, telegramHandler.HandleWebhook)

	return &Server{
		apiServer: &http.Server{
			Addr:         cfg.Server.Addr(),
			Handler:      apiRouter,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
			IdleTimeout:  60 * time.Second,
		},
		telegramServer: &http.Server{
			Addr:         cfg.RuntimeSeed.Telegram.ListenAddr,
			Handler:      telegramRouter,
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
	errCh := make(chan error, 2)

	go func() {
		s.logger.Info().Str("addr", s.config.Server.Addr()).Msg("starting API HTTP server")
		if err := s.apiServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("API HTTP server error: %w", err)
		}
	}()

	go func() {
		s.logger.Info().Str("addr", s.config.RuntimeSeed.Telegram.ListenAddr).Msg("starting Telegram Webhook HTTP server")
		if err := s.telegramServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("Telegram Webhook HTTP server error: %w", err)
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-time.After(1 * time.Second): // Give servers a moment to start without error
		return nil
	}
}

func (s *Server) Stop(ctx context.Context) error {
	s.logger.Info().Msg("stopping HTTP servers")

	err1 := s.apiServer.Shutdown(ctx)
	err2 := s.telegramServer.Shutdown(ctx)

	if err1 != nil {
		return err1
	}
	return err2
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
