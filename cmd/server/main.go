package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/hulipa487/catgirl/internal/config"
	"github.com/hulipa487/catgirl/internal/database"
	"github.com/hulipa487/catgirl/internal/runtime"
	"github.com/hulipa487/catgirl/internal/transport/http"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
)

var (
	configPath string
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start the Catgirl runtime server",
	RunE:  runServer,
}

func init() {
	rootCmd.AddCommand(serverCmd)
	serverCmd.PersistentFlags().StringVarP(&configPath, "config", "c", "/etc/catgirl.conf", "Path to configuration file")
}

func runServer(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	logger := setupLogger(cfg.Logging.Level, cfg.Logging.Format)

	logger.Info().
		Str("config_path", configPath).
		Msg("configuration loaded")

	dbInstance, err := database.New(&cfg.Database, logger)
	if err != nil {
		return fmt.Errorf("failed to create database: %w", err)
	}
	defer dbInstance.Close()

	migrationRunner := database.NewMigrationRunner(dbInstance, logger)
	if err := migrationRunner.Run(context.Background(), "./migrations"); err != nil {
		logger.Warn().Err(err).Msg("migration warning (continuing anyway)")
	}

	rt, err := runtime.NewRuntimeCoordinator(cfg, logger)
	if err != nil {
		return fmt.Errorf("failed to create runtime: %w", err)
	}

	server := http.NewServer(rt, cfg, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := rt.Start(ctx); err != nil {
		return fmt.Errorf("failed to start runtime: %w", err)
	}

	go func() {
		if err := server.Start(); err != nil {
			logger.Error().Err(err).Msg("HTTP server error")
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	<-sigCh

	logger.Info().Msg("shutdown signal received")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30)
	defer shutdownCancel()

	if err := server.Stop(shutdownCtx); err != nil {
		logger.Error().Err(err).Msg("failed to stop HTTP server")
	}

	if err := rt.Stop(); err != nil {
		logger.Error().Err(err).Msg("failed to stop runtime")
	}

	logger.Info().Msg("server stopped")
	return nil
}

func setupLogger(level, format string) zerolog.Logger {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	lvl, err := zerolog.ParseLevel(level)
	if err != nil {
		lvl = zerolog.InfoLevel
	}

	zerolog.SetGlobalLevel(lvl)

	var output io.Writer
	output = zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: "2006-01-02 15:04:05"}

	return zerolog.New(output).With().Timestamp().Logger()
}

var rootCmd = &cobra.Command{
	Use:   "catgirl",
	Short: "Catgirl Agentic Runtime",
	Long:  "A multi-agent task execution system with global task queue and agent pool",
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
