package database

import (
	"context"
	"fmt"
	"time"

	"github.com/hulipa487/catgirl/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

type DB struct {
	Pool   *pgxpool.Pool
	Config *config.DatabaseConfig
	Logger zerolog.Logger
}

func New(cfg *config.DatabaseConfig, logger zerolog.Logger) (*DB, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("failed to parse database config: %w", err)
	}

	poolConfig.MaxConns = 25
	poolConfig.MinConns = 5
	poolConfig.MaxConnLifetime = time.Hour
	poolConfig.MaxConnIdleTime = 30 * time.Minute

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &DB{
		Pool:   pool,
		Config: cfg,
		Logger: logger,
	}, nil
}

func (db *DB) Close() {
	db.Pool.Close()
}

func (db *DB) Ping(ctx context.Context) error {
	return db.Pool.Ping(ctx)
}

func (db *DB) Health(ctx context.Context) map[string]interface{} {
	healthy := true
	latency := time.Duration(0)

	start := time.Now()
	if err := db.Pool.Ping(ctx); err != nil {
		healthy = false
	}
	latency = time.Since(start)

	stats := db.Pool.Stat()

	return map[string]interface{}{
		"healthy":          healthy,
		"latency_ms":       latency.Milliseconds(),
		"total_conns":      stats.TotalConns(),
		"idle_conns":       stats.IdleConns(),
		"max_conns":        stats.MaxConns(),
		"acquired_conns":   stats.AcquiredConns(),
	}
}
