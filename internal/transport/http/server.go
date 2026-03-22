package http

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/hulipa487/catgirl/internal/config"
	"github.com/hulipa487/catgirl/internal/runtime"
	"github.com/hulipa487/catgirl/internal/services/auth"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

type Server struct {
	apiServer      *http.Server
	telegramServer *http.Server
	runtime        *runtime.RuntimeCoordinator
	config         *config.Config
	logger         zerolog.Logger
	authService    *auth.AuthService
}

func NewServer(rt *runtime.RuntimeCoordinator, cfg *config.Config, logger zerolog.Logger) *Server {
	// API Router
	apiRouter := gin.New()
	apiRouter.Use(gin.Recovery())
	apiRouter.Use(requestLogger(logger))

	handlers := NewHandlers(rt, cfg, logger)
	authSvc := auth.NewAuthService(&cfg.RuntimeSeed.Auth, logger)

	// Public endpoints (no auth required)
	apiRouter.GET("/health", handlers.HealthCheck)
	apiRouter.GET("/health/detailed", handlers.GetHealth)

	// Protected endpoints - require admin authentication
	protected := apiRouter.Group("")
	protected.Use(authMiddleware(authSvc))
	{
		protected.GET("/api/v1/sessions", handlers.ListSessions)
		protected.GET("/api/v1/sessions/:session_id", handlers.GetSession)
		protected.GET("/api/v1/tasks", handlers.ListTasks)
		protected.GET("/api/v1/tasks/:instance_id", handlers.GetTask)
		protected.GET("/api/v1/queue/status", handlers.GetQueueStatus)
		protected.GET("/api/v1/agents", handlers.ListAgents)
		protected.GET("/api/v1/agents/pool/status", handlers.GetAgentPoolStatus)
		protected.GET("/api/v1/snapshots", handlers.ListSnapshots)
		protected.GET("/api/v1/usage/summary", handlers.GetUsageSummary)
		protected.GET("/api/v1/memory/search", handlers.SearchMemory)
		protected.GET("/api/v1/metrics", handlers.GetSystemMetrics)

		protected.GET("/api/v1/config", handlers.GetConfig)
		protected.PUT("/api/v1/config", handlers.UpdateConfig)
		protected.GET("/api/v1/tools", handlers.ListTools)
	}

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

	for i, b := range cfg.RuntimeSeed.Telegram.Bots {
		webhookPath := fmt.Sprintf("/telegram/webhook/%d", i)
		if u, err := url.Parse(b.WebhookURL); err == nil && u.Path != "" {
			webhookPath = u.Path
		}
		// Map the path to the handler, passing the bot index
		botIndex := i
		telegramRouter.POST(webhookPath, func(c *gin.Context) {
			telegramHandler.HandleWebhookForBot(c, botIndex)
		})
	}

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
		runtime:     rt,
		config:      cfg,
		logger:      logger,
		authService: authSvc,
	}
}

// authMiddleware validates the mtfpass JWT token (via cookie or Bearer header) and requires admin role
func authMiddleware(authSvc *auth.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var token string

		// First try to get token from Authorization header (Bearer token)
		authHeader := c.GetHeader("Authorization")
		if authHeader != "" && len(authHeader) > 7 && authHeader[:7] == "Bearer " {
			token = authHeader[7:]
		} else {
			// Fallback to cookie
			var err error
			token, err = c.Cookie("mtf_auth")
			if err != nil {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
					"success": false,
					"error":   "missing authentication token",
				})
				return
			}
		}

		// Validate token with mtfpass - only admin role is allowed
		result, err := authSvc.Authorize(c.Request.Context(), token)
		if err != nil || !result.Authorized {
			reason := "authentication failed"
			if result != nil && result.Reason != "" {
				reason = result.Reason
			}
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"success": false,
				"error":   reason,
			})
			return
		}

		// Store user ID in context for later use
		c.Set("user_id", result.UserID)
		c.Next()
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
