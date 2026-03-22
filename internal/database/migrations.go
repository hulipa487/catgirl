package database

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rs/zerolog"
)

type Migration struct {
	Version string
	Name    string
	SQL     string
}

func (m *Migration) Applied() string {
	return fmt.Sprintf("applied_%s", m.Version)
}

type MigrationRunner struct {
	db     *DB
	logger zerolog.Logger
}

func NewMigrationRunner(db *DB, logger zerolog.Logger) *MigrationRunner {
	return &MigrationRunner{
		db:     db,
		logger: logger,
	}
}

func (r *MigrationRunner) LoadMigrations(migrationsPath string) ([]*Migration, error) {
	entries, err := os.ReadDir(migrationsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read migrations directory: %w", err)
	}

	var migrations []*Migration
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		content, err := os.ReadFile(filepath.Join(migrationsPath, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("failed to read migration %s: %w", entry.Name(), err)
		}

		version := strings.TrimSuffix(entry.Name(), ".sql")

		nameParts := strings.SplitN(version, "_", 2)
		if len(nameParts) < 2 {
			// If no underscore, treat the whole name as both version and name (e.g. "schema")
			migrations = append(migrations, &Migration{
				Version: version,
				Name:    version,
				SQL:     string(content),
			})
			continue
		}

		migrations = append(migrations, &Migration{
			Version: nameParts[0],
			Name:    nameParts[1],
			SQL:     string(content),
		})
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	return migrations, nil
}

func (r *MigrationRunner) Run(ctx context.Context, migrationsPath string) error {
	// Check if this is the first run
	var tableCount int
	err := r.db.Pool.QueryRow(ctx, "SELECT count(*) FROM information_schema.tables WHERE table_name = 'sessions'").Scan(&tableCount)
	if err != nil {
		return fmt.Errorf("failed to check existing tables: %w", err)
	}

	if tableCount == 0 {
		r.logger.Info().Msg("no tables found, running initial schema setup")
		// The 001_initial_schema.sql will be applied automatically by the logic below
	}

	migrations, err := r.LoadMigrations(migrationsPath)
	if err != nil {
		return fmt.Errorf("failed to load migrations: %w", err)
	}

	for _, m := range migrations {
		applied, err := r.isApplied(ctx, m.Version)
		if err != nil {
			return fmt.Errorf("failed to check migration %s: %w", m.Version, err)
		}

		if applied {
			r.logger.Info().Str("version", m.Version).Str("name", m.Name).Msg("migration already applied")
			continue
		}

		r.logger.Info().Str("version", m.Version).Str("name", m.Name).Msg("applying migration")

		tx, err := r.db.Pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("failed to begin transaction for migration %s: %w", m.Version, err)
		}

		if _, err := tx.Exec(ctx, m.SQL); err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("failed to execute migration %s: %w", m.Version, err)
		}

		if _, err := tx.Exec(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", m.Version); err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("failed to record migration %s: %w", m.Version, err)
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("failed to commit migration %s: %w", m.Version, err)
		}

		r.logger.Info().Str("version", m.Version).Str("name", m.Name).Msg("migration applied successfully")
	}

	return nil
}

func (r *MigrationRunner) isApplied(ctx context.Context, version string) (bool, error) {
	// ensure the table exists before querying
	_, err := r.db.Pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version VARCHAR(255) PRIMARY KEY,
			applied_at TIMESTAMPTZ DEFAULT NOW()
		)
	`)
	if err != nil {
		return false, fmt.Errorf("failed to create schema_migrations table: %w", err)
	}

	var count int
	err = r.db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM schema_migrations WHERE version = $1", version).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check migration status: %w", err)
	}
	return count > 0, nil
}
