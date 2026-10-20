package database

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPool opens a pgx connection pool for the api-gateway database.
func NewPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}

	cfg.MaxConns = 10
	cfg.MinConns = 1
	cfg.MaxConnLifetime = time.Hour
	cfg.MaxConnIdleTime = 10 * time.Minute

	return pgxpool.NewWithConfig(ctx, cfg)
}
