package db

import (
	"context"
	"embed"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/001_init.sql
var migrationsFS embed.FS

func Connect(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse db config: %w", err)
	}

	cfg.MaxConns = 10
	cfg.MinConns = 2
	cfg.MaxConnLifetime = time.Hour

	deadline := time.Now().Add(30 * time.Second)
	var lastErr error

	for time.Now().Before(deadline) {
		pool, err := pgxpool.NewWithConfig(ctx, cfg)
		if err != nil {
			lastErr = err
			time.Sleep(500 * time.Millisecond)
			continue
		}

		pingCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err = pool.Ping(pingCtx)
		cancel()

		if err == nil {
			return pool, nil
		}

		lastErr = err
		pool.Close()
		time.Sleep(500 * time.Millisecond)
	}

	return nil, lastErr
}

func RunMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	sqlBytes, err := migrationsFS.ReadFile("migrations/001_init.sql")
	if err != nil {
		return fmt.Errorf("read migration: %w", err)
	}

	queries := strings.Split(string(sqlBytes), ";")
	for _, q := range queries {
		q = strings.TrimSpace(q)
		if q == "" {
			continue
		}
		if _, err := pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}
	}
	return nil
}
