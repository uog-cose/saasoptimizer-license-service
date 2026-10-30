package migrations

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// currentVersion identifies the latest embedded schema version.
	currentVersion int64 = 1
	// migrationLockID serializes startup migration across service instances.
	migrationLockID int64 = 1770301
)

// Apply installs all embedded migrations exactly once and serializes concurrent
// service startup with a PostgreSQL advisory transaction lock.
func Apply(ctx context.Context, pool *pgxpool.Pool) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if pool == nil {
		return errors.New("migration PostgreSQL pool is required")
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", migrationLockID); err != nil {
		return fmt.Errorf("lock migration runner: %w", err)
	}
	if _, err := tx.Exec(ctx, `
CREATE TABLE IF NOT EXISTS saasoptimizer_license_schema_migrations (
    version BIGINT PRIMARY KEY,
    applied_at TIMESTAMPTZ NOT NULL
)`); err != nil {
		return fmt.Errorf("create migration metadata: %w", err)
	}
	var applied bool
	if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM saasoptimizer_license_schema_migrations WHERE version = $1)", currentVersion).Scan(&applied); err != nil {
		return fmt.Errorf("check migration version: %w", err)
	}
	if !applied {
		schema, err := Files.ReadFile("00001_activation_records.up.sql")
		if err != nil {
			return fmt.Errorf("read activation migration: %w", err)
		}
		if _, err := tx.Exec(ctx, string(schema)); err != nil {
			return fmt.Errorf("apply activation migration: %w", err)
		}
		if _, err := tx.Exec(ctx, "INSERT INTO saasoptimizer_license_schema_migrations (version, applied_at) VALUES ($1, CURRENT_TIMESTAMP)", currentVersion); err != nil {
			return fmt.Errorf("record migration version: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit migrations: %w", err)
	}
	return nil
}
